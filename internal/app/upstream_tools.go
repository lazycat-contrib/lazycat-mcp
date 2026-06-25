package app

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"lazycat-mcp/ent"
	"lazycat-mcp/ent/upstreamprovider"
	"lazycat-mcp/internal/buildinfo"
	"lazycat-mcp/internal/proxy"
)

const (
	upstreamToolRefreshTimeout            = 60 * time.Second
	upstreamToolRefreshPerProviderTimeout = 10 * time.Second
	upstreamToolCallTimeout               = 2 * time.Minute
)

var aggregateToolPartPattern = regexp.MustCompile(`[^A-Za-z0-9_]+`)

type upstreamToolRef struct {
	AggregateName string
	ProviderSlug  string
	ProviderName  string
	UpstreamName  string
}

func (a *App) refreshUpstreamToolsAsync() {
	if a == nil || a.mcpServer == nil || a.providers == nil {
		return
	}
	go func() {
		a.refreshUpstreamToolsBestEffort(context.Background())
	}()
}

func (a *App) refreshUpstreamToolsBestEffort(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), upstreamToolRefreshTimeout)
	defer cancel()
	if err := a.refreshUpstreamTools(refreshCtx); err != nil && a.logger != nil {
		a.logger.Warn().Err(err).Msg("refresh upstream mcp tools failed")
	}
}

func (a *App) refreshUpstreamTools(ctx context.Context) error {
	if a == nil || a.mcpServer == nil || a.providers == nil {
		return nil
	}
	providers, err := a.providers.Enabled(ctx)
	if err != nil {
		return err
	}
	skillOnlySlugs := a.skillOnlyProviderSlugs(ctx, providers)

	a.upstreamToolMu.RLock()
	oldRefs := make(map[string]upstreamToolRef, len(a.upstreamToolRefs))
	for name, ref := range a.upstreamToolRefs {
		oldRefs[name] = ref
	}
	a.upstreamToolMu.RUnlock()

	activeSlugs := make(map[string]bool, len(providers))
	successSlugs := make(map[string]bool, len(providers))
	a.upstreamFailureReasons = make(map[string]string, len(providers))
	providerViews := make([]*ProviderDTOView, 0, len(providers))
	added := make([]mcpserver.ServerTool, 0)
	newRefsByName := make(map[string]upstreamToolRef)
	usedNames := a.localToolNames()
	for name := range oldRefs {
		usedNames[name] = true
	}
	for _, provider := range providers {
		activeSlugs[provider.Slug] = true
		providerViews = append(providerViews, &ProviderDTOView{Slug: provider.Slug, AppID: provider.AppID, ResourceID: derefString(provider.ResourceID)})
		if skillOnlySlugs[provider.Slug] {
			successSlugs[provider.Slug] = true
			delete(a.upstreamFailureReasons, provider.Slug)
			continue
		}
		if provider.Transport != upstreamprovider.TransportStreamableHTTP {
			successSlugs[provider.Slug] = true
			delete(a.upstreamFailureReasons, provider.Slug)
			continue
		}
		perProviderCtx, perProviderCancel := context.WithTimeout(ctx, upstreamToolRefreshPerProviderTimeout)
		tools, err := a.listUpstreamTools(perProviderCtx, provider)
		perProviderCancel()
		if err != nil {
			if a.logger != nil {
				a.logger.Warn().Err(err).Str("provider", provider.Slug).Msg("list upstream mcp tools failed")
			}
			a.upstreamFailureReasons[provider.Slug] = err.Error()
			continue
		}
		successSlugs[provider.Slug] = true
		delete(a.upstreamFailureReasons, provider.Slug)
		for name, ref := range oldRefs {
			if ref.ProviderSlug == provider.Slug {
				delete(usedNames, name)
			}
		}
		for _, tool := range tools {
			if strings.TrimSpace(tool.Name) == "" {
				continue
			}
			ref := upstreamToolRef{
				AggregateName: uniqueAggregateToolName(provider.Slug, tool.Name, usedNames),
				ProviderSlug:  provider.Slug,
				ProviderName:  provider.Name,
				UpstreamName:  tool.Name,
			}
			tool.Name = ref.AggregateName
			tool.Description = aggregateToolDescription(provider, ref.UpstreamName, tool.Description)
			toolCopy := tool
			refCopy := ref
			added = append(added, mcpserver.ServerTool{
				Tool: toolCopy,
				Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return a.callUpstreamTool(ctx, refCopy, request)
				},
			})
			newRefsByName[ref.AggregateName] = ref
		}
	}

	finalRefs := make(map[string]upstreamToolRef)
	removeNames := make([]string, 0)
	for name, ref := range oldRefs {
		switch {
		case !activeSlugs[ref.ProviderSlug]:
			removeNames = append(removeNames, name)
		case successSlugs[ref.ProviderSlug]:
			removeNames = append(removeNames, name)
		default:
			finalRefs[name] = ref
		}
	}
	for name, ref := range newRefsByName {
		finalRefs[name] = ref
	}

	a.upstreamToolMu.Lock()
	a.upstreamToolRefs = finalRefs
	a.upstreamHealthySlugs = make(map[string]bool, len(successSlugs))
	for slug, ok := range successSlugs {
		if ok {
			a.upstreamHealthySlugs[slug] = true
		}
	}
	a.upstreamToolMu.Unlock()

	skillErrors := a.refreshSkillStates(ctx, providerViews)
	for slug, reason := range skillErrors {
		a.upstreamFailureReasons[slug] = reason
	}
	a.registerSkillResources()

	if len(removeNames) > 0 {
		a.mcpServer.DeleteTools(removeNames...)
	}
	if len(added) > 0 {
		a.mcpServer.AddTools(added...)
	}
	return nil
}

