package app

import (
	"context"
	"database/sql"
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
	if err := ensureLegacySchemaColumns(ctx, dsn); err != nil {
		return nil, err
	}
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

func ensureLegacySchemaColumns(ctx context.Context, dsn string) error {
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return fmt.Errorf("open sqlite database for migration: %w", err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations_guard (id integer primary key)`); err != nil {
		return fmt.Errorf("verify sqlite migration connection: %w", err)
	}
	_, _ = db.ExecContext(ctx, `DROP TABLE IF EXISTS schema_migrations_guard`)
	hasProviderTable, err := sqliteTableExists(ctx, db, "upstream_providers")
	if err != nil {
		return err
	}
	if !hasProviderTable {
		return nil
	}
	hasOwnerColumn, err := sqliteColumnExists(ctx, db, "upstream_providers", "owner_user_id")
	if err != nil {
		return err
	}
	if !hasOwnerColumn {
		if _, err := db.ExecContext(ctx, `ALTER TABLE upstream_providers ADD COLUMN owner_user_id text NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add upstream_providers.owner_user_id: %w", err)
		}
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS upstreamprovider_owner_user_id ON upstream_providers(owner_user_id)`); err != nil {
		return fmt.Errorf("create upstreamprovider_owner_user_id index: %w", err)
	}
	if err := ensureLegacyMCPTokenColumns(ctx, db); err != nil {
		return err
	}
	return nil
}

func ensureLegacyMCPTokenColumns(ctx context.Context, db *sql.DB) error {
	hasTokenTable, err := sqliteTableExists(ctx, db, "mcp_tokens")
	if err != nil {
		return err
	}
	if !hasTokenTable {
		return nil
	}
	hasOwnerColumn, err := sqliteColumnExists(ctx, db, "mcp_tokens", "owner_user_id")
	if err != nil {
		return err
	}
	if !hasOwnerColumn {
		if _, err := db.ExecContext(ctx, `ALTER TABLE mcp_tokens ADD COLUMN owner_user_id text NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add mcp_tokens.owner_user_id: %w", err)
		}
	}
	hasAdminColumn, err := sqliteColumnExists(ctx, db, "mcp_tokens", "owner_is_admin")
	if err != nil {
		return err
	}
	if !hasAdminColumn {
		if _, err := db.ExecContext(ctx, `ALTER TABLE mcp_tokens ADD COLUMN owner_is_admin bool NOT NULL DEFAULT false`); err != nil {
			return fmt.Errorf("add mcp_tokens.owner_is_admin: %w", err)
		}
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS mcptoken_owner_user_id ON mcp_tokens(owner_user_id)`); err != nil {
		return fmt.Errorf("create mcptoken_owner_user_id index: %w", err)
	}
	return nil
}

func sqliteTableExists(ctx context.Context, db *sql.DB, table string) (bool, error) {
	var name string
	err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect sqlite table %s existence: %w", table, err)
	}
	return true, nil
}

func sqliteColumnExists(ctx context.Context, db *sql.DB, table, column string) (bool, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return false, fmt.Errorf("inspect sqlite table %s: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return false, fmt.Errorf("scan sqlite table %s info: %w", table, err)
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate sqlite table %s info: %w", table, err)
	}
	return false, nil
}
