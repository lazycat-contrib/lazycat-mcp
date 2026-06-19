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
		mcpserver.WithLogging(),
		mcpserver.WithRecovery(),
		mcpserver.WithToolCapabilities(true),
		mcpserver.WithResourceCapabilities(true, true),
	)

	tools := []mcpserver.ServerTool{a.providerListTool()}
	tools = append(tools, a.kit.DomainKits()...)
	if a.kit.Available() {
		tools = append(tools, a.kit.PowerKits()...)
		tools = append(tools, a.kit.DeviceKits()...)
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
			payload := map[string]any{
				"local": map[string]any{
					"name":      "LazyCat MCP",
					"endpoint":  "/mcp",
					"transport": "streamable_http",
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