func (a *App) listUpstreamTools(ctx context.Context, provider *ent.UpstreamProvider) ([]mcp.Tool, error) {
	client, err := a.newUpstreamMCPClient(provider, upstreamToolRefreshTimeout)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	if err := client.Start(ctx); err != nil {
		return nil, err
	}
	if _, err := client.Initialize(ctx, upstreamInitializeRequest()); err != nil {
		return nil, err
	}
	result, err := client.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, err
	}
	return result.Tools, nil
}

func (a *App) callUpstreamTool(ctx context.Context, ref upstreamToolRef, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	provider, err := a.providers.GetEnabledBySlug(ctx, ref.ProviderSlug)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if provider.Transport != upstreamprovider.TransportStreamableHTTP {
		return mcp.NewToolResultError("upstream tool aggregation only supports streamable_http providers"), nil
	}
	callCtx, cancel := context.WithTimeout(ctx, upstreamToolCallTimeout)
	defer cancel()
	client, err := a.newUpstreamMCPClient(provider, upstreamToolCallTimeout)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	defer client.Close()
	if err := client.Start(callCtx); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if _, err := client.Initialize(callCtx, upstreamInitializeRequest()); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	upstreamRequest := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      ref.UpstreamName,
			Arguments: request.Params.Arguments,
			Meta:      request.Params.Meta,
		},
	}
	result, err := client.CallTool(callCtx, upstreamRequest)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	a.providers.MarkUsed(context.WithoutCancel(ctx), provider.ID)
	return result, nil
}

func (a *App) newUpstreamMCPClient(provider *ent.UpstreamProvider, timeout time.Duration) (*client.Client, error) {
	target, headers, err := a.upstreamClientTarget(provider)
	if err != nil {
		return nil, err
	}
	headers = ensureStreamableHTTPAccept(headers)
	return client.NewStreamableHttpClient(target,
		transport.WithHTTPHeaders(headerMap(headers)),
		transport.WithHTTPTimeout(timeout),
	)
}

