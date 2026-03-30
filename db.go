package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "modernc.org/sqlite"
)

var db *sql.DB

func initDB(path string) error {
	var err error
	db, err = sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	// Enable WAL mode for better concurrent read performance
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return fmt.Errorf("set WAL mode: %w", err)
	}

	// Create table if it doesn't exist
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS weights (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			weight_kg REAL NOT NULL,
			created_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)
	`)
	if err != nil {
		return fmt.Errorf("create table: %w", err)
	}

	return nil
}

func closeDB() {
	if db != nil {
		db.Close()
	}
}

type Weight struct {
	ID        int64   `json:"id"`
	WeightKg  float64 `json:"weight_kg"`
	CreatedAt string  `json:"created_at"`
}

func insertWeight(weightKg float64) (Weight, error) {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	result, err := db.Exec(
		"INSERT INTO weights (weight_kg, created_at) VALUES (?, ?)",
		weightKg, now,
	)
	if err != nil {
		return Weight{}, fmt.Errorf("insert weight: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return Weight{}, fmt.Errorf("get last insert id: %w", err)
	}

	return Weight{
		ID:        id,
		WeightKg:  weightKg,
		CreatedAt: now,
	}, nil
}

func getWeights(days int) ([]Weight, error) {
	var rows *sql.Rows
	var err error

	if days > 0 {
		rows, err = db.Query(
			"SELECT id, weight_kg, created_at FROM weights WHERE created_at >= datetime('now', ?) ORDER BY created_at DESC",
			fmt.Sprintf("-%d days", days),
		)
	} else {
		rows, err = db.Query(
			"SELECT id, weight_kg, created_at FROM weights ORDER BY created_at DESC",
		)
	}
	if err != nil {
		return nil, fmt.Errorf("query weights: %w", err)
	}
	defer rows.Close()

	var weights []Weight
	for rows.Next() {
		var w Weight
		if err := rows.Scan(&w.ID, &w.WeightKg, &w.CreatedAt); err != nil {
			log.Printf("scan weight row: %v", err)
			continue
		}
		weights = append(weights, w)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate weights: %w", err)
	}

	// Return empty array instead of null in JSON
	if weights == nil {
		weights = []Weight{}
	}

	return weights, nil
}
