package store

import (
	"context"
	"sync"
	"time"
)

// MemoryStore is an in-memory replacement for Redis when Redis is not available.
type MemoryStore struct {
	mu       sync.Mutex
	sessions map[string]sessionEntry
	slots    map[string]int64
}

type sessionEntry struct {
	accountID int64
	expiresAt time.Time
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		sessions: make(map[string]sessionEntry),
		slots:    make(map[string]int64),
	}
}

func (s *MemoryStore) GetSessionAccountID(_ context.Context, sessionHash string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.sessions["session:"+sessionHash]
	if !ok || time.Now().After(entry.expiresAt) {
		delete(s.sessions, "session:"+sessionHash)
		return 0, nil
	}
	return entry.accountID, nil
}

func (s *MemoryStore) SetSessionAccountID(_ context.Context, sessionHash string, accountID int64, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions["session:"+sessionHash] = sessionEntry{accountID: accountID, expiresAt: time.Now().Add(ttl)}
	return nil
}

func (s *MemoryStore) DeleteSession(_ context.Context, sessionHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, "session:"+sessionHash)
	return nil
}

func (s *MemoryStore) AcquireSlot(_ context.Context, key string, max int, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	val := s.slots[key] + 1
	if val > int64(max) {
		return false, nil
	}
	s.slots[key] = val
	return true, nil
}

func (s *MemoryStore) ReleaseSlot(_ context.Context, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.slots[key] > 0 {
		s.slots[key]--
	}
}

func (s *MemoryStore) Close() error { return nil }
