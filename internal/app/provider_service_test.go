package app

import (
	"context"
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