func (a *App) upstreamClientTarget(provider *ent.UpstreamProvider) (string, http.Header, error) {
	switch provider.ProviderType {
	case upstreamprovider.ProviderTypeLazycat:
		if a.tickets == nil {
			return "", nil, proxy.ErrTicketMissing
		}
		ticket, ok := a.tickets.Get()
		if !ok {
			return "", nil, proxy.ErrTicketMissing
		}
		target, err := proxy.LazyCatTargetURL(provider.AppID, provider.Endpoint, "", "")
		if err != nil {
			return "", nil, err
		}
		return target, proxy.HeadersForLazyCatUpstream(http.Header{}, ticket), nil
	case upstreamprovider.ProviderTypeCustom:
		target, err := proxy.CustomTargetURL(provider.BaseURL, provider.Endpoint, "", "")
		if err != nil {
			return "", nil, err
		}
		headers, err := proxy.HeadersForCustomUpstream(http.Header{}, provider.Headers)
		if err != nil {
			return "", nil, err
		}
		return target, headers, nil
	default:
		return "", nil, fmt.Errorf("unsupported provider type: %s", provider.ProviderType)
	}
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func (a *App) skillOnlyProviderSlugs(ctx context.Context, providers []*ent.UpstreamProvider) map[string]bool {
	out := make(map[string]bool)
	if a == nil || a.resources == nil {
		return out
	}
	index := a.resources.Scan(ctx)
	for _, provider := range providers {
		if strings.TrimSpace(provider.AppID) == "" {
			continue
		}
		hasSkill := len(index.SkillsByApp[provider.AppID]) > 0
		hasMCP := len(index.MCPByApp[provider.AppID]) > 0
		if hasSkill && !hasMCP {
			out[provider.Slug] = true
		}
	}
	return out
}

func (a *App) aggregatedSlugs() map[string]bool {
	out := make(map[string]bool)
	a.upstreamToolMu.RLock()
	defer a.upstreamToolMu.RUnlock()
	for slug, ok := range a.upstreamHealthySlugs {
		if ok {
			out[slug] = true
		}
	}
	return out
}

func (a *App) aggregateErrors() map[string]string {
	out := make(map[string]string, len(a.upstreamFailureReasons))
	for slug, reason := range a.upstreamFailureReasons {
		out[slug] = reason
	}
	return out
}

func ensureStreamableHTTPAccept(headers http.Header) http.Header {
	out := headers.Clone()
	accept := strings.TrimSpace(out.Get("Accept"))
	if accept == "" {
		out.Set("Accept", "application/json, text/event-stream")
		return out
	}
	lower := strings.ToLower(accept)
	hasJSON := strings.Contains(lower, "application/json")
	hasSSE := strings.Contains(lower, "text/event-stream")
	if !hasJSON || !hasSSE {
		out.Set("Accept", "application/json, text/event-stream")
	}
	return out
}

func (a *App) upstreamToolRef(name string) (upstreamToolRef, bool) {
	a.upstreamToolMu.RLock()
	defer a.upstreamToolMu.RUnlock()
	ref, ok := a.upstreamToolRefs[name]
	return ref, ok
}

func (a *App) localToolNames() map[string]bool {
	out := make(map[string]bool)
	if a.mcpServer == nil {
		return out
	}
	for name := range a.mcpServer.ListTools() {
		out[name] = true
	}
	a.upstreamToolMu.RLock()
	for name := range a.upstreamToolRefs {
		delete(out, name)
	}
	a.upstreamToolMu.RUnlock()
	return out
}

func upstreamInitializeRequest() mcp.InitializeRequest {
	info := buildinfo.Snapshot()
	return mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "lazycat-mcp",
				Version: info.Version,
			},
		},
	}
}

func headerMap(headers http.Header) map[string]string {
	out := make(map[string]string)
	for key, values := range headers {
		if len(values) == 0 {
			continue
		}
		out[key] = strings.Join(values, ",")
	}
	return out
}

func aggregateToolDescription(provider *ent.UpstreamProvider, upstreamName string, description string) string {
	description = strings.TrimSpace(description)
	prefix := fmt.Sprintf("Upstream MCP provider %q tool %q.", provider.Slug, upstreamName)
	if description == "" {
		return prefix
	}
	return prefix + " " + description
}

func uniqueAggregateToolName(providerSlug string, upstreamName string, used map[string]bool) string {
	base := sanitizeToolNamePart(providerSlug) + "__" + sanitizeToolNamePart(upstreamName)
	if base == "__" {
		base = "upstream__tool"
	}
	name := base
	for i := 2; used[name]; i++ {
		name = fmt.Sprintf("%s__%d", base, i)
	}
	used[name] = true
	return name
}

func sanitizeToolNamePart(value string) string {
	value = strings.Trim(aggregateToolPartPattern.ReplaceAllString(value, "_"), "_")
	if value == "" {
		return "tool"
	}
	return value
}
