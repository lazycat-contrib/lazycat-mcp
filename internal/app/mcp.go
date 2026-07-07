package app

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

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
		mcpserver.WithToolFilter(a.filterBuiltinToolsByRole),
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
			providers, err := a.providers.EnabledPublicForOwner(ctx, mcpTokenOwnerFromContext(ctx), mcpTokenAdminFromContext(ctx))
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

type lazycatRoleContextKey struct{}
type mcpTokenOwnerContextKey struct{}
type mcpTokenAdminContextKey struct{}

func (a *App) contextWithLazycatRole(ctx context.Context, r *http.Request) context.Context {
	if r == nil {
		return ctx
	}
	if a != nil && a.isLazycatAdminRequest(r) {
		return context.WithValue(ctx, lazycatRoleContextKey{}, "admin")
	}
	return context.WithValue(ctx, lazycatRoleContextKey{}, currentLazycatUserRole(r))
}

func lazycatRoleFromContext(ctx context.Context) string {
	role, _ := ctx.Value(lazycatRoleContextKey{}).(string)
	return role
}

func contextWithMCPToken(ctx context.Context, token TokenDTO) context.Context {
	ctx = context.WithValue(ctx, mcpTokenOwnerContextKey{}, token.OwnerUserID)
	ctx = context.WithValue(ctx, mcpTokenAdminContextKey{}, token.OwnerIsAdmin)
	return ctx
}

func mcpTokenOwnerFromContext(ctx context.Context) string {
	owner, _ := ctx.Value(mcpTokenOwnerContextKey{}).(string)
	return owner
}

func mcpTokenAdminFromContext(ctx context.Context) bool {
	admin, _ := ctx.Value(mcpTokenAdminContextKey{}).(bool)
	return admin
}

func isLazycatAdminRole(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "admin", "administrator", "owner", "superadmin", "super_admin", "root":
		return true
	default:
		return false
	}
}

func (a *App) filterBuiltinToolsByRole(ctx context.Context, tools []mcp.Tool) []mcp.Tool {
	if mcpTokenAdminFromContext(ctx) || isLazycatAdminRole(lazycatRoleFromContext(ctx)) {
		return tools
	}
	ownerUserID := mcpTokenOwnerFromContext(ctx)
	out := tools[:0]
	for _, tool := range tools {
		if isAdminOnlyBuiltinToolName(tool.Name) {
			continue
		}
		if ref, ok := a.upstreamRefByAggregateName(tool.Name); ok {
			if ownerUserID == "" || !a.providerOwnedBy(ctx, ref.ProviderSlug, ownerUserID) {
				continue
			}
		}
		out = append(out, tool)
	}
	return out
}

func isAdminOnlyBuiltinToolName(name string) bool {
	name = strings.TrimSpace(name)
	switch name {
	case "lazycat_mcp_provider_list", "domain_base_info_lookup", "skill_prompt", "lazycat_device_query", "lazycat_device_notify", "lazycat_power":
		return true
	}
	for _, suffix := range []string{
		"__lazycat_mcp_provider_list",
		"__domain_base_info_lookup",
		"__skill_prompt",
		"__lazycat_device_query",
		"__lazycat_device_notify",
		"__lazycat_power",
	} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func (a *App) upstreamRefByAggregateName(name string) (upstreamToolRef, bool) {
	if a == nil {
		return upstreamToolRef{}, false
	}
	a.upstreamToolMu.RLock()
	defer a.upstreamToolMu.RUnlock()
	ref, ok := a.upstreamToolRefs[name]
	return ref, ok
}

func (a *App) providerOwnedBy(ctx context.Context, slug, ownerUserID string) bool {
	if a == nil || a.providers == nil || ownerUserID == "" {
		return false
	}
	provider, err := a.providers.GetBySlug(ctx, slug)
	if err != nil {
		return false
	}
	return provider.OwnerUserID == ownerUserID
}
