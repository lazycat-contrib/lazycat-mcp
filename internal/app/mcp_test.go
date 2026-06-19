package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestProviderListToolOnlyExposesGatewayEndpoints(t *testing.T) {
	ctx := context.Background()
	db, err := openDB(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	providers := NewProviderService(db)
	if _, err := providers.Create(ctx, ProviderInput{
		Type:        "custom",
		Name:        "Context7",
		Description: "Documentation MCP",
		Slug:        "context7",
		BaseURL:     "https://secret.example.com",
		Endpoint:    "/mcp",
		Transport:   "streamable_http",
		Headers: []ProviderHeader{
			{Name: "Authorization", Value: "Bearer secret"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	app := &App{providers: providers}
	result, err := app.providerListTool().Handler(ctx, mcp.CallToolRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content length = %d", len(result.Content))
	}
	content, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T", result.Content[0])
	}
	if !strings.Contains(content.Text, `"/mcp/apps/context7"`) {
		t.Fatalf("provider endpoint missing from %s", content.Text)
	}
	for _, leaked := range []string{"secret.example.com", "Bearer secret", "base_url", "headers", "Authorization"} {
		if strings.Contains(content.Text, leaked) {
			t.Fatalf("provider list leaked %q in %s", leaked, content.Text)
		}
	}
}
