package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMCPCallLogServiceRecordListAndFilter(t *testing.T) {
	ctx := context.Background()
	db, err := openDB(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	service := NewMCPCallLogService(db, 30)
	firstTime := time.Date(2026, 6, 20, 9, 0, 0, 0, time.UTC)
	secondTime := firstTime.Add(time.Minute)
	statusCode := 502
	longError := strings.Repeat("x", callLogErrorMaxLen+20)
	if _, err := service.Record(ctx, MCPCallLogInput{
		Source:    "local",
		Transport: "streamable_http",
		Method:    "lazycat_mcp_provider_list",
		Target:    "lazycat_mcp_provider_list",
		Status:    "success",
		Duration:  12 * time.Millisecond,
		CreatedAt: &firstTime,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Record(ctx, MCPCallLogInput{
		Source:       "upstream",
		Transport:    "http",
		Method:       "POST",
		Target:       "/mcp/apps/context7",
		ProviderSlug: "context7",
		Status:       "error",
		StatusCode:   &statusCode,
		Duration:     34 * time.Millisecond,
		Error:        longError,
		CreatedAt:    &secondTime,
	}); err != nil {
		t.Fatal(err)
	}

	logs, err := service.List(ctx, MCPCallLogFilter{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 {
		t.Fatalf("logs = %d", len(logs))
	}
	if logs[0].ProviderSlug != "context7" || logs[0].Status != "error" || logs[0].StatusCode == nil || *logs[0].StatusCode != 502 {
		t.Fatalf("newest log = %#v", logs[0])
	}
	if logs[0].DurationMs != 34 {
		t.Fatalf("duration_ms = %d", logs[0].DurationMs)
	}
	if len([]rune(logs[0].Error)) != callLogErrorMaxLen {
		t.Fatalf("error length = %d", len([]rune(logs[0].Error)))
	}

	filtered, err := service.List(ctx, MCPCallLogFilter{Source: "upstream", Status: "error", ProviderSlug: "context7"})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0].Target != "/mcp/apps/context7" {
		t.Fatalf("filtered logs = %#v", filtered)
	}
}

func TestMCPCallLogServiceCapsLimit(t *testing.T) {
	ctx := context.Background()
	db, err := openDB(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	service := NewMCPCallLogService(db, 30)
	for i := 0; i < maxCallLogLimit+25; i++ {
		if _, err := service.Record(ctx, MCPCallLogInput{
			Method: "tool",
			Target: "tool",
			Status: "success",
		}); err != nil {
			t.Fatal(err)
		}
	}
	logs, err := service.List(ctx, MCPCallLogFilter{Limit: maxCallLogLimit + 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != maxCallLogLimit {
		t.Fatalf("logs = %d, want %d", len(logs), maxCallLogLimit)
	}
}

func TestMCPCallLogServiceCleanupUsesRetentionWindow(t *testing.T) {
	ctx := context.Background()
	db, err := openDB(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	service := NewMCPCallLogService(db, 7)
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -8)
	recent := now.AddDate(0, 0, -2)
	for _, createdAt := range []time.Time{old, recent} {
		if _, err := service.Record(ctx, MCPCallLogInput{
			Method:    "tool",
			Target:    "tool",
			Status:    "success",
			CreatedAt: &createdAt,
		}); err != nil {
			t.Fatal(err)
		}
	}

	deleted, err := service.Cleanup(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d", deleted)
	}
	logs, err := service.List(ctx, MCPCallLogFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || !logs[0].CreatedAt.Equal(recent) {
		t.Fatalf("remaining logs = %#v", logs)
	}
}

func TestMCPCallLogServiceCleanupCanBeDisabled(t *testing.T) {
	ctx := context.Background()
	db, err := openDB(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	service := NewMCPCallLogService(db, 0)
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := service.Record(ctx, MCPCallLogInput{
		Method:    "tool",
		Target:    "tool",
		CreatedAt: &old,
	}); err != nil {
		t.Fatal(err)
	}
	deleted, err := service.Cleanup(ctx, time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 0 {
		t.Fatalf("deleted = %d", deleted)
	}
}
