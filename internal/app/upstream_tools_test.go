package app

import (
	"context"
	"os"
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

func TestSelectSkillResourceFallsBackToSingleSkillEvenWhenResourceIDDiffers(t *testing.T) {
	resources := []SkillResource{{
		AppID:      "cloud.lazycat.app.wx-data-helper-skill",
		ResourceID: "wx-agent",
		FilePath:   "/tmp/SKILL.md",
		PublicPath: "/skills/cloud.lazycat.app.wx-data-helper-skill/wx-agent/SKILL.md",
	}}
	selected, err := selectSkillResource(resources, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected.ResourceID != "wx-agent" {
		t.Fatalf("resource_id = %q", selected.ResourceID)
	}
}

func TestRefreshUpstreamToolsLoadsSkillContentFromResourceScanner(t *testing.T) {
	ctx := context.Background()
	skillStatesMu.Lock()
	originalSkillStates := skillStatesMap
	skillStatesMap = map[string]*skillState{}
	skillStatesMu.Unlock()
	defer func() {
		skillStatesMu.Lock()
		skillStatesMap = originalSkillStates
		skillStatesMu.Unlock()
	}()

	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "anna-skill", "wx-agent")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillMD := "---\nname: 安娜智能下载\ndescription: 这是一个给 Agent 使用的技能页面。\n---\n## Prompt Examples\n- 帮我找一本书\n- 帮我批量下载\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		t.Fatal(err)
	}

	db, err := openDB(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	providers := NewProviderService(db)
	if _, err := providers.Create(ctx, ProviderInput{
		Type:       "lazycat",
		Name:       "Anna Skill",
		Slug:       "anna-skill",
		AppID:      "anna-skill",
		ResourceID: "default",
		Endpoint:   "/mcp",
		Transport:  "streamable_http",
	}); err != nil {
		t.Fatal(err)
	}

	app := &App{providers: providers, resources: NewResourceScanner(root)}
	app.mcpServer = app.newMCPServer()
	app.upstreamToolRefs = make(map[string]upstreamToolRef)
	app.upstreamFailureReasons = make(map[string]string)
	if err := app.refreshUpstreamTools(ctx); err != nil {
		t.Fatal(err)
	}

	skill := skillContentBySlug("anna-skill")
	if skill == nil {
		t.Fatal("expected skill content to be registered")
	}
	if skill.Title != "安娜智能下载" {
		t.Fatalf("title = %q", skill.Title)
	}
	if skill.Summary != "这是一个给 Agent 使用的技能页面。" {
		t.Fatalf("summary = %q", skill.Summary)
	}
	if got := strings.Join(skill.PromptExamples, "|"); got != "帮我找一本书|帮我批量下载" {
		t.Fatalf("prompts = %q", got)
	}
	if skill.ResourceURI != "skills://anna-skill/SKILL.md" {
		t.Fatalf("resource uri = %q", skill.ResourceURI)
	}

	prompt, err := app.skillPromptTool().Handler(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"slug": "anna-skill"}}})
	if err != nil {
		t.Fatal(err)
	}
	content, ok := prompt.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T", prompt.Content[0])
	}
	if !strings.Contains(content.Text, "name: 安娜智能下载") {
		t.Fatalf("unexpected prompt text: %s", content.Text)
	}
}


func TestRefreshUpstreamToolsSkillOnlyProviderDoesNotProbeMCPTransport(t *testing.T) {
	ctx := context.Background()
	skillStatesMu.Lock()
	originalSkillStates := skillStatesMap
	skillStatesMap = map[string]*skillState{}
	skillStatesMu.Unlock()
	defer func() {
		skillStatesMu.Lock()
		skillStatesMap = originalSkillStates
		skillStatesMu.Unlock()
	}()

	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "anna-skill", "default")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillMD := "---\nname: 安娜智能下载\ndescription: skill only\n---\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		t.Fatal(err)
	}

	db, err := openDB(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	providers := NewProviderService(db)
	if _, err := providers.Create(ctx, ProviderInput{
		Type:       "lazycat",
		Name:       "Anna Skill",
		Slug:       "anna-skill",
		AppID:      "anna-skill",
		ResourceID: "default",
		Endpoint:   "/mcp",
		Transport:  "streamable_http",
	}); err != nil {
		t.Fatal(err)
	}

	app := &App{providers: providers, resources: NewResourceScanner(root)}
	app.mcpServer = app.newMCPServer()
	app.upstreamToolRefs = make(map[string]upstreamToolRef)
	app.upstreamHealthySlugs = make(map[string]bool)
	app.upstreamFailureReasons = make(map[string]string)
	if err := app.refreshUpstreamTools(ctx); err != nil {
		t.Fatal(err)
	}

	if got := app.aggregateErrors()["anna-skill"]; got != "" {
		t.Fatalf("unexpected aggregate error for skill-only provider: %q", got)
	}
	if !app.aggregatedSlugs()["anna-skill"] {
		t.Fatal("expected skill-only provider to be marked healthy")
	}
}
