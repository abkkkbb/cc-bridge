package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestSQLiteTimeRoundTrip(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create a minimal table
	db.Exec(`CREATE TABLE test_time (
		id INTEGER PRIMARY KEY,
		ts TEXT NOT NULL,
		ts_null TEXT
	)`)

	now := time.Now().UTC().Truncate(time.Second)
	tv := timeVal(now)

	// Insert with timeVal
	_, err = db.Exec(`INSERT INTO test_time (id, ts, ts_null) VALUES (1, $1, $2)`, tv, timeVal(now))
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Read back with timeScanner
	var got time.Time
	var gotNull *time.Time
	sc := &timeScanner{target: &got}
	nsc := &nullTimeScanner{target: &gotNull}
	err = db.QueryRow(`SELECT ts, ts_null FROM test_time WHERE id=1`).Scan(sc, nsc)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if !got.Equal(now) {
		t.Errorf("time mismatch: wrote %v, got %v", now, got)
	}
	if gotNull == nil || !gotNull.Equal(now) {
		t.Errorf("nullable time mismatch: wrote %v, got %v", now, gotNull)
	}

	// Test NULL scan
	db.Exec(`INSERT INTO test_time (id, ts, ts_null) VALUES (2, $1, NULL)`, tv)
	var got2 time.Time
	var gotNull2 *time.Time
	sc2 := &timeScanner{target: &got2}
	nsc2 := &nullTimeScanner{target: &gotNull2}
	err = db.QueryRow(`SELECT ts, ts_null FROM test_time WHERE id=2`).Scan(sc2, nsc2)
	if err != nil {
		t.Fatalf("scan null: %v", err)
	}
	if gotNull2 != nil {
		t.Errorf("expected nil, got %v", gotNull2)
	}

	// Test strftime default matches
	db.Exec(`CREATE TABLE test_default (
		id INTEGER PRIMARY KEY,
		ts TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
	)`)
	db.Exec(`INSERT INTO test_default (id) VALUES (1)`)
	var gotDefault time.Time
	scDefault := &timeScanner{target: &gotDefault}
	err = db.QueryRow(`SELECT ts FROM test_default WHERE id=1`).Scan(scDefault)
	if err != nil {
		t.Fatalf("scan default: %v", err)
	}
	if gotDefault.IsZero() {
		t.Error("default time is zero")
	}
	t.Logf("default time parsed OK: %v", gotDefault)

	// Test comparison works (critical for WHERE clauses)
	past := now.Add(-1 * time.Hour)
	future := now.Add(1 * time.Hour)
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM test_time WHERE ts >= $1`, timeVal(past)).Scan(&count)
	if err != nil {
		t.Fatalf("comparison query: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 rows >= past, got %d", count)
	}
	err = db.QueryRow(`SELECT COUNT(*) FROM test_time WHERE ts >= $1`, timeVal(future)).Scan(&count)
	if err != nil {
		t.Fatalf("comparison query: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 rows >= future, got %d", count)
	}
}

func TestAccountStoreRateLimit(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := Migrate(db, "sqlite"); err != nil {
		t.Fatal(err)
	}

	s := NewAccountStore(db, "sqlite")
	ctx := context.Background()

	// Create an account
	db.Exec(`INSERT INTO accounts (name, email, status, token, proxy_url, device_id, canonical_env, canonical_prompt_env, canonical_process, concurrency, priority)
		VALUES ('test', 'test@test.com', 'active', 'tok', '', 'dev1', '{}', '{}', '{}', 3, 50)`)

	// Set rate limit
	resetAt := time.Now().Add(1 * time.Minute).UTC().Truncate(time.Second)
	err = s.SetRateLimit(ctx, 1, resetAt)
	if err != nil {
		t.Fatalf("SetRateLimit: %v", err)
	}

	// Read back
	a, err := s.GetByID(ctx, 1)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if a.RateLimitedAt == nil {
		t.Fatal("rate_limited_at is nil")
	}
	if a.RateLimitResetAt == nil {
		t.Fatal("rate_limit_reset_at is nil")
	}
	if !a.RateLimitResetAt.Truncate(time.Second).Equal(resetAt) {
		t.Errorf("reset_at mismatch: want %v, got %v", resetAt, *a.RateLimitResetAt)
	}

	// Clear and verify
	err = s.ClearRateLimit(ctx, 1)
	if err != nil {
		t.Fatalf("ClearRateLimit: %v", err)
	}
	a, _ = s.GetByID(ctx, 1)
	if a.RateLimitedAt != nil {
		t.Error("rate_limited_at should be nil after clear")
	}

	// Test ListSchedulable
	accounts, err := s.ListSchedulable(ctx)
	if err != nil {
		t.Fatalf("ListSchedulable: %v", err)
	}
	if len(accounts) != 1 {
		t.Errorf("expected 1 schedulable, got %d", len(accounts))
	}
}
