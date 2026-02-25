package postgres

import (
	"context"
	"database/sql"
	"log"
	"net/url"
	"os"
	"strings"

	// Import the PostgreSQL driver.
	_ "github.com/lib/pq"
	"github.com/pkg/errors"

	"github.com/usememos/memos/internal/profile"
	"github.com/usememos/memos/store"
)

type DB struct {
	db      *sql.DB
	profile *profile.Profile
}

func NewDB(profile *profile.Profile) (store.Driver, error) {
	if profile == nil {
		return nil, errors.New("profile is nil")
	}
	dsn, err := ensureSupabaseSSLMode(profile.DSN)
	if err != nil {
		return nil, err
	}

	// Open the PostgreSQL connection
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Printf("Failed to open database: %s", err)
		return nil, errors.Wrapf(err, "failed to open database: %s", dsn)
	}

	var driver store.Driver = &DB{
		db:      db,
		profile: profile,
	}

	// Return the DB struct
	return driver, nil
}

func (d *DB) GetDB() *sql.DB {
	return d.db
}

func (d *DB) Close() error {
	return d.db.Close()
}

func (d *DB) IsInitialized(ctx context.Context) (bool, error) {
	var exists bool
	err := d.db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_catalog = current_database() AND table_name = 'memo' AND table_type = 'BASE TABLE')").Scan(&exists)
	if err != nil {
		return false, errors.Wrap(err, "failed to check if database is initialized")
	}
	return exists, nil
}

func ensureSupabaseSSLMode(dsn string) (string, error) {
	// Supabase runtime should use SSL when MEMOS_SUPABASE_PROJECT_URL is configured.
	if strings.TrimSpace(os.Getenv("MEMOS_SUPABASE_PROJECT_URL")) == "" {
		return dsn, nil
	}

	trimmed := strings.TrimSpace(dsn)
	if trimmed == "" {
		return "", errors.New("postgres dsn is required")
	}

	parsedURL, err := url.Parse(trimmed)
	if err != nil || parsedURL.Host == "" {
		if strings.Contains(strings.ToLower(trimmed), "sslmode=require") {
			return dsn, nil
		}
		return "", errors.New("supabase postgres dsn must include sslmode=require")
	}

	query := parsedURL.Query()
	sslMode := strings.ToLower(strings.TrimSpace(query.Get("sslmode")))
	if sslMode == "" {
		query.Set("sslmode", "require")
		parsedURL.RawQuery = query.Encode()
		return parsedURL.String(), nil
	}
	if sslMode != "require" {
		return "", errors.New("supabase postgres dsn must use sslmode=require")
	}
	return dsn, nil
}
