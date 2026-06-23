package app

import (
	"context"
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"lazycat-mcp/internal/buildinfo"
)

func (a *App) newMCPServer() *mcpserver.MCPServer {
	info := buildinfo.Snapshot()
	svr := mcpserver.NewMCPServer(
		"LazyCat MCP",
		info.Version,
		mcpserver.WithToolHandlerMiddleware(a.mcpCallLogToolMiddleware()),
		mcpserver.WithLogging(),
		mcpserver.WithRecovery(),
		mcpserver.WithToolCapabilities(true),
		mcpserver.WithResourceCapabilities(true, true),
	)

	tools := []mcpserver.ServerTool{a.providerListTool()}
	if a.kit != nil {
		tools = append(tools, a.kit.DomainKits()...)
		if a.kit.Available() {
			tools = append(tools, a.kit.PowerKits()...)
			tools = append(tools, a.kit.DeviceKits()...)
		}
	}
	svr.AddTools(tools...)
	return svr
}

func (a *App) providerListTool() mcpserver.ServerTool {
	return mcpserver.ServerTool{
		Tool: mcp.NewTool("lazycat_mcp_provider_list",
			mcp.WithDescription("List MCP providers available through this program's gateway endpoints."),
		),
		Handler: func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			providers, err := a.providers.EnabledPublic(ctx)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			aggregated := a.aggregatedSlugs()
			errors := a.aggregateErrors()
			for i := range providers {
				providers[i].Kind = "mcp"
				providers[i].AggregateOK = aggregated[providers[i].Slug]
				providers[i].AggregateError = errors[providers[i].Slug]
				if skill := skillContentBySlug(providers[i].Slug); skill != nil {
					providers[i].Kind = "skill"
					providers[i].SkillTitle = skill.Title
					providers[i].SkillSummary = skill.Summary
					providers[i].SkillPrompts = append([]string(nil), skill.PromptExamples...)
				}
			}
			payload := map[string]any{
				"local": map[string]any{
					"name":      "LazyCat MCP",
					"endpoint":  "/mcp",
					"transport": "streamable_http",
				},
				"aggregate": map[string]any{
					"endpoint":    "/mcp",
					"transport":   "streamable_http",
					"tool_naming": "<provider_slug>__<upstream_tool_name>",
					"description": "Enabled upstream provider tools are exposed directly in this server's tools/list with a namespaced name.",
				},
				"providers": providers,
			}
			data, err := json.Marshal(payload)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(string(data)), nil
		},
	}
}
