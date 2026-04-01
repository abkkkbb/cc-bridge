package model

import "time"

type UsageLog struct {
	ID                  int64     `json:"id" db:"id"`
	AccountID           int64     `json:"account_id" db:"account_id"`
	Model               string    `json:"model" db:"model"`
	InputTokens         int       `json:"input_tokens" db:"input_tokens"`
	OutputTokens        int       `json:"output_tokens" db:"output_tokens"`
	CacheReadTokens     int       `json:"cache_read_tokens" db:"cache_read_tokens"`
	CacheCreationTokens int       `json:"cache_creation_tokens" db:"cache_creation_tokens"`
	DurationMS          int       `json:"duration_ms" db:"duration_ms"`
	CreatedAt           time.Time `json:"created_at" db:"created_at"`
}
