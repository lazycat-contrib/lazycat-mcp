package app

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

type SkillContent struct {
	Title          string
	Summary        string
	PromptExamples []string
	RawMarkdown    string
	ResourceURI    string
	SourcePath     string
}

type skillState struct {
	content SkillContent
}

var skillStatesMu sync.RWMutex
var skillStatesMap = make(map[string]*skillState)

func (a *App) refreshSkillStates(ctx context.Context, providers []*ProviderDTOView) map[string]string {
	nextStates := make(map[string]*skillState)
	errors := make(map[string]string)
	if a == nil || a.resources == nil {
		skillStatesMu.Lock()
		skillStatesMap = nextStates
		skillStatesMu.Unlock()
		return errors
	}
	index := a.resources.Scan(ctx)

	for _, provider := range providers {
		resources := index.SkillsByApp[provider.AppID]
		if len(resources) == 0 {
			continue
		}
		selected, err := selectSkillResource(resources, provider.ResourceID)
		if err != nil {
			errors[provider.Slug] = err.Error()
			continue
		}
		content, err := loadSkillContent(selected)
		if err != nil {
			errors[provider.Slug] = err.Error()
			continue
		}
		nextStates[provider.Slug] = &skillState{content: content}
	}

	skillStatesMu.Lock()
	skillStatesMap = nextStates
	skillStatesMu.Unlock()
	return errors
}

func selectSkillResource(resources []SkillResource, preferred string) (*SkillResource, error) {
	if len(resources) == 0 {
		return nil, fmt.Errorf("missing SKILL.md")
	}
	preferred = strings.TrimSpace(preferred)
	if preferred != "" {
		for i := range resources {
			if resources[i].ResourceID == preferred {
				return &resources[i], nil
			}
		}
		return nil, fmt.Errorf("missing SKILL.md for resource %q", preferred)
	}
	if len(resources) == 1 {
		return &resources[0], nil
	}
	for i := range resources {
		if resources[i].ResourceID == "default" {
			return &resources[i], nil
		}
	}
	return &resources[0], nil
}

func loadSkillContent(resource *SkillResource) (SkillContent, error) {
	data, err := os.ReadFile(resource.FilePath)
	if err != nil {
		return SkillContent{}, fmt.Errorf("read SKILL.md: %w", err)
	}
	text := string(data)
	title, summary, prompts := parseSkillMarkdown(text)
	if strings.TrimSpace(title) == "" {
		title = resource.ResourceID
	}
	if strings.TrimSpace(summary) == "" {
		summary = "Skill resource available via SKILL.md"
	}
	return SkillContent{
		Title:          title,
		Summary:        summary,
		PromptExamples: prompts,
		RawMarkdown:    text,
		ResourceURI:    "skills://" + resource.AppID + "/SKILL.md",
		SourcePath:     resource.FilePath,
	}, nil
}

func parseSkillMarkdown(raw string) (title, summary string, prompts []string) {
	scanner := bufio.NewScanner(strings.NewReader(raw))
	inFrontmatter := false
	frontmatterDone := false
	inPromptSection := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if !frontmatterDone && trimmed == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			inFrontmatter = false
			frontmatterDone = true
			continue
		}
		if inFrontmatter {
			lower := strings.ToLower(trimmed)
			if title == "" && strings.HasPrefix(lower, "name:") {
				title = strings.Trim(strings.TrimSpace(trimmed[len("name:"):]), "\"'")
			}
			if summary == "" && strings.HasPrefix(lower, "description:") {
				summary = strings.Trim(strings.TrimSpace(trimmed[len("description:"):]), "\"'")
			}
			continue
		}

		if title == "" && strings.HasPrefix(trimmed, "# ") {
			title = strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
			continue
		}
		if summary == "" && trimmed != "" && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "*") {
			summary = trimmed
		}

		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(trimmed, "## ") {
			inPromptSection = strings.Contains(lower, "prompt") || strings.Contains(lower, "示例")
			continue
		}
		if inPromptSection {
			if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
				prompt := strings.TrimSpace(trimmed[2:])
				if prompt != "" {
					prompts = append(prompts, prompt)
				}
			}
		}
	}

	return title, summary, prompts
}

func (a *App) registerSkillResources() {
	skillStatesMu.RLock()
	defer skillStatesMu.RUnlock()

	for _, st := range skillStatesMap {
		content := st.content
		mcpResource := mcp.NewResource(
			content.ResourceURI,
			content.Title+" (Skill)",
			mcp.WithResourceDescription(content.Summary),
			mcp.WithMIMEType("text/markdown"),
		)
		a.mcpServer.AddResource(mcpResource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			return []mcp.ResourceContents{
				mcp.TextResourceContents{
					URI:      content.ResourceURI,
					MIMEType: "text/markdown",
					Text:     content.RawMarkdown,
				},
			}, nil
		})
	}
}

func (a *App) skillPromptTool() mcpserver.ServerTool {
	return mcpserver.ServerTool{
		Tool: mcp.NewTool("skill_prompt",
			mcp.WithDescription("Return the SKILL.md content for a LazyCat skill resource. Use this to learn how to interact with skill-based apps that export a real SKILL.md."),
			mcp.WithString("slug",
				mcp.Required(),
				mcp.Description("Provider slug for the skill (e.g. 'cloud.lazycat.app.anna-book-smart-download-skill'). Get available slugs from lazycat_mcp_provider_list."),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args, _ := request.Params.Arguments.(map[string]interface{})
			slug, _ := args["slug"].(string)
			slug = strings.TrimSpace(slug)
			if slug == "" {
				return mcp.NewToolResultError("slug is required"), nil
			}

			skillStatesMu.RLock()
			st, ok := skillStatesMap[slug]
			skillStatesMu.RUnlock()
			if !ok {
				return mcp.NewToolResultError(fmt.Sprintf("skill not found for slug %q — missing SKILL.md or resource not registered", slug)), nil
			}

			return mcp.NewToolResultText(st.content.RawMarkdown), nil
		},
	}
}

func skillContentBySlug(slug string) *SkillContent {
	skillStatesMu.RLock()
	defer skillStatesMu.RUnlock()
	if st, ok := skillStatesMap[slug]; ok {
		cp := st.content
		return &cp
	}
	return nil
}

type ProviderDTOView struct {
	Slug       string
	AppID      string
	ResourceID string
}
