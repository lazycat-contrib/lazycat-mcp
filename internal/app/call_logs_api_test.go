package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestMCPCallLogAPIListCleanupAndClear(t *testing.T) {
	ctx := context.Background()
	db, err := openDB(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	logs := NewMCPCallLogService(db, 7)
	app := &App{
		cfg:     Config{MCPLogRetentionDays: 7},
		mcpLogs: logs,
	}
	old := time.Now().AddDate(0, 0, -8)
	recent := time.Now()
	if _, err := logs.Record(ctx, MCPCallLogInput{
		Source:    "local",
		Method:    "old_tool",
		Target:    "old_tool",
		CreatedAt: &old,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := logs.Record(ctx, MCPCallLogInput{
		Source:    "local",
		Method:    "new_tool",
		Target:    "new_tool",
		CreatedAt: &recent,
	}); err != nil {
		t.Fatal(err)
	}

	listResp := httptest.NewRecorder()
	app.handleAPI(listResp, httptest.NewRequest(http.MethodGet, "/api/mcp-logs?limit=1&source=local", nil))
	if listResp.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", listResp.Code, listResp.Body.String())
	}
	var listed struct {
		Logs          []MCPCallLogDTO `json:"logs"`
		RetentionDays int             `json:"retention_days"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if listed.RetentionDays != 7 || len(listed.Logs) != 1 || listed.Logs[0].Method != "new_tool" {
		t.Fatalf("listed = %#v", listed)
	}

	cleanupResp := httptest.NewRecorder()
	app.handleAPI(cleanupResp, httptest.NewRequest(http.MethodPost, "/api/mcp-logs/cleanup", nil))
	if cleanupResp.Code != http.StatusOK {
		t.Fatalf("cleanup status = %d body=%s", cleanupResp.Code, cleanupResp.Body.String())
	}
	var cleaned struct {
		Deleted int `json:"deleted"`
	}
	if err := json.Unmarshal(cleanupResp.Body.Bytes(), &cleaned); err != nil {
		t.Fatal(err)
	}
	if cleaned.Deleted != 1 {
		t.Fatalf("deleted = %d", cleaned.Deleted)
	}

	clearResp := httptest.NewRecorder()
	app.handleAPI(clearResp, httptest.NewRequest(http.MethodDelete, "/api/mcp-logs", nil))
	if clearResp.Code != http.StatusOK {
		t.Fatalf("clear status = %d body=%s", clearResp.Code, clearResp.Body.String())
	}
	remaining, err := logs.List(ctx, MCPCallLogFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("remaining logs = %d", len(remaining))
	}
}

func TestMCPCallLogAPIRejectsInvalidFilter(t *testing.T) {
	ctx := context.Background()
	db, err := openDB(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	app := &App{mcpLogs: NewMCPCallLogService(db, 7)}
	resp := httptest.NewRecorder()
	app.handleAPI(resp, httptest.NewRequest(http.MethodGet, "/api/mcp-logs?source=bad", nil))
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", resp.Code, resp.Body.String())
	}
}
