package redisq

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type Config struct {
	URL      string
	Password string
	DB       int
	Timeout  time.Duration
}

type Client struct {
	rdb *redis.Client
}

func Connect(ctx context.Context, cfg Config) (*Client, error) {
	opts, err := parseOptions(cfg)
	if err != nil {
		return nil, err
	}
	rdb := redis.NewClient(opts)
	pingCtx, cancel := context.WithTimeout(ctx, timeoutOr(cfg.Timeout, 5*time.Second))
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return &Client{rdb: rdb}, nil
}

func (c *Client) Raw() *redis.Client { return c.rdb }

func (c *Client) Ping(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

func (c *Client) Close() error {
	return c.rdb.Close()
}

func parseOptions(cfg Config) (*redis.Options, error) {
	url := strings.TrimSpace(cfg.URL)
	if url == "" {
		return nil, fmt.Errorf("redis url is required")
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		host := url
		if !strings.Contains(host, ":") {
			host = net.JoinHostPort(host, "6379")
		}
		opts = &redis.Options{Addr: host}
	}
	if pw := strings.TrimSpace(cfg.Password); pw != "" {
		opts.Password = pw
	}
	if cfg.DB > 0 {
		opts.DB = cfg.DB
	}
	timeout := timeoutOr(cfg.Timeout, 5*time.Second)
	opts.DialTimeout = timeout
	opts.ReadTimeout = timeout
	opts.WriteTimeout = timeout
	if opts.TLSConfig != nil {
		opts.TLSConfig.MinVersion = tls.VersionTLS12
	}
	return opts, nil
}

func timeoutOr(value, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return value
}
