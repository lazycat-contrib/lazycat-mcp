package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"lazycat-mcp/ent/mcpcalllog"
)

func TestMCPCallLogToolMiddlewareRecordsLocalToolCall(t *testing.T) {
	ctx := context.Background()
	db, err := openDB(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	logs := NewMCPCallLogService(db, 30)
	app := &App{mcpLogs: logs}
	handler := app.mcpCallLogToolMiddleware()(func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("ok"), nil
	})
	req := mcp.CallToolRequest{}
	req.Params.Name = "lazycat_mcp_provider_list"
	req.Header = http.Header{}
	req.Header.Set("Authorization", "Bearer lcmcp_test-token-value")
	req.Header.Set("Mcp-Session-Id", "session-1")
	req.Header.Set("X-Request-Id", "request-1")

	if _, err := handler(ctx, req); err != nil {
		t.Fatal(err)
	}

	got, err := logs.List(ctx, MCPCallLogFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("logs = %d", len(got))
	}
	log := got[0]
	if log.Source != mcpcalllog.SourceLocal.String() || log.Method != "lazycat_mcp_provider_list" || log.Status != mcpcalllog.StatusSuccess.String() {
		t.Fatalf("log = %#v", log)
	}
	if log.TokenPrefix != "lcmcp_test-tok" || log.SessionID != "session-1" || log.RequestID != "request-1" {
		t.Fatalf("identity fields = %#v", log)
	}
}

func TestMCPProxyLoggingRecordsUpstreamHTTPStatus(t *testing.T) {
	ctx := context.Background()
	db, err := openDB(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	logs := NewMCPCallLogService(db, 30)
	app := &App{mcpLogs: logs}
	handler := app.withMCPProxyLogging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}))
	req := httptest.NewRequest(http.MethodPost, "/mcp/apps/context7/tools/call", nil)
	req.Header.Set("X-MCP-Token", "lcmcp_proxy-token-value")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d", rec.Code)
	}
	got, err := logs.List(ctx, MCPCallLogFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("logs = %d", len(got))
	}
	log := got[0]
	if log.Source != mcpcalllog.SourceUpstream.String() || log.ProviderSlug != "context7" || log.Target != "/mcp/apps/context7" {
		t.Fatalf("log = %#v", log)
	}
	if log.Status != mcpcalllog.StatusError.String() || log.StatusCode == nil || *log.StatusCode != http.StatusBadGateway {
		t.Fatalf("status fields = %#v", log)
	}
}

func TestMCPCallLogToolMiddlewareRecordsAggregatedToolAsUpstream(t *testing.T) {
	ctx := context.Background()
	db, err := openDB(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	logs := NewMCPCallLogService(db, 30)
	app := &App{
		mcpLogs: logs,
		upstreamToolRefs: map[string]upstreamToolRef{
			"context7__resolve": {
				AggregateName: "context7__resolve",
				ProviderSlug:  "context7",
				UpstreamName:  "resolve",
			},
		},
	}
	handler := app.mcpCallLogToolMiddleware()(func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("ok"), nil
	})
	req := mcp.CallToolRequest{}
	req.Params.Name = "context7__resolve"

	if _, err := handler(ctx, req); err != nil {
		t.Fatal(err)
	}
	got, err := logs.List(ctx, MCPCallLogFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("logs = %d", len(got))
	}
	log := got[0]
	if log.Source != mcpcalllog.SourceUpstream.String() || log.ProviderSlug != "context7" || log.Target != "resolve" {
		t.Fatalf("log = %#v", log)
	}
}
