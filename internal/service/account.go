package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	mrand "math/rand/v2"
	"strings"
	"time"

	"cc2api/internal/model"
	"cc2api/internal/store"
)

const stickySessionTTL = 24 * time.Hour

type AccountService struct {
	store *store.AccountStore
	cache store.CacheStore
}

func NewAccountService(s *store.AccountStore, c store.CacheStore) *AccountService {
	return &AccountService{store: s, cache: c}
}

// CreateAccount creates a new account with auto-generated identity.
func (s *AccountService) CreateAccount(ctx context.Context, a *model.Account) error {
	deviceID, env, prompt, process := model.GenerateCanonicalIdentity()
	a.DeviceID = deviceID
	a.CanonicalEnv = env
	a.CanonicalPrompt = prompt
	a.CanonicalProcess = process
	if a.Status == "" {
		a.Status = model.AccountStatusActive
	}
	if a.Concurrency == 0 {
		a.Concurrency = 3
	}
	if a.Priority == 0 {
		a.Priority = 50
	}
	return s.store.Create(ctx, a)
}

func (s *AccountService) UpdateAccount(ctx context.Context, a *model.Account) error {
	return s.store.Update(ctx, a)
}

func (s *AccountService) DeleteAccount(ctx context.Context, id int64) error {
	return s.store.Delete(ctx, id)
}

func (s *AccountService) GetAccount(ctx context.Context, id int64) (*model.Account, error) {
	return s.store.GetByID(ctx, id)
}

func (s *AccountService) ListAccounts(ctx context.Context) ([]*model.Account, error) {
	return s.store.List(ctx)
}

// SelectAccount picks an account for a request using sticky session.
func (s *AccountService) SelectAccount(ctx context.Context, sessionHash string, excludeIDs []int64) (*model.Account, error) {
	// Check sticky session
	if sessionHash != "" {
		accountID, err := s.cache.GetSessionAccountID(ctx, sessionHash)
		if err == nil && accountID > 0 {
			account, err := s.store.GetByID(ctx, accountID)
			if err == nil && account.IsSchedulable() && !contains(excludeIDs, accountID) {
				return account, nil
			}
			// Stale binding, delete
			_ = s.cache.DeleteSession(ctx, sessionHash)
		}
	}

	// Get schedulable accounts
	accounts, err := s.store.ListSchedulable(ctx)
	if err != nil {
		return nil, err
	}

	// Filter excluded
	var candidates []*model.Account
	for _, a := range accounts {
		if !contains(excludeIDs, a.ID) {
			candidates = append(candidates, a)
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no available accounts")
	}

	// Group by priority, shuffle within same priority
	selected := selectByPriority(candidates)

	// Bind sticky session
	if sessionHash != "" {
		_ = s.cache.SetSessionAccountID(ctx, sessionHash, selected.ID, stickySessionTTL)
	}

	return selected, nil
}

// AcquireSlot tries to acquire a concurrency slot for the account.
func (s *AccountService) AcquireSlot(ctx context.Context, accountID int64, max int) (bool, error) {
	key := fmt.Sprintf("concurrency:account:%d", accountID)
	return s.cache.AcquireSlot(ctx, key, max, 5*time.Minute)
}

// ReleaseSlot releases a concurrency slot.
func (s *AccountService) ReleaseSlot(ctx context.Context, accountID int64) {
	key := fmt.Sprintf("concurrency:account:%d", accountID)
	s.cache.ReleaseSlot(ctx, key)
}

func (s *AccountService) SetRateLimit(ctx context.Context, id int64, resetAt time.Time) error {
	return s.store.SetRateLimit(ctx, id, resetAt)
}

// GenerateSessionHash creates a session hash based on client type.
// For CC clients: uses session_id from metadata.user_id.
// For API clients: uses sha256(UA + system_or_first_msg + hour_window).
func GenerateSessionHash(userAgent string, body map[string]any, clientType ClientType) string {
	if clientType == ClientTypeClaudeCode {
		if metadata, ok := body["metadata"].(map[string]any); ok {
			if userIDStr, ok := metadata["user_id"].(string); ok {
				var uid map[string]any
				if err := json.Unmarshal([]byte(userIDStr), &uid); err == nil {
					if sid, ok := uid["session_id"].(string); ok && sid != "" {
						return sid
					}
				}
				// Legacy format
				if idx := strings.LastIndex(userIDStr, "_session_"); idx >= 0 {
					return userIDStr[idx+9:]
				}
			}
		}
	}

	// API mode: UA + system/first_msg + hour window
	var content string

	// Try system prompt first
	switch sys := body["system"].(type) {
	case string:
		content = sys
	case []any:
		for _, item := range sys {
			if block, ok := item.(map[string]any); ok {
				if text, ok := block["text"].(string); ok {
					content = text
					break
				}
			}
		}
	}

	// Fallback to first message
	if content == "" {
		if messages, ok := body["messages"].([]any); ok && len(messages) > 0 {
			if msg, ok := messages[0].(map[string]any); ok {
				switch c := msg["content"].(type) {
				case string:
					content = c
				case []any:
					for _, item := range c {
						if block, ok := item.(map[string]any); ok {
							if text, ok := block["text"].(string); ok {
								content = text
								break
							}
						}
					}
				}
			}
		}
	}

	hourWindow := time.Now().UTC().Format("2006-01-02T15")
	raw := userAgent + "|" + content + "|" + hourWindow
	h := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", h[:16])
}

func selectByPriority(accounts []*model.Account) *model.Account {
	if len(accounts) == 1 {
		return accounts[0]
	}

	// Find highest priority (lowest number)
	bestPriority := accounts[0].Priority
	for _, a := range accounts[1:] {
		if a.Priority < bestPriority {
			bestPriority = a.Priority
		}
	}

	// Collect all with best priority
	var best []*model.Account
	for _, a := range accounts {
		if a.Priority == bestPriority {
			best = append(best, a)
		}
	}

	// Random pick within same priority
	return best[mrand.IntN(len(best))]
}

func contains(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}
