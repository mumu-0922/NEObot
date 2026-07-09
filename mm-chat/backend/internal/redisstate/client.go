package redisstate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"neo-chat/mm-chat/backend/internal/config"
)

const defaultKeyPrefix = "mm-chat"

type Client struct {
	rdb       *redis.Client
	keyPrefix string
}

func Open(ctx context.Context, cfg config.RedisConfig) (*Client, error) {
	redisURL := strings.TrimSpace(cfg.URL)
	if redisURL == "" {
		return nil, nil
	}

	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, errors.New("parse REDIS_URL")
	}
	rdb := redis.NewClient(options)
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return &Client{
		rdb:       rdb,
		keyPrefix: normalizeKeyPrefix(cfg.KeyPrefix),
	}, nil
}

func (c *Client) Close() error {
	if c == nil || c.rdb == nil {
		return nil
	}

	return c.rdb.Close()
}

func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.rdb == nil {
		return nil
	}

	return c.rdb.Ping(ctx).Err()
}

func (c *Client) CheckReady(ctx context.Context) error {
	return c.Ping(ctx)
}

func (c *Client) RunCancellationStore(ttl time.Duration) *RunCancellationStore {
	if c == nil || c.rdb == nil {
		return nil
	}
	if ttl <= 0 {
		ttl = config.DefaultRedisRunCancelTTL
	}

	return &RunCancellationStore{
		rdb:       c.rdb,
		keyPrefix: c.keyPrefix,
		ttl:       ttl,
	}
}

func normalizeKeyPrefix(prefix string) string {
	prefix = strings.Trim(strings.TrimSpace(prefix), ":")
	if prefix == "" {
		return defaultKeyPrefix
	}

	return prefix
}
