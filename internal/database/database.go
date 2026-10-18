// Package database opens the shared PostgreSQL schema (owned by the Ace
// migrations in kaderisasi-admin-be).
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"kaderisasi/web/internal/config"
)

func Open(ctx context.Context, c config.Config) (*pgxpool.Pool, error) {
	conf, err := pgxpool.ParseConfig("")
	if err != nil {
		return nil, err
	}
	conf.ConnConfig.Host, conf.ConnConfig.Port = c.DBHost, c.DBPort
	conf.ConnConfig.User, conf.ConnConfig.Password, conf.ConnConfig.Database = c.DBUser, c.DBPassword, c.DBName
	conf.ConnConfig.RuntimeParams["search_path"] = c.DBSchema
	conf.ConnConfig.RuntimeParams["application_name"] = "kaderisasi-web-go"
	conf.MaxConns, conf.MinConns = 10, 2
	conf.ConnConfig.ConnectTimeout = 30 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, conf)
	if err != nil {
		return nil, err
	}
	var schema string
	if err = pool.QueryRow(ctx, "select current_schema()").Scan(&schema); err != nil || schema != c.DBSchema {
		pool.Close()
		return nil, fmt.Errorf("database schema unavailable: %s", c.DBSchema)
	}
	return pool, nil
}
