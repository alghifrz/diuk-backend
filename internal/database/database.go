// Package database owns the PostgreSQL connection pool used by the backend.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool defaults. Supabase enforces its own connection limits, so the pool is
// kept small enough to stay well below them with a few backend replicas.
const (
	maxConns          = 10
	minConns          = 2
	maxConnLifetime   = time.Hour
	maxConnIdleTime   = 30 * time.Minute
	healthCheckPeriod = time.Minute
	connectTimeout    = 10 * time.Second
	pingTimeout       = 5 * time.Second
)

// DB is the application's handle on PostgreSQL.
type DB struct {
	pool *pgxpool.Pool
}

// PoolSettings overrides the default pool bounds. Zero values keep the
// production-safe defaults.
type PoolSettings struct {
	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
}

// Connect builds the connection pool from a PostgreSQL connection string and
// verifies that the database is reachable before returning.
func Connect(ctx context.Context, databaseURL string, settings ...PoolSettings) (*DB, error) {
	poolCfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	s := PoolSettings{}
	if len(settings) > 0 {
		s = settings[0]
	}
	poolCfg.MaxConns = firstPositive32(s.MaxConns, maxConns)
	poolCfg.MinConns = firstPositive32(s.MinConns, minConns)
	if poolCfg.MinConns > poolCfg.MaxConns {
		poolCfg.MinConns = poolCfg.MaxConns
	}
	poolCfg.MaxConnLifetime = firstPositiveDuration(s.MaxConnLifetime, maxConnLifetime)
	poolCfg.MaxConnIdleTime = firstPositiveDuration(s.MaxConnIdleTime, maxConnIdleTime)
	poolCfg.HealthCheckPeriod = firstPositiveDuration(s.HealthCheckPeriod, healthCheckPeriod)
	poolCfg.ConnConfig.ConnectTimeout = connectTimeout

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	db := &DB{pool: pool}
	if err := db.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return db, nil
}

// Pool exposes the underlying pgx pool for repositories.
func (db *DB) Pool() *pgxpool.Pool {
	return db.pool
}

// Ping verifies that the database answers within a short timeout.
func (db *DB) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	if err := db.pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	return nil
}

// Close releases every connection in the pool.
func (db *DB) Close() {
	db.pool.Close()
}

func firstPositive32(value, fallback int32) int32 {
	if value > 0 {
		return value
	}
	return fallback
}

func firstPositiveDuration(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}
