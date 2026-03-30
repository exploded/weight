package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
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

func TestPostWeight(t *testing.T) {
	setupTestDB(t)

	body := `{"weight_kg": 85.5}`
	req := httptest.NewRequest("POST", "/api/weight", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
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
			w := httptest.NewRecorder()
			handlePostWeight(w, req)
			if w.Code != tt.code {
				t.Errorf("expected %d, got %d", tt.code, w.Code)
			}
		})
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
