package store

import (
	"context"
	"database/sql"
	"time"

	"cc2api/internal/model"
)

type UsageStore struct {
	db     *sql.DB
	driver string
}

func NewUsageStore(db *sql.DB, driver string) *UsageStore {
	return &UsageStore{db: db, driver: driver}
}

func (s *UsageStore) fmtTime(t time.Time) any {
	if s.driver == "sqlite" {
		return timeVal(t)
	}
	return t
}

func (s *UsageStore) Insert(ctx context.Context, log *model.UsageLog) error {
	var ca timeScanner
	ca.target = &log.CreatedAt
	return s.db.QueryRowContext(ctx, `
		INSERT INTO usage_logs (account_id, model, input_tokens, output_tokens,
			cache_read_tokens, cache_creation_tokens, duration_ms)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id, created_at`,
		log.AccountID, log.Model, log.InputTokens, log.OutputTokens,
		log.CacheReadTokens, log.CacheCreationTokens, log.DurationMS,
	).Scan(&log.ID, &ca)
}

type UsageStats struct {
	AccountID          int64  `json:"account_id"`
	AccountName        string `json:"account_name"`
	TotalRequests      int    `json:"total_requests"`
	TotalInputTokens   int    `json:"total_input_tokens"`
	TotalOutputTokens  int    `json:"total_output_tokens"`
	TotalCacheRead     int    `json:"total_cache_read"`
	TotalCacheCreation int    `json:"total_cache_creation"`
}

func (s *UsageStore) StatsByAccount(ctx context.Context, since time.Time) ([]UsageStats, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.account_id, a.name,
			COUNT(*), SUM(u.input_tokens), SUM(u.output_tokens),
			SUM(u.cache_read_tokens), SUM(u.cache_creation_tokens)
		FROM usage_logs u JOIN accounts a ON u.account_id = a.id
		WHERE u.created_at >= $1
		GROUP BY u.account_id, a.name
		ORDER BY COUNT(*) DESC`, s.fmtTime(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []UsageStats
	for rows.Next() {
		var st UsageStats
		if err := rows.Scan(&st.AccountID, &st.AccountName,
			&st.TotalRequests, &st.TotalInputTokens, &st.TotalOutputTokens,
			&st.TotalCacheRead, &st.TotalCacheCreation); err != nil {
			return nil, err
		}
		stats = append(stats, st)
	}
	return stats, rows.Err()
}
