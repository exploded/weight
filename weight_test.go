package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func setupTestDB(t *testing.T) {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "weight-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	t.Cleanup(func() {
		closeDB()
		os.Remove(tmpFile.Name())
	})
	if err := initDB(tmpFile.Name()); err != nil {
		t.Fatal(err)
	}
}

const testAPIKey = "test-key-0123456789abcdef"

// setAPIKey sets the shared key for one test and restores it afterwards.
func setAPIKey(t *testing.T, key string) {
	t.Helper()
	old := apiKey
	apiKey = key
	t.Cleanup(func() { apiKey = old })
}

func TestPostWeight(t *testing.T) {
	setupTestDB(t)
	setAPIKey(t, testAPIKey)

	body := `{"weight_kg": 85.5}`
	req := httptest.NewRequest("POST", "/api/weight", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testAPIKey)
	w := httptest.NewRecorder()

	handlePostWeight(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var result Weight
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result.WeightKg != 85.5 {
		t.Errorf("expected weight 85.5, got %f", result.WeightKg)
	}
	if result.ID == 0 {
		t.Error("expected non-zero ID")
	}
}

func TestPostWeightInvalid(t *testing.T) {
	setupTestDB(t)
	setAPIKey(t, testAPIKey)

	tests := []struct {
		name string
		body string
		code int
	}{
		{"bad json", `not json`, http.StatusBadRequest},
		{"zero weight", `{"weight_kg": 0}`, http.StatusBadRequest},
		{"negative weight", `{"weight_kg": -5}`, http.StatusBadRequest},
		{"too heavy", `{"weight_kg": 1001}`, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/weight", bytes.NewBufferString(tt.body))
			req.Header.Set("Authorization", "Bearer "+testAPIKey)
			w := httptest.NewRecorder()
			handlePostWeight(w, req)
			if w.Code != tt.code {
				t.Errorf("expected %d, got %d", tt.code, w.Code)
			}
		})
	}
}

func TestPostWeightAuth(t *testing.T) {
	tests := []struct {
		name    string
		key     string // server's WEIGHT_API_KEY
		headers map[string]string
		code    int
	}{
		{"no header", testAPIKey, nil, http.StatusUnauthorized},
		{"wrong key", testAPIKey, map[string]string{"Authorization": "Bearer wrong"}, http.StatusUnauthorized},
		{"key without Bearer", testAPIKey, map[string]string{"Authorization": testAPIKey}, http.StatusUnauthorized},
		{"wrong scheme", testAPIKey, map[string]string{"Authorization": "Basic " + testAPIKey}, http.StatusUnauthorized},
		{"key prefix only", testAPIKey, map[string]string{"Authorization": "Bearer " + testAPIKey[:8]}, http.StatusUnauthorized},
		{"key in X-API-Key", testAPIKey, map[string]string{"X-API-Key": testAPIKey}, http.StatusUnauthorized},
		{"correct key", testAPIKey, map[string]string{"Authorization": "Bearer " + testAPIKey}, http.StatusCreated},
		{"server key unset", "", nil, http.StatusServiceUnavailable},
		{"server key unset, empty token", "", map[string]string{"Authorization": "Bearer "}, http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupTestDB(t)
			setAPIKey(t, tt.key)

			req := httptest.NewRequest("POST", "/api/weight", bytes.NewBufferString(`{"weight_kg": 85.5}`))
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			handlePostWeight(w, req)

			if w.Code != tt.code {
				t.Fatalf("expected %d, got %d: %s", tt.code, w.Code, w.Body.String())
			}
			// A rejected request must not store anything.
			weights, err := getWeights(0)
			if err != nil {
				t.Fatal(err)
			}
			if want := map[bool]int{true: 1, false: 0}[tt.code == http.StatusCreated]; len(weights) != want {
				t.Errorf("expected %d stored readings, got %d", want, len(weights))
			}
		})
	}
}

func TestInsertWeightDuplicateWindow(t *testing.T) {
	tests := []struct {
		name     string
		existing float64       // weight already stored
		age      time.Duration // how long ago it was stored
		wantDup  bool
	}{
		{"same weight 2 minutes ago", 85.5, 2 * time.Minute, false},
		{"same weight 30 seconds ago", 85.5, 30 * time.Second, true},
		{"different weight 30 seconds ago", 85.4, 30 * time.Second, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupTestDB(t)
			createdAt := time.Now().UTC().Add(-tt.age).Format(time.RFC3339)
			if _, err := db.Exec("INSERT INTO weights (weight_kg, created_at) VALUES (?, ?)", tt.existing, createdAt); err != nil {
				t.Fatal(err)
			}

			_, err := insertWeight(85.5)
			if tt.wantDup && err != errDuplicate {
				t.Errorf("expected errDuplicate, got %v", err)
			}
			if !tt.wantDup && err != nil {
				t.Errorf("expected reading to be accepted, got %v", err)
			}
		})
	}
}

