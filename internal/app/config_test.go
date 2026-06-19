package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDBPathUsesExplicitEnv(t *testing.T) {
	t.Setenv("LAZYCAT_MCP_DB", "/custom/lazycat-mcp.db")
	t.Setenv("LAZYCAT_MCP_DEV_DB", "/dev/lazycat-mcp.db")

	if got := resolveDBPath(filepath.Join(t.TempDir(), "missing")); got != "/custom/lazycat-mcp.db" {
		t.Fatalf("db path = %q", got)
	}
}

func TestResolveDBPathUsesLazyCatVarWhenDataDirIsMissing(t *testing.T) {
	t.Setenv("LAZYCAT_MCP_DB", "")
	t.Setenv("LAZYCAT_MCP_DEV_DB", "/dev/lazycat-mcp.db")

	lazycatVarDir := filepath.Join(t.TempDir(), "var")
	if err := os.MkdirAll(lazycatVarDir, 0o755); err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(lazycatVarDir, "data", "lazycat-mcp.db")
	if got := resolveDBPath(lazycatVarDir); got != want {
		t.Fatalf("db path = %q, want %q", got, want)
	}
}

func TestResolveDBPathFallsBackToDevPathOutsideLazyCat(t *testing.T) {
	t.Setenv("LAZYCAT_MCP_DB", "")
	t.Setenv("LAZYCAT_MCP_DEV_DB", "/dev/lazycat-mcp.db")

	if got := resolveDBPath(filepath.Join(t.TempDir(), "missing")); got != "/dev/lazycat-mcp.db" {
		t.Fatalf("db path = %q", got)
	}
}
