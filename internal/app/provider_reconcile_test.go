package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestReconcileLazyCatProvidersDeletesMissingResources(t *testing.T) {
	ctx := context.Background()
	root := newReconcileResourceRoot(t)
	writeMCPResource(t, root, "valid-mcp", "default")
	writeSkillResource(t, root, "valid-skill", "assistant")

	db, err := openDB(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	providers := NewProviderService(db)
	for _, input := range []ProviderInput{
		{Type: "lazycat", Name: "Stale", Slug: "stale", AppID: "stale", ResourceID: "default", Endpoint: "/mcp", Transport: "streamable_http"},
		{Type: "lazycat", Name: "Valid MCP", Slug: "valid-mcp", AppID: "valid-mcp", ResourceID: "default", Endpoint: "/mcp", Transport: "streamable_http"},
		{Type: "lazycat", Name: "Valid Skill", Slug: "valid-skill", AppID: "valid-skill", ResourceID: "assistant", Endpoint: "/mcp", Transport: "streamable_http"},
		{Type: "lazycat", Name: "Built-in", Slug: selfPackageID, AppID: selfPackageID, ResourceID: "default", Endpoint: "/mcp", Transport: "streamable_http"},
		{Type: "custom", Name: "Custom", Slug: "custom", BaseURL: "https://example.com", Endpoint: "/mcp", Transport: "streamable_http"},
	} {
		if _, err := providers.Create(ctx, input); err != nil {
			t.Fatal(err)
		}
	}

	app := &App{providers: providers, resources: NewResourceScanner(root)}
	firstSeen := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	deleted, err := app.reconcileLazyCatProvidersAt(ctx, firstSeen)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 0 {
		t.Fatalf("first pass deleted = %d, want 0", deleted)
	}
	deleted, err = app.reconcileLazyCatProvidersAt(ctx, firstSeen.Add(providerReconcileGracePeriod))
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("second pass deleted = %d, want 1", deleted)
	}
	if _, err := providers.GetBySlug(ctx, "stale"); err == nil {
		t.Fatal("expected stale provider to be deleted")
	}
	for _, slug := range []string{"valid-mcp", "valid-skill", selfPackageID, "custom"} {
		if _, err := providers.GetBySlug(ctx, slug); err != nil {
			t.Fatalf("expected provider %q to remain: %v", slug, err)
		}
	}
}

