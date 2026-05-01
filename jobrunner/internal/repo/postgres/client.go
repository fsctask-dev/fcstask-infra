package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Client struct {
	Pool *pgxpool.Pool
}

type Config struct {
	DSN         string
	Host        string
	Port        int
	User        string
	Password    string
	DBName      string
	MaxConns    int32
	ConnTimeout time.Duration
}

func NewClient(ctx context.Context, cfg *Config) (*Client, error) {
	dsn := cfg.DSN
	if dsn == "" {
		dsn = fmt.Sprintf("postgres://%s:%s@%s:%d/%s", cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.DBName)
	}

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse DSN: %w", err)
	}

	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.ConnConfig.ConnectTimeout = cfg.ConnTimeout

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Client{
		Pool: pool,
	}, nil
}

func (c *Client) Close() {
    if c.Pool != nil {
        c.Pool.Close()
    }
}

func (c *Client) Ping(ctx context.Context) error {
    return c.Pool.Ping(ctx)
}