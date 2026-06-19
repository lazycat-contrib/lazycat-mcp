package app

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "github.com/lib-x/entsqlite"

	"lazycat-mcp/ent"
)

func openDB(ctx context.Context, dbPath string) (*ent.Client, error) {
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		return nil, fmt.Errorf("resolve db path: %w", err)
	}
	dbPath = absPath
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}
	u := url.URL{Scheme: "file", Path: dbPath}
	q := u.Query()
	q.Set("cache", "shared")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Add("_pragma", "busy_timeout(10000)")
	u.RawQuery = q.Encode()
	dsn := u.String()
	client, err := ent.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	if err := client.Schema.Create(ctx); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	return client, nil
}
