package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestResourceIndexHasProviderResourceFallsBackToSingleSkill(t *testing.T) {
	index := ResourceIndex{
		MCPByApp: map[string][]MCPResource{},
		SkillsByApp: map[string][]SkillResource{
			"anna-skill": {{AppID: "anna-skill", ResourceID: "wx-agent"}},
		},
	}
	if !index.HasProviderResource("anna-skill", "default") {
		t.Fatal("expected single Skill resource fallback")
	}
}

func TestResourceIndexHasProviderResourceRejectsSkillFallbackWhenMCPResourcesRemain(t *testing.T) {
	index := ResourceIndex{
		MCPByApp: map[string][]MCPResource{
			"anna-skill": {{AppID: "anna-skill", ResourceID: "other"}},
		},
		SkillsByApp: map[string][]SkillResource{
			"anna-skill": {{AppID: "anna-skill", ResourceID: "wx-agent"}},
		},
	}
	if index.HasProviderResource("anna-skill", "default") {
		t.Fatal("expected unrelated Skill resource not to preserve missing MCP provider")
	}
}

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