func TestReconcileLazyCatProvidersSkipsCleanupWhenScanFails(t *testing.T) {
	ctx := context.Background()
	db, err := openDB(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	providers := NewProviderService(db)
	if _, err := providers.Create(ctx, ProviderInput{
		Type: "lazycat", Name: "Penpot", Slug: "penpot", AppID: "penpot", ResourceID: "default", Endpoint: "/mcp", Transport: "streamable_http",
	}); err != nil {
		t.Fatal(err)
	}

	app := &App{providers: providers, resources: NewResourceScanner(t.TempDir())}
	deleted, err := app.reconcileLazyCatProviders(ctx)
	if err == nil {
		t.Fatal("expected scan failure")
	}
	if deleted != 0 {
		t.Fatalf("deleted = %d, want 0", deleted)
	}
	if _, err := providers.GetBySlug(ctx, "penpot"); err != nil {
		t.Fatalf("provider should remain after scan failure: %v", err)
	}
}

func TestReconcileLazyCatProvidersRestartsGracePeriodAfterScanFailure(t *testing.T) {
	ctx := context.Background()
	root := newReconcileResourceRoot(t)
	db, err := openDB(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	providers := NewProviderService(db)
	if _, err := providers.Create(ctx, ProviderInput{
		Type: "lazycat", Name: "Penpot", Slug: "penpot", AppID: "penpot", ResourceID: "default", Endpoint: "/mcp", Transport: "streamable_http",
	}); err != nil {
		t.Fatal(err)
	}

	app := &App{providers: providers, resources: NewResourceScanner(root)}
	firstSeen := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	if _, err := app.reconcileLazyCatProvidersAt(ctx, firstSeen); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "skills")); err != nil {
		t.Fatal(err)
	}
	if _, err := app.reconcileLazyCatProvidersAt(ctx, firstSeen.Add(providerReconcileGracePeriod)); err == nil {
		t.Fatal("expected scan failure")
	}
	if err := os.MkdirAll(filepath.Join(root, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	deleted, err := app.reconcileLazyCatProvidersAt(ctx, firstSeen.Add(2*providerReconcileGracePeriod))
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 0 {
		t.Fatalf("deleted immediately after scan recovery = %d, want 0", deleted)
	}
	deleted, err = app.reconcileLazyCatProvidersAt(ctx, firstSeen.Add(3*providerReconcileGracePeriod))
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("deleted after renewed grace period = %d, want 1", deleted)
	}
}

func TestReconcileLazyCatProvidersSkipsCleanupWhenResourceIsMalformed(t *testing.T) {
	ctx := context.Background()
	root := newReconcileResourceRoot(t)
	if err := os.MkdirAll(filepath.Join(root, "mcp-providers", "broken", "default"), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := openDB(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	providers := NewProviderService(db)
	if _, err := providers.Create(ctx, ProviderInput{
		Type: "lazycat", Name: "Penpot", Slug: "penpot", AppID: "penpot", ResourceID: "default", Endpoint: "/mcp", Transport: "streamable_http",
	}); err != nil {
		t.Fatal(err)
	}

	app := &App{providers: providers, resources: NewResourceScanner(root)}
	deleted, err := app.reconcileLazyCatProviders(ctx)
	if err == nil {
		t.Fatal("expected malformed resource scan to fail")
	}
	if deleted != 0 {
		t.Fatalf("deleted = %d, want 0", deleted)
	}
	if _, err := providers.GetBySlug(ctx, "penpot"); err != nil {
		t.Fatalf("provider should remain after malformed resource scan: %v", err)
	}
}

func TestReconcileLazyCatProvidersPreservesManualProviderWithoutResourceID(t *testing.T) {
	ctx := context.Background()
	root := newReconcileResourceRoot(t)
	db, err := openDB(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	providers := NewProviderService(db)
	if _, err := providers.Create(ctx, ProviderInput{
		Type: "lazycat", Name: "Manual", Slug: "manual", AppID: "manual", Endpoint: "/mcp", Transport: "streamable_http",
	}); err != nil {
		t.Fatal(err)
	}

	app := &App{providers: providers, resources: NewResourceScanner(root)}
	firstSeen := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	if _, err := app.reconcileLazyCatProvidersAt(ctx, firstSeen); err != nil {
		t.Fatal(err)
	}
	if _, err := app.reconcileLazyCatProvidersAt(ctx, firstSeen.Add(2*providerReconcileGracePeriod)); err != nil {
		t.Fatal(err)
	}
	if _, err := providers.GetBySlug(ctx, "manual"); err != nil {
		t.Fatalf("manual provider should remain: %v", err)
	}
}

func TestReconcileLazyCatProvidersClearsMissingObservationWhenResourceReturns(t *testing.T) {
	ctx := context.Background()
	root := newReconcileResourceRoot(t)
	db, err := openDB(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	providers := NewProviderService(db)
	if _, err := providers.Create(ctx, ProviderInput{
		Type: "lazycat", Name: "Penpot", Slug: "penpot", AppID: "penpot", ResourceID: "default", Endpoint: "/mcp", Transport: "streamable_http",
	}); err != nil {
		t.Fatal(err)
	}

	app := &App{providers: providers, resources: NewResourceScanner(root)}
	firstSeen := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	if _, err := app.reconcileLazyCatProvidersAt(ctx, firstSeen); err != nil {
		t.Fatal(err)
	}
	writeMCPResource(t, root, "penpot", "default")
	if _, err := app.reconcileLazyCatProvidersAt(ctx, firstSeen.Add(providerReconcileGracePeriod)); err != nil {
		t.Fatal(err)
	}
	if _, err := providers.GetBySlug(ctx, "penpot"); err != nil {
		t.Fatalf("provider should remain after resource returns: %v", err)
	}
}

func TestRefreshUpstreamToolsRemovesToolsForReconciledProvider(t *testing.T) {
	ctx := context.Background()
	root := newReconcileResourceRoot(t)
	db, err := openDB(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	providers := NewProviderService(db)
	if _, err := providers.Create(ctx, ProviderInput{
		Type: "lazycat", Name: "Penpot", Slug: "penpot", AppID: "penpot", ResourceID: "default", Endpoint: "/mcp", Transport: "streamable_http",
	}); err != nil {
		t.Fatal(err)
	}

	app := &App{providers: providers, resources: NewResourceScanner(root)}
	app.mcpServer = app.newMCPServer()
	app.upstreamToolRefs = map[string]upstreamToolRef{
		"penpot__execute_code": {
			AggregateName: "penpot__execute_code",
			ProviderSlug:  "penpot",
			ProviderName:  "Penpot",
			UpstreamName:  "execute_code",
		},
	}
	app.mcpServer.AddTool(mcp.NewTool("penpot__execute_code"), nil)

	firstSeen := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	if _, err := app.reconcileLazyCatProvidersAt(ctx, firstSeen); err != nil {
		t.Fatal(err)
	}
	if _, err := app.reconcileLazyCatProvidersAt(ctx, firstSeen.Add(providerReconcileGracePeriod)); err != nil {
		t.Fatal(err)
	}
	if err := app.refreshUpstreamTools(ctx); err != nil {
		t.Fatal(err)
	}
	if app.mcpServer.GetTool("penpot__execute_code") != nil {
		t.Fatal("expected aggregate tool to be removed")
	}
	if _, err := providers.GetBySlug(ctx, "penpot"); err == nil {
		t.Fatal("expected stale provider to be deleted")
	}
}

func newReconcileResourceRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, kind := range []string{"mcp-providers", "skills"} {
		if err := os.MkdirAll(filepath.Join(root, kind), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func writeMCPResource(t *testing.T, root, appID, resourceID string) {
	t.Helper()
	dir := filepath.Join(root, "mcp-providers", appID, resourceID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mcp.yml"), []byte("endpoint: /mcp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeSkillResource(t *testing.T, root, appID, resourceID string) {
	t.Helper()
	dir := filepath.Join(root, "skills", appID, resourceID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: test\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
