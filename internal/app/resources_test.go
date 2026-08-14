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

func TestResourceScannerScansExporterWrappedResources(t *testing.T) {
	root := t.TempDir()
	exporterID := "cloud.lazycat.app.exporter"
	resourceAppID := "cloud.lazycat.app.photo"
	mcpDir := filepath.Join(root, "mcp-providers", exporterID, resourceAppID, "default")
	skillDir := filepath.Join(root, "skills", exporterID, resourceAppID, "photo.skill")
	for _, dir := range []string{mcpDir, skillDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(mcpDir, "mcp.yml"), []byte("endpoint: /api/mcp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: photo\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	scanner := NewResourceScanner(root)
	indexes := []struct {
		name  string
		index ResourceIndex
	}{
		{name: "lenient", index: scanner.Scan(context.Background())},
	}
	strictIndex, err := scanner.ScanForReconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	indexes = append(indexes, struct {
		name  string
		index ResourceIndex
	}{name: "strict", index: strictIndex})

	for _, tt := range indexes {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.index.DefaultMCPEndpoint(resourceAppID); got != "/api/mcp" {
				t.Fatalf("endpoint = %q", got)
			}
			if got := tt.index.DefaultMCPResourceID(resourceAppID); got != "default" {
				t.Fatalf("resource id = %q", got)
			}
			if len(tt.index.MCPByApp[exporterID]) != 0 {
				t.Fatal("exporter package must not be indexed as the resource app")
			}
			skilled := tt.index.SkillsByApp[resourceAppID]
			if len(skilled) != 1 {
				t.Fatalf("skills = %d", len(skilled))
			}
			if got := skilled[0].ResourceID; got != "photo.skill" {
				t.Fatalf("skill resource id = %q", got)
			}
			wantPublicPath := "/skills/" + exporterID + "/" + resourceAppID + "/photo.skill/SKILL.md"
			if got := skilled[0].PublicPath; got != wantPublicPath {
				t.Fatalf("public path = %q, want %q", got, wantPublicPath)
			}
			if len(tt.index.SkillsByApp[exporterID]) != 0 {
				t.Fatal("exporter package must not be indexed as the resource app")
			}
		})
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
