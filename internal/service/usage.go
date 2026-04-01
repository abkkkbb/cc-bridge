package service

import (
	"context"
	"time"

	"cc2api/internal/model"
	"cc2api/internal/store"
)

type UsageService struct {
	store *store.UsageStore
}

func NewUsageService(s *store.UsageStore) *UsageService {
	return &UsageService{store: s}
}

func (s *UsageService) Record(ctx context.Context, log *model.UsageLog) {
	// Fire and forget
	go func() {
		ctx2, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.store.Insert(ctx2, log)
	}()
}

func (s *UsageService) StatsByAccount(ctx context.Context, since time.Time) ([]store.UsageStats, error) {
	return s.store.StatsByAccount(ctx, since)
}
