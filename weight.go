package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/exploded/monitor/pkg/logship"
)

var discordWebhookURL string

// Template cache
var templates *template.Template

// Initialize templates at startup
func init() {
	var err error
	templates, err = template.ParseGlob("templates/*.html")
	if err != nil {
		slog.Warn("error parsing templates", "error", err)
	}
}

// Add request logging
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("request", "method", r.Method, "uri", r.RequestURI, "duration", time.Since(start))
	})
}

// Add security headers to all responses
func securityHeaders(isProd bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		if isProd {
			w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}

// Add caching headers for static assets
func cacheStaticAssets(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
		next.ServeHTTP(w, r)
	})
}

func makeServerFromMux(mux *http.ServeMux, isProd bool) *http.Server {
	return &http.Server{
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
		Handler:      requestLogger(securityHeaders(isProd, mux)),
	}
}

func makeHTTPServer(isProd bool) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/favicon.ico", handleFavicon)
	mux.HandleFunc("POST /api/weight", handlePostWeight)
	mux.HandleFunc("GET /api/weights", handleGetWeights)

	path, _ := os.Getwd()
	slog.Info("working directory", "path", path)
	fileServer := http.FileServer(http.Dir(path + "/static"))
	mux.Handle("/static/", http.StripPrefix("/static/", cacheStaticAssets(fileServer)))

	return makeServerFromMux(mux, isProd)
}

func main() {
	var isProd bool
	if os.Getenv("PROD") == "True" {
		isProd = true
	}

	httpPort := os.Getenv("PORT")
	if httpPort == "" {
		httpPort = "8990"
	}
	httpPort = ":" + httpPort

	discordWebhookURL = os.Getenv("DISCORD_WEBHOOK_URL")

	// Set up log shipping to monitor portal
	monitorURL := os.Getenv("MONITOR_URL")
	monitorKey := os.Getenv("MONITOR_API_KEY")

	if monitorURL != "" && monitorKey != "" {
		ship := logship.New(logship.Options{
			Endpoint: monitorURL + "/api/logs",
			APIKey:   monitorKey,
			App:      "weight",
			Level:    slog.LevelWarn,
		})
		defer ship.Shutdown()

		logger := slog.New(logship.Multi(
			slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}),
			ship,
		))
		slog.SetDefault(logger)
		slog.Warn("weight app started, log shipping active", "endpoint", monitorURL+"/api/logs")
	} else {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))
	}

	// Initialize database
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "weight.db"
	}
	if err := initDB(dbPath); err != nil {
		slog.Error("failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer closeDB()

	slog.Info("configuration", "prod", isProd, "port", httpPort, "db", dbPath)

	httpSrv := makeHTTPServer(isProd)
	httpSrv.Addr = httpPort

	// Start server in goroutine
	go func() {
		slog.Info("starting HTTP server", "addr", httpSrv.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down server")

	// Give outstanding requests 5 seconds to complete
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
	}

	slog.Info("server exited")
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if templates != nil {
		if err := templates.ExecuteTemplate(w, "index.html", nil); err != nil {
			slog.Error("error executing index template", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	} else {
		t, err := template.ParseFiles("templates/index.html")
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			slog.Error("error parsing index template", "error", err)
			return
		}
		if err := t.Execute(w, nil); err != nil {
			slog.Error("error executing index template", "error", err)
		}
	}
}

type postWeightRequest struct {
	WeightKg float64 `json:"weight_kg"`
}

func handlePostWeight(w http.ResponseWriter, r *http.Request) {
	var req postWeightRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if req.WeightKg <= 0 || req.WeightKg > 1000 {
		http.Error(w, `{"error":"weight_kg must be between 0 and 1000"}`, http.StatusBadRequest)
		return
	}

	weight, err := insertWeight(req.WeightKg)
	if err != nil {
		slog.Error("error inserting weight", "error", err)
		http.Error(w, `{"error":"failed to store reading"}`, http.StatusInternalServerError)
		return
	}

	go notifyDiscord(weight)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(weight)
}

func handleGetWeights(w http.ResponseWriter, r *http.Request) {
	days := 0
	if d := r.URL.Query().Get("days"); d != "" {
		parsed, err := strconv.Atoi(d)
		if err == nil && parsed > 0 {
			days = parsed
		}
	}

	weights, err := getWeights(days)
	if err != nil {
		slog.Error("error getting weights", "error", err)
		http.Error(w, `{"error":"failed to fetch readings"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(weights)
}

func handleFavicon(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "favicon.ico")
}

func notifyDiscord(w Weight) {
	if discordWebhookURL == "" {
		return
	}

	payload := map[string]any{
		"embeds": []map[string]any{{
			"title":       fmt.Sprintf("%.1f kg", w.WeightKg),
			"description": "New weight recorded",
			"color":       0x3B82F6,
			"timestamp":   w.CreatedAt,
		}},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		slog.Error("discord marshal", "error", err)
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(discordWebhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		slog.Error("discord send", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		slog.Warn("discord webhook error", "status", resp.StatusCode)
	}
}
