package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Connect Postgres'ga ulanadi va tayyor bo'lguncha kutadi (retry bilan).
func Connect(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	// Postgres konteyneri ko'tarilguncha bir necha marta urinish.
	var pingErr error
	for i := 0; i < 30; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		pingErr = db.PingContext(ctx)
		cancel()
		if pingErr == nil {
			return db, Migrate(db)
		}
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("postgresga ulanib bo'lmadi: %w", pingErr)
}

// Migrate kerakli jadvallarni yaratadi.
func Migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS interactions (
			id              BIGSERIAL PRIMARY KEY,
			created         TIMESTAMPTZ NOT NULL DEFAULT now(),
			conversation_id BIGINT,
			client_id       BIGINT,
			client_name     TEXT,
			title           TEXT,
			client_message  TEXT,
			ai_reply        TEXT,
			sent            BOOLEAN NOT NULL DEFAULT false
		)`,
		`CREATE TABLE IF NOT EXISTS escalations (
			tg_message_id   BIGINT PRIMARY KEY,
			conversation_id BIGINT,
			client_name     TEXT,
			question        TEXT,
			created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
			resolved        BOOLEAN NOT NULL DEFAULT false,
			answer          TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS kv (
			key   TEXT PRIMARY KEY,
			value TEXT
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("migratsiya: %w", err)
		}
	}
	return nil
}

// SetKV kalit-qiymatni saqlaydi.
func SetKV(db *sql.DB, key, value string) error {
	_, err := db.Exec(
		`INSERT INTO kv(key, value) VALUES($1,$2)
		 ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value`, key, value)
	return err
}

// GetKV kalit qiymatini o'qiydi (bo'lmasa "" va nil).
func GetKV(db *sql.DB, key string) (string, error) {
	var v string
	err := db.QueryRow(`SELECT value FROM kv WHERE key=$1`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}
