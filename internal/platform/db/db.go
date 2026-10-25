package db

import (
	"context"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	pgxvec "github.com/pgvector/pgvector-go/pgx"
)

type Querier interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Open requires the vector extension to have been installed by migrations.
func Open(ctx context.Context, url string, maxConns int) (*pgxpool.Pool, error) {
	if maxConns <= 0 || maxConns > math.MaxInt32 {
		return nil, fmt.Errorf("database max_conns must be between 1 and %d", math.MaxInt32)
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database configuration: %w", err)
	}
	cfg.MaxConns = int32(maxConns)
	cfg.AfterConnect = pgxvec.RegisterTypes
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect database: %w", err)
	}
	return pool, nil
}
