package store

import (
	"context"
	"time"
)

// CacheStore abstracts Redis/Memory for session and concurrency operations.
type CacheStore interface {
	GetSessionAccountID(ctx context.Context, sessionHash string) (int64, error)
	SetSessionAccountID(ctx context.Context, sessionHash string, accountID int64, ttl time.Duration) error
	DeleteSession(ctx context.Context, sessionHash string) error
	AcquireSlot(ctx context.Context, key string, max int, ttl time.Duration) (bool, error)
	ReleaseSlot(ctx context.Context, key string)
	Close() error
}

// Compile-time interface checks
var (
	_ CacheStore = (*RedisStore)(nil)
	_ CacheStore = (*MemoryStore)(nil)
)
