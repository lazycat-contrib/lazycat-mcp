package app

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestProviderServiceCreatesCustomProvider(t *testing.T) {
	ctx := context.Background()
	db, err := openDB(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	service := NewProviderService(db)
	provider, err := service.Create(ctx, ProviderInput{
		Type:        "custom",
		Name:        "Context7",
		Description: "Documentation MCP",
		Slug:        "context7",
		BaseURL:     "https://example.com/",
		Endpoint:    "/mcp",
		Transport:   "streamable_http",
		Headers: []ProviderHeader{
			{Name: "Authorization", Value: "Bearer secret"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.Type != "custom" {
		t.Fatalf("type = %q", provider.Type)
	}
	if provider.AppID != "" {
		t.Fatalf("app id = %q", provider.AppID)
	}
	if provider.BaseURL != "https://example.com" {
		t.Fatalf("base url = %q", provider.BaseURL)
	}
	if provider.PublicEndpoint != "/mcp/apps/context7" {
		t.Fatalf("public endpoint = %q", provider.PublicEndpoint)
	}
	if provider.HeaderCount != 1 || len(provider.HeaderNames) != 1 || provider.HeaderNames[0] != "Authorization" {
		t.Fatalf("headers = %#v count=%d", provider.HeaderNames, provider.HeaderCount)
	}
}

func TestProviderServicePublicListDoesNotExposeUpstreamDetails(t *testing.T) {
	ctx := context.Background()
	db, err := openDB(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	service := NewProviderService(db)
	if _, err := service.Create(ctx, ProviderInput{
		Type:      "custom",
		Name:      "Context7",
		Slug:      "context7",
		BaseURL:   "https://secret.example.com",
		Endpoint:  "/mcp",
		Transport: "sse",
		Headers: []ProviderHeader{
			{Name: "Authorization", Value: "Bearer secret"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	providers, err := service.EnabledPublic(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 1 {
		t.Fatalf("providers = %d", len(providers))
	}
	got := providers[0]
	if got.Name != "Context7" || got.Endpoint != "/mcp/apps/context7" || got.Transport != "sse" {
		t.Fatalf("public provider = %#v", got)
	}
}

func TestProviderServiceRejectsLazyCatProviderWithoutAppID(t *testing.T) {
	ctx := context.Background()
	db, err := openDB(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	service := NewProviderService(db)
	if _, err := service.Create(ctx, ProviderInput{
		Type:      "lazycat",
		Name:      "Broken",
		Slug:      "broken",
		Endpoint:  "/mcp",
		Transport: "streamable_http",
	}); err == nil {
		t.Fatal("expected lazycat provider without app id to fail")
	}
}

func TestProviderServiceClaimsLegacyOwnerlessProvider(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	raw, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = raw.Exec(`CREATE TABLE upstream_providers (
		id integer primary key autoincrement,
		name text not null,
		description text,
		slug text not null unique,
		type text not null default 'lazycat',
		app_id text not null default '',
		deploy_id text,
		app_title text,
		resource_id text,
		base_url text,
		endpoint text not null default '/mcp',
		headers text not null default '[]',
		transport text not null default 'streamable_http',
		enabled bool not null default true,
		last_used_at datetime,
		created_at datetime not null,
		updated_at datetime not null
	)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = raw.Exec(`INSERT INTO upstream_providers (name, slug, type, app_id, endpoint, headers, transport, enabled, created_at, updated_at)
		VALUES ('Legacy FileDrop', 'cloud.lazycat.app.lazycat-filedrop-skill', 'lazycat', 'cloud.lazycat.app.lazycat-filedrop-skill', '/api/mcp', '[]', 'streamable_http', true, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	if err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := openDB(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	service := NewProviderService(db)
	provider, err := service.Create(ctx, ProviderInput{
		Type:        "lazycat",
		Name:        "懒猫文件管家",
		Slug:        "cloud.lazycat.app.lazycat-filedrop-skill",
		AppID:       "cloud.lazycat.app.lazycat-filedrop-skill",
		OwnerUserID: "user-a",
		DeployID:    "cloud.lazycat.app.lazycat-filedrop-skill",
		AppTitle:    "懒猫文件管家",
		ResourceID:  "filedrop",
		Endpoint:    "/api/mcp",
		Transport:   "streamable_http",
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.ID != 1 {
		t.Fatalf("provider id = %d, want legacy row id 1", provider.ID)
	}
	if provider.OwnerUserID != "user-a" {
		t.Fatalf("owner = %q", provider.OwnerUserID)
	}
}
