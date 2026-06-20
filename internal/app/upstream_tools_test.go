package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func TestRefreshUpstreamToolsRegistersNamespacedTools(t *testing.T) {
	ctx := context.Background()
	upstream := mcpserver.NewMCPServer("upstream", "1.0", mcpserver.WithToolCapabilities(true))
	upstream.AddTool(
		mcp.NewTool("resolve-library",
			mcp.WithDescription("Resolve a package name."),
			mcp.WithString("name", mcp.Required()),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("resolved"), nil
		},
	)
	upstreamHTTP := mcpserver.NewTestStreamableHTTPServer(upstream)
	defer upstreamHTTP.Close()

	db, err := openDB(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	providers := NewProviderService(db)
	if _, err := providers.Create(ctx, ProviderInput{
		Type:      "custom",
		Name:      "Context7",
		Slug:      "context7",
		BaseURL:   upstreamHTTP.URL,
		Endpoint:  "/",
		Transport: "streamable_http",
	}); err != nil {
		t.Fatal(err)
	}

	app := &App{providers: providers}
	app.mcpServer = app.newMCPServer()
	app.upstreamToolRefs = make(map[string]upstreamToolRef)
	if err := app.refreshUpstreamTools(ctx); err != nil {
		t.Fatal(err)
	}

	tool := app.mcpServer.GetTool("context7__resolve_library")
	if tool == nil {
		t.Fatal("expected namespaced upstream tool to be registered")
	}
	if !strings.Contains(tool.Tool.Description, `provider "context7"`) || !strings.Contains(tool.Tool.Description, `tool "resolve-library"`) {
		t.Fatalf("description = %q", tool.Tool.Description)
	}
	if _, ok := app.upstreamToolRef("context7__resolve_library"); !ok {
		t.Fatal("expected upstream tool ref")
	}
}

func TestAggregatedToolCallsOriginalUpstreamTool(t *testing.T) {
	ctx := context.Background()
	upstream := mcpserver.NewMCPServer("upstream", "1.0", mcpserver.WithToolCapabilities(true))
	upstream.AddTool(
		mcp.NewTool("echo"),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := request.GetArguments()
			value, _ := args["value"].(string)
			return mcp.NewToolResultText("upstream:" + value), nil
		},
	)
	upstreamHTTP := mcpserver.NewTestStreamableHTTPServer(upstream)
	defer upstreamHTTP.Close()

	db, err := openDB(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	providers := NewProviderService(db)
	if _, err := providers.Create(ctx, ProviderInput{
		Type:      "custom",
		Name:      "Context7",
		Slug:      "context7",
		BaseURL:   upstreamHTTP.URL,
		Endpoint:  "/",
		Transport: "streamable_http",
	}); err != nil {
		t.Fatal(err)
	}

	app := &App{providers: providers}
	app.mcpServer = app.newMCPServer()
	app.upstreamToolRefs = make(map[string]upstreamToolRef)
	if err := app.refreshUpstreamTools(ctx); err != nil {
		t.Fatal(err)
	}

	tool := app.mcpServer.GetTool("context7__echo")
	if tool == nil {
		t.Fatal("expected aggregate echo tool")
	}
	result, err := tool.Handler(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "context7__echo",
			Arguments: map[string]any{
				"value": "ok",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content length = %d", len(result.Content))
	}
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T", result.Content[0])
	}
	if text.Text != "upstream:ok" {
		t.Fatalf("text = %q", text.Text)
	}
}

func TestRefreshUpstreamToolsRemovesDisabledProviderTools(t *testing.T) {
	ctx := context.Background()
	upstream := mcpserver.NewMCPServer("upstream", "1.0", mcpserver.WithToolCapabilities(true))
	upstream.AddTool(
		mcp.NewTool("echo"),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("ok"), nil
		},
	)
	upstreamHTTP := mcpserver.NewTestStreamableHTTPServer(upstream)
	defer upstreamHTTP.Close()

	db, err := openDB(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	providers := NewProviderService(db)
	provider, err := providers.Create(ctx, ProviderInput{
		Type:      "custom",
		Name:      "Context7",
		Slug:      "context7",
		BaseURL:   upstreamHTTP.URL,
		Endpoint:  "/",
		Transport: "streamable_http",
	})
	if err != nil {
		t.Fatal(err)
	}

	app := &App{providers: providers}
	app.mcpServer = app.newMCPServer()
	app.upstreamToolRefs = make(map[string]upstreamToolRef)
	if err := app.refreshUpstreamTools(ctx); err != nil {
		t.Fatal(err)
	}
	if app.mcpServer.GetTool("context7__echo") == nil {
		t.Fatal("expected aggregate tool")
	}

	disabled := false
	if _, err := providers.Update(ctx, provider.ID, ProviderInput{Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	if err := app.refreshUpstreamTools(ctx); err != nil {
		t.Fatal(err)
	}
	if app.mcpServer.GetTool("context7__echo") != nil {
		t.Fatal("expected aggregate tool to be removed")
	}
}
