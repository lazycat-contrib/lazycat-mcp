package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestResourceScannerScansNestedMCPProviders(t *testing.T) {
	root := t.TempDir()
	mcpDir := filepath.Join(root, "mcp-providers", "cloud.lazycat.app.photo", "default")
	skillDir := filepath.Join(root, "skills", "cloud.lazycat.app.photo", "photo.skill")
	if err := os.MkdirAll(mcpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mcpDir, "mcp.yml"), []byte("endpoint: /api/mcp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: photo\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	index := NewResourceScanner(root).Scan(context.Background())
	if got := index.DefaultMCPEndpoint("cloud.lazycat.app.photo"); got != "/api/mcp" {
		t.Fatalf("endpoint = %q", got)
	}
	if got := index.DefaultMCPResourceID("cloud.lazycat.app.photo"); got != "default" {
		t.Fatalf("resource id = %q", got)
	}
	if len(index.SkillsByApp["cloud.lazycat.app.photo"]) != 1 {
		t.Fatalf("skills = %d", len(index.SkillsByApp["cloud.lazycat.app.photo"]))
	}
}