// The scale treats 409 as "already stored" and stops retrying, so a repeat of
// the same reading must keep returning 409.
func TestPostWeightDuplicateReturns409(t *testing.T) {
	setupTestDB(t)
	setAPIKey(t, testAPIKey)

	for i, want := range []int{http.StatusCreated, http.StatusConflict} {
		req := httptest.NewRequest("POST", "/api/weight", bytes.NewBufferString(`{"weight_kg": 85.5}`))
		req.Header.Set("Authorization", "Bearer "+testAPIKey)
		w := httptest.NewRecorder()
		handlePostWeight(w, req)
		if w.Code != want {
			t.Fatalf("post %d: expected %d, got %d: %s", i+1, want, w.Code, w.Body.String())
		}
	}
}

func TestGetWeights(t *testing.T) {
	setupTestDB(t)

	// Insert a reading
	_, err := insertWeight(90.0)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/weights", nil)
	w := httptest.NewRecorder()
	handleGetWeights(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var results []Weight
	if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if results[0].WeightKg != 90.0 {
		t.Errorf("expected 90.0, got %f", results[0].WeightKg)
	}
}

// Rows just either side of the cutoff, in both stored formats: RFC 3339 (since
// March 2026) and the older "2006-01-02 15:04:05". Each pair shares the
// cutoff's date, which is where a plain text comparison goes wrong.
func TestGetWeightsDaysCutoff(t *testing.T) {
	setupTestDB(t)

	const days = 7
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	const legacy = "2006-01-02 15:04:05"
	rows := []struct {
		kg        float64
		createdAt string
		want      bool
	}{
		{70.1, cutoff.Add(time.Minute).Format(time.RFC3339), true},
		{70.2, cutoff.Add(-time.Minute).Format(time.RFC3339), false},
		{70.3, cutoff.Add(time.Minute).Format(legacy), true},
		{70.4, cutoff.Add(-time.Minute).Format(legacy), false},
	}
	for _, r := range rows {
		if _, err := db.Exec("INSERT INTO weights (weight_kg, created_at) VALUES (?, ?)", r.kg, r.createdAt); err != nil {
			t.Fatal(err)
		}
	}

	got, err := getWeights(days)
	if err != nil {
		t.Fatal(err)
	}
	returned := map[float64]bool{}
	for _, w := range got {
		returned[w.WeightKg] = true
	}
	for _, r := range rows {
		if returned[r.kg] != r.want {
			t.Errorf("%v kg at %s: returned=%v, want %v", r.kg, r.createdAt, returned[r.kg], r.want)
		}
	}
}

// The frontend takes weights[0] as newest, so order must be by time even when
// one day holds both formats.
func TestGetWeightsOrderMixedFormats(t *testing.T) {
	setupTestDB(t)

	for _, r := range []struct {
		kg        float64
		createdAt string
	}{
		{80.0, "2026-03-29 23:00:00"},  // legacy, latest
		{80.1, "2026-03-29T10:00:00Z"}, // RFC 3339, earlier the same day
	} {
		if _, err := db.Exec("INSERT INTO weights (weight_kg, created_at) VALUES (?, ?)", r.kg, r.createdAt); err != nil {
			t.Fatal(err)
		}
	}

	got, err := getWeights(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].WeightKg != 80.0 {
		t.Errorf("expected the 23:00 reading first, got %+v", got)
	}
}

func TestGetWeightsEmpty(t *testing.T) {
	setupTestDB(t)

	req := httptest.NewRequest("GET", "/api/weights", nil)
	w := httptest.NewRecorder()
	handleGetWeights(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Should return [] not null
	if w.Body.String() != "[]\n" {
		t.Errorf("expected empty array, got %q", w.Body.String())
	}
}

func TestGetWeightsWithDaysFilter(t *testing.T) {
	setupTestDB(t)

	_, err := insertWeight(80.0)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/weights?days=30", nil)
	w := httptest.NewRecorder()
	handleGetWeights(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var results []Weight
	if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}
