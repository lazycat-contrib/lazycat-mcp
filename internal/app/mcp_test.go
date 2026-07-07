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
		OwnerUserID: "u1",
		Headers: []ProviderHeader{
			{Name: "Authorization", Value: "Bearer secret"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	app := &App{providers: providers}
	ctx = contextWithMCPToken(ctx, TokenDTO{OwnerUserID: "u1"})
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

func TestProviderListToolIncludesSkillMetadata(t *testing.T) {
	ctx := context.Background()
	db, err := openDB(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	providers := NewProviderService(db)
	if _, err := providers.Create(ctx, ProviderInput{
		Type:        "custom",
		Name:        "Anna Skill",
		Slug:        "anna-skill",
		BaseURL:     "https://skill.example.com",
		Endpoint:    "/mcp",
		Transport:   "streamable_http",
		OwnerUserID: "u1",
	}); err != nil {
		t.Fatal(err)
	}

	skillStatesMu.Lock()
	originalSkillStates := skillStatesMap
	skillStatesMap = map[string]*skillState{
		"anna-skill": {
			content: SkillContent{
				Title:          "安娜智能下载",
				Summary:        "这是一个技能页面摘要。",
				PromptExamples: []string{"帮我找一本书", "帮我批量下载"},
				RawMarkdown:    "---\nname: 安娜智能下载\ndescription: 这是一个技能页面摘要。\n---\n## Prompt Examples\n- 帮我找一本书\n- 帮我批量下载\n",
				ResourceURI:    "skills://anna-skill/SKILL.md",
			},
		},
	}
	skillStatesMu.Unlock()
	defer func() {
		skillStatesMu.Lock()
		skillStatesMap = originalSkillStates
		skillStatesMu.Unlock()
	}()

	app := &App{providers: providers, upstreamFailureReasons: map[string]string{"anna-skill": "missing SKILL.md"}}
	ctx = contextWithMCPToken(ctx, TokenDTO{OwnerUserID: "u1"})
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

	if !strings.Contains(content.Text, `"kind":"skill"`) {
		t.Fatalf("expected skill kind in %s", content.Text)
	}
	for _, want := range []string{
		`"skill_title":"安娜智能下载"`,
		`"skill_summary":"这是一个技能页面摘要。"`,
		`"skill_prompts":["帮我找一本书","帮我批量下载"]`,
		`"slug":"anna-skill"`,
	} {
		if !strings.Contains(content.Text, want) {
			t.Fatalf("expected %s in %s", want, content.Text)
		}
	}
}

func TestBuiltinToolFilterHidesPowerForNonAdmin(t *testing.T) {
	app := &App{}
	tools := []mcp.Tool{
		mcp.NewTool("domain_base_info_lookup"),
		mcp.NewTool("lazycat_power"),
		mcp.NewTool("cloud.lazycat.app.czyt.lazycat-mcp__lazycat_power"),
	}
	filtered := app.filterBuiltinToolsByRole(context.Background(), tools)
	if len(filtered) != 0 {
		t.Fatalf("filtered tools = %#v", filtered)
	}
}

func TestBuiltinToolFilterAllowsPowerForAdmin(t *testing.T) {
	app := &App{}
	ctx := context.WithValue(context.Background(), lazycatRoleContextKey{}, "admin")
	tools := []mcp.Tool{
		mcp.NewTool("domain_base_info_lookup"),
		mcp.NewTool("lazycat_power"),
		mcp.NewTool("cloud.lazycat.app.czyt.lazycat-mcp__lazycat_power"),
	}
	filtered := app.filterBuiltinToolsByRole(ctx, tools)
	if len(filtered) != len(tools) {
		t.Fatalf("filtered tools = %#v", filtered)
	}
}
