package store

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisStore struct {
	client *redis.Client
}

func NewRedisStore(addr, password string, db int) (*RedisStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return &RedisStore{client: client}, nil
}

// Sticky session

func (s *RedisStore) GetSessionAccountID(ctx context.Context, sessionHash string) (int64, error) {
	val, err := s.client.Get(ctx, "session:"+sessionHash).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(val, 10, 64)
}

func (s *RedisStore) SetSessionAccountID(ctx context.Context, sessionHash string, accountID int64, ttl time.Duration) error {
	return s.client.Set(ctx, "session:"+sessionHash, strconv.FormatInt(accountID, 10), ttl).Err()
}

func (s *RedisStore) DeleteSession(ctx context.Context, sessionHash string) error {
	return s.client.Del(ctx, "session:"+sessionHash).Err()
}

// Concurrency slots

func (s *RedisStore) AcquireSlot(ctx context.Context, key string, max int, ttl time.Duration) (bool, error) {
	val, err := s.client.Incr(ctx, key).Result()
	if err != nil {
		return false, err
	}
	if val == 1 {
		s.client.Expire(ctx, key, ttl)
	}
	if val > int64(max) {
		s.client.Decr(ctx, key)
		return false, nil
	}
	return true, nil
}

func (s *RedisStore) ReleaseSlot(ctx context.Context, key string) {
	s.client.Decr(ctx, key)
}

func (s *RedisStore) Close() error {
	return s.client.Close()
}
