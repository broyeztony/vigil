package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/viper"
)

var Pool *pgxpool.Pool

func Init(ctx context.Context) error {
	connString := viper.GetString("database.url")
	if connString == "" {
		return fmt.Errorf("database.url not configured")
	}

	var err error
	Pool, err = pgxpool.New(ctx, connString)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Test connection
	if err := Pool.Ping(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	return nil
}

func Close() {
	if Pool != nil {
		Pool.Close()
	}
}

