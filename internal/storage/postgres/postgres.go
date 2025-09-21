package postgres

import (
	"context"
	"fmt"
	"time"

	"where-is-my-PIZZA/internal/config"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewClient(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Database,
	)

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse pgx config: %w", err)
	}
	poolConfig.MaxConns = int32(cfg.Database.PoolMaxConns)

	var pool *pgxpool.Pool
	// Retry connecting to the database
	for i := 0; i < 5; i++ {
		// Use a timeout for each connection attempt
		connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		pool, err = pgxpool.NewWithConfig(connectCtx, poolConfig)
		cancel()
		if err == nil {
			pingCtx, pingCancel := context.WithTimeout(ctx, 2*time.Second)
			err = pool.Ping(pingCtx)
			pingCancel()
			if err == nil {
				return pool, nil
			}
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}

	return nil, fmt.Errorf("failed to connect to postgres after several retries: %w", err)
}
