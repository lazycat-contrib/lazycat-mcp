package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"gitee.com/linakesi/lzc-sdk/lang/go/common"
	"gitee.com/linakesi/lzc-sdk/lang/go/sys"
	"google.golang.org/grpc/metadata"

	"lazycat-mcp/internal/buildinfo"
)

type appOption struct {
	AppID               string          `json:"app_id"`
	Title               string          `json:"title"`
	Icon                string          `json:"icon,omitempty"`
	Domain              string          `json:"domain,omitempty"`
	Subdomain           string          `json:"subdomain,omitempty"`
	DeployID            string          `json:"deploy_id,omitempty"`
	Owner               string          `json:"owner,omitempty"`
	Status              string          `json:"status,omitempty"`
	InstanceStatus      string          `json:"instance_status,omitempty"`
	MultiInstance       bool            `json:"multi_instance"`
	HasMCP              bool            `json:"has_mcp"`
	HasSkills           bool            `json:"has_skills"`
	DefaultSlug         string          `json:"default_slug"`
	DefaultEndpoint     string          `json:"default_endpoint,omitempty"`
	DefaultMCPResource  string          `json:"default_mcp_resource,omitempty"`
	MCPProviders        []MCPResource   `json:"mcp_providers,omitempty"`
	Skills              []SkillResource `json:"skills,omitempty"`
	SuggestedPublicPath string          `json:"suggested_public_path"`
}

func (a *App) handleAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api")
	switch {
	case path == "/status" && r.Method == http.MethodGet:
		a.handleStatus(w, r)
	case path == "/apps" && r.Method == http.MethodGet:
		a.handleApps(w, r)
	case path == "/tokens" && r.Method == http.MethodGet:
		a.handleListTokens(w, r)
	case path == "/tokens" && r.Method == http.MethodPost:
		a.handleCreateToken(w, r)
	case strings.HasPrefix(path, "/tokens/"):
		a.handleTokenByID(w, r, strings.TrimPrefix(path, "/tokens/"))
	case path == "/providers" && r.Method == http.MethodGet:
		a.handleListProviders(w, r)
	case path == "/providers" && r.Method == http.MethodPost:
		a.handleCreateProvider(w, r)
	case path == "/providers/batch" && r.Method == http.MethodPost:
		a.handleBatchProviders(w, r)
	case strings.HasPrefix(path, "/providers/"):
		a.handleProviderByID(w, r, strings.TrimPrefix(path, "/providers/"))
	case path == "/mcp-logs" && r.Method == http.MethodGet:
		a.handleListMCPLogs(w, r)
	case path == "/mcp-logs" && r.Method == http.MethodDelete:
		a.handleClearMCPLogs(w, r)
	case path == "/mcp-logs/cleanup" && r.Method == http.MethodPost:
		a.handleCleanupMCPLogs(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (a *App) handleStatus(w http.ResponseWriter, r *http.Request) {
	build := buildinfo.Snapshot()
	hasTicket := false
	if _, ok := a.tickets.Get(); ok {
		hasTicket = true
	}
	tokenCount, _ := a.db.MCPToken.Query().Count(r.Context())
	providerCount, _ := a.db.UpstreamProvider.Query().Count(r.Context())
	mcpLogCount, _ := a.db.MCPCallLog.Query().Count(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":                 selfPackageID,
		"resource_root":          a.resources.Root(),
		"mcp_endpoint":           "/mcp",
		"skill_install_path":     SelfSkillInstallPath(),
		"version":                build.Version,
		"commit":                 build.Commit,
		"build_time":             build.BuildTime,
		"has_user_ticket":        hasTicket,
		"user_ticket_seen":       a.tickets.UpdatedAt(),
		"token_count":            tokenCount,
		"provider_count":         providerCount,
		"mcp_log_count":          mcpLogCount,
		"mcp_log_retention_days": a.cfg.MCPLogRetentionDays,
	})
}

func (a *App) handleApps(w http.ResponseWriter, r *http.Request) {
	index := a.resources.Scan(r.Context())
	resourceAppIDs := index.AppIDs()
	appsByID := a.lazycatAppsByID(r)

	out := make([]appOption, 0, len(resourceAppIDs)+1)

	// Only expose resources belonging to apps visible to the current LazyCat user.
	// Resource files under /lzcapp/run/resources are global to the app container;
	// listing them directly leaks other users' installed MCP/Skill apps.
	visibleAppIDs := make([]string, 0, len(resourceAppIDs)+1)
	seen := make(map[string]struct{}, len(resourceAppIDs)+1)
	for _, appID := range resourceAppIDs {
		if appID == selfPackageID || appsByID[appID] != nil {
			visibleAppIDs = append(visibleAppIDs, appID)
			seen[appID] = struct{}{}
		}
	}
	// Only admins see the built-in LazyCat MCP resource in the management list.
	if a.isLazycatAdminRequest(r) {
		if _, ok := seen[selfPackageID]; !ok {
			visibleAppIDs = append(visibleAppIDs, selfPackageID)
		}
	}

	for _, appID := range visibleAppIDs {
		if appID == "system" || strings.HasPrefix(appID, ".") {
			continue
		}
		if !appIDPattern.MatchString(appID) {
			continue
		}
		info := appsByID[appID]
		mcpProviders := index.MCPByApp[appID]
		// Self app: inject synthetic MCP resource so built-in tools are publishable.
		if appID == selfPackageID {
			toolNames := a.selfToolNamesForRequest(r)
			if len(mcpProviders) == 0 {
				mcpProviders = []MCPResource{{
					AppID:      selfPackageID,
					ResourceID: "default",
					Endpoint:   "/mcp",
					ToolNames:  toolNames,
				}}
			} else {
				for i := range mcpProviders {
					mcpProviders[i].ToolNames = toolNames
				}
			}
		}
		item := appOption{
			AppID:               appID,
			Title:               appID,
			HasMCP:              len(mcpProviders) > 0,
			HasSkills:           len(index.SkillsByApp[appID]) > 0,
			DefaultSlug:         appID,
			DefaultEndpoint:     firstNonEmpty(index.DefaultMCPEndpoint(appID), "/mcp"),
			DefaultMCPResource:  index.DefaultMCPResourceID(appID),
			MCPProviders:        mcpProviders,
			Skills:              index.SkillsByApp[appID],
			SuggestedPublicPath: "/mcp/apps/" + appID,
		}
		if info != nil {
			item.Title = firstNonEmpty(info.GetTitle(), appID)
			item.Icon = info.GetIcon()
			item.Domain = info.GetDomain()
			item.Subdomain = info.GetSubdomain()
			item.DeployID = info.GetDeployId()
			item.Owner = info.GetOwner()
			item.Status = info.GetStatus().String()
			item.InstanceStatus = info.GetInstanceStatus().String()
			item.MultiInstance = info.GetMultiInstance()
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
	})
	writeJSON(w, http.StatusOK, map[string]any{"apps": out})
}

func (a *App) lazycatAppsByID(r *http.Request) map[string]*sys.AppInfo {
	out := make(map[string]*sys.AppInfo)
	if a.gateway == nil {
		return out
	}
	ctx, cancel := context.WithTimeout(lazycatContextFromRequest(r), 10*time.Second)
	defer cancel()
	resp, err := a.gateway.PkgManager.QueryApplication(ctx, &sys.QueryApplicationRequest{})
	if err != nil {
		a.logger.Warn().Err(err).Msg("query lazycat applications failed")
		return out
	}
	for _, info := range resp.GetInfoList() {
		out[info.GetAppid()] = info
	}
	return out
}

func lazycatContextFromRequest(r *http.Request) context.Context {
	ctx := r.Context()
	var pairs []string
	for _, key := range []string{
		"x-hc-user-id",
		"x-hc-user-role",
		"x-hc-device-id",
		"x-hc-device-version",
		"x-hc-user-ticket",
	} {
		if value := r.Header.Get(key); value != "" {
			pairs = append(pairs, key, value)
		}
	}
	if len(pairs) == 0 {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, pairs...)
}

func (a *App) handleListTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := a.tokens.ListForOwner(r.Context(), currentLazycatUserID(r), a.isLazycatAdminRequest(r))
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": tokens})
}

func (a *App) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string `json:"name"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := readJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	expiresAt, err := parseOptionalTime(req.ExpiresAt)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	ownerUserID := currentLazycatUserID(r)
	if ownerUserID == "" {
		writeAPIError(w, http.StatusForbidden, "current lazycat user is required")
		return
	}
	token, err := a.tokens.Create(r.Context(), req.Name, expiresAt, ownerUserID, a.isLazycatAdminRequest(r))
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"token": token})
}

func (a *App) handleTokenByID(w http.ResponseWriter, r *http.Request, rawID string) {
	id, err := strconv.Atoi(strings.Trim(rawID, "/"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid token id")
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var req struct {
			Name           *string `json:"name"`
			Enabled        *bool   `json:"enabled"`
			ExpiresAt      *string `json:"expires_at"`
			ClearExpiresAt bool    `json:"clear_expires_at"`
		}
		if err := readJSON(r, &req); err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
		var expiresAt *time.Time
		if req.ExpiresAt != nil {
			expiresAt, err = parseOptionalTime(*req.ExpiresAt)
			if err != nil {
				writeAPIError(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		token, err := a.tokens.Update(r.Context(), id, req.Name, req.Enabled, expiresAt, req.ClearExpiresAt)
		if err != nil {
			writeAPIError(w, statusFromEntError(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"token": token})
	case http.MethodDelete:
		if err := a.tokens.Delete(r.Context(), id); err != nil {
			writeAPIError(w, statusFromEntError(err), err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) handleListProviders(w http.ResponseWriter, r *http.Request) {
	visibleApps := a.visibleLazycatAppIDs(r)
	providers, err := a.providers.ListForOwner(r.Context(), currentLazycatUserID(r))
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	aggregated := a.aggregatedSlugs()
	errs := a.aggregateErrors()
	providers = filterProvidersByVisibleApps(providers, visibleApps)
	if !a.isLazycatAdminRequest(r) {
		filtered := providers[:0]
		for _, provider := range providers {
			if provider.AppID == selfPackageID {
				continue
			}
			filtered = append(filtered, provider)
		}
		providers = filtered
	}
	for i := range providers {
		providers[i].AggregateOK = aggregated[providers[i].Slug]
		providers[i].AggregateError = errs[providers[i].Slug]
		if skill := skillContentBySlug(providers[i].Slug); skill != nil {
			providers[i].Kind = "skill"
			providers[i].SkillTitle = skill.Title
			providers[i].SkillSummary = skill.Summary
			providers[i].SkillPrompts = append([]string(nil), skill.PromptExamples...)
		} else {
			providers[i].Kind = "mcp"
		}
	}
	// Populate upstream tool names from live tool refs.
	a.upstreamToolMu.RLock()
	for i := range providers {
		var names []string
		if providers[i].AppID == selfPackageID {
			names = a.selfToolNamesForRequest(r)
		} else {
			for _, ref := range a.upstreamToolRefs {
				if ref.ProviderSlug == providers[i].Slug {
					names = append(names, ref.UpstreamName)
				}
			}
		}
		if len(names) > 0 {
			providers[i].UpstreamToolNames = names
		}
	}
	a.upstreamToolMu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{"providers": providers})
}

func (a *App) handleCreateProvider(w http.ResponseWriter, r *http.Request) {
	var input ProviderInput
	if err := readJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	input.OwnerUserID = currentLazycatUserID(r)
	if err := a.validateProviderVisibleForRequest(r, input); err != nil {
		writeAPIError(w, http.StatusForbidden, err.Error())
		return
	}
	provider, err := a.providers.Create(r.Context(), input)
	if err != nil {
		writeAPIError(w, statusFromProviderError(err), err.Error())
		return
	}
	a.refreshUpstreamToolsBestEffort(r.Context())
	writeJSON(w, http.StatusCreated, map[string]any{"provider": provider})
}

func (a *App) handleProviderByID(w http.ResponseWriter, r *http.Request, rawID string) {
	id, err := strconv.Atoi(strings.Trim(rawID, "/"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid provider id")
		return
	}
	switch r.Method {
	case http.MethodPatch, http.MethodPut:
		var input ProviderInput
		if err := readJSON(r, &input); err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := a.validateProviderUpdateVisibleForRequest(r, id, input); err != nil {
			writeAPIError(w, http.StatusForbidden, err.Error())
			return
		}
		provider, err := a.providers.Update(r.Context(), id, input)
		if err != nil {
			writeAPIError(w, statusFromProviderError(err), err.Error())
			return
		}
		a.refreshUpstreamToolsBestEffort(r.Context())
		writeJSON(w, http.StatusOK, map[string]any{"provider": provider})
	case http.MethodDelete:
		if err := a.validateProviderIDVisibleForRequest(r, id); err != nil {
			writeAPIError(w, http.StatusForbidden, err.Error())
			return
		}
		if err := a.providers.Delete(r.Context(), id); err != nil {
			writeAPIError(w, statusFromEntError(err), err.Error())
			return
		}
		a.refreshUpstreamToolsBestEffort(r.Context())
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) handleListMCPLogs(w http.ResponseWriter, r *http.Request) {
	if a.mcpLogs == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "mcp call log service is unavailable")
		return
	}
	limit := defaultCallLogLimit
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid log limit")
			return
		}
		limit = parsed
	}
	logs, err := a.mcpLogs.List(r.Context(), MCPCallLogFilter{
		Limit:        limit,
		Source:       r.URL.Query().Get("source"),
		Status:       r.URL.Query().Get("status"),
		ProviderSlug: r.URL.Query().Get("provider_slug"),
	})
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"logs":           logs,
		"retention_days": a.cfg.MCPLogRetentionDays,
	})
}

func (a *App) handleClearMCPLogs(w http.ResponseWriter, r *http.Request) {
	if a.mcpLogs == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "mcp call log service is unavailable")
		return
	}
	deleted, err := a.mcpLogs.Clear(r.Context())
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})
}

func (a *App) handleCleanupMCPLogs(w http.ResponseWriter, r *http.Request) {
	if a.mcpLogs == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "mcp call log service is unavailable")
		return
	}
	deleted, err := a.mcpLogs.Cleanup(r.Context(), time.Now())
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"deleted":        deleted,
		"retention_days": a.cfg.MCPLogRetentionDays,
	})
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeAPIError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"message": message,
			"status":  status,
		},
	})
}

func parseOptionalTime(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func statusFromProviderError(err error) int {
	if errors.Is(err, errProviderInvalid) {
		return http.StatusBadRequest
	}
	return statusFromEntError(err)
}

func statusFromEntError(err error) int {
	if err == nil {
		return http.StatusOK
	}
	if strings.Contains(err.Error(), "not found") {
		return http.StatusNotFound
	}
	if strings.Contains(err.Error(), "constraint failed") || strings.Contains(err.Error(), "UNIQUE constraint") {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

func (a *App) handleBatchProviders(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs       []int  `json:"ids"`
		Action    string `json:"action"`
		Transport string `json:"transport,omitempty"`
	}
	if err := readJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.IDs) == 0 {
		writeAPIError(w, http.StatusBadRequest, "ids is required")
		return
	}
	if len(req.IDs) > 100 {
		writeAPIError(w, http.StatusBadRequest, "batch limit is 100")
		return
	}

	type result struct {
		ID     int    `json:"id"`
		Status string `json:"status"`
		Error  string `json:"error,omitempty"`
	}
	var results []result

	for _, id := range req.IDs {
		res := result{ID: id, Status: "ok"}
		if err := a.validateProviderIDVisibleForRequest(r, id); err != nil {
			res.Status = "error"
			res.Error = err.Error()
			results = append(results, res)
			continue
		}
		switch req.Action {
		case "enable":
			input := ProviderInput{Enabled: boolPtr(true)}
			if _, err := a.providers.Update(r.Context(), id, input); err != nil {
				res.Status = "error"
				res.Error = err.Error()
			}
		case "disable":
			input := ProviderInput{Enabled: boolPtr(false)}
			if _, err := a.providers.Update(r.Context(), id, input); err != nil {
				res.Status = "error"
				res.Error = err.Error()
			}
		case "delete":
			if err := a.providers.Delete(r.Context(), id); err != nil {
				res.Status = "error"
				res.Error = err.Error()
			}
		case "update_transport":
			transport := strings.TrimSpace(req.Transport)
			if transport == "" {
				transport = "streamable_http"
			}
			input := ProviderInput{Transport: transport}
			if _, err := a.providers.Update(r.Context(), id, input); err != nil {
				res.Status = "error"
				res.Error = err.Error()
			}
		default:
			res.Status = "error"
			res.Error = "unknown action: " + req.Action
		}
		results = append(results, res)
	}

	a.refreshUpstreamToolsBestEffort(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func currentLazycatUserID(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("X-HC-USER-ID"))
}

func currentLazycatUserRole(r *http.Request) string {
	for _, key := range []string{
		"X-HC-USER-ROLE",
		"X-HC-User-Role",
		"X-Hc-User-Role",
		"X-Lzc-User-Role",
		"X-Lazycat-User-Role",
	} {
		if value := strings.TrimSpace(r.Header.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func (a *App) visibleLazycatAppIDs(r *http.Request) map[string]bool {
	visible := map[string]bool{selfPackageID: true}
	for appID := range a.lazycatAppsByID(r) {
		visible[appID] = true
	}
	return visible
}

func filterProvidersByVisibleApps(providers []ProviderDTO, visibleApps map[string]bool) []ProviderDTO {
	out := providers[:0]
	for _, p := range providers {
		if p.Type == "lazycat" {
			if !visibleApps[p.AppID] {
				continue
			}
		}
		out = append(out, p)
	}
	return out
}

func (a *App) validateProviderVisibleForRequest(r *http.Request, input ProviderInput) error {
	ownerUserID := currentLazycatUserID(r)
	if ownerUserID == "" {
		return errors.New("current lazycat user is required")
	}
	if strings.TrimSpace(input.OwnerUserID) != "" && strings.TrimSpace(input.OwnerUserID) != ownerUserID {
		return errors.New("provider owner does not match current user")
	}
	if strings.TrimSpace(input.Type) != "" && strings.TrimSpace(input.Type) != "lazycat" {
		return nil
	}
	appID := strings.TrimSpace(input.AppID)
	if appID == "" {
		return nil
	}
	if appID == selfPackageID && !a.isLazycatAdminRequest(r) {
		return errors.New("built-in provider is admin-only")
	}
	if !a.visibleLazycatAppIDs(r)[appID] {
		return errors.New("provider app is not visible to current user")
	}
	return nil
}

func (a *App) validateProviderIDVisibleForRequest(r *http.Request, id int) error {
	ownerUserID := currentLazycatUserID(r)
	if ownerUserID == "" {
		return errors.New("current lazycat user is required")
	}
	provider, err := a.providers.Get(r.Context(), id)
	if err != nil {
		return err
	}
	if provider.OwnerUserID != ownerUserID {
		return errors.New("provider is not owned by current user")
	}
	if provider.AppID == selfPackageID && !a.isLazycatAdminRequest(r) {
		return errors.New("built-in provider is admin-only")
	}
	if provider.Type == "lazycat" && !a.visibleLazycatAppIDs(r)[provider.AppID] {
		return errors.New("provider is not visible to current user")
	}
	return nil
}

func (a *App) validateProviderUpdateVisibleForRequest(r *http.Request, id int, input ProviderInput) error {
	if err := a.validateProviderIDVisibleForRequest(r, id); err != nil {
		return err
	}
	return a.validateProviderVisibleForRequest(r, input)
}

func boolPtr(v bool) *bool { return &v }

func (a *App) cleanupOrphanProviders(ctx context.Context, installed map[string]*sys.AppInfo) {
	providers, err := a.providers.List(ctx)
	if err != nil {
		return
	}
	for _, p := range providers {
		if p.Type != "lazycat" || p.AppID == "" || p.AppID == selfPackageID {
			continue
		}
		if _, ok := installed[p.AppID]; !ok {
			if a.logger != nil {
				a.logger.Info().Str("slug", p.Slug).Str("app_id", p.AppID).Msg("auto-cleaning orphan provider")
			}
			_ = a.providers.Delete(ctx, p.ID)
		}
	}
}

func (a *App) selfToolNamesForRequest(r *http.Request) []string {
	return a.selfToolNames(a.isLazycatAdminRequest(r))
}

func (a *App) isLazycatAdminRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	if isLazycatAdminRole(currentLazycatUserRole(r)) {
		return true
	}
	userID := currentLazycatUserID(r)
	if userID == "" {
		return false
	}
	return a.lazycatUserIsAdmin(lazycatContextFromRequest(r), userID)
}

func (a *App) lazycatUserIsAdmin(ctx context.Context, userID string) bool {
	userID = strings.TrimSpace(userID)
	if a == nil || a.gateway == nil || userID == "" {
		return false
	}
	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	info, err := a.gateway.Users.QueryUserInfo(queryCtx, &common.UserID{Uid: userID})
	return err == nil && info.GetRole() == common.Role_ROLE_ADMIN
}

func (a *App) selfToolNames(admin bool) []string {
	if !admin {
		return []string{}
	}
	names := []string{
		"lazycat_mcp_provider_list",
		"domain_base_info_lookup",
		"skill_prompt",
	}
	if a.kit != nil && a.kit.Available() {
		names = append(names, "lazycat_device_query", "lazycat_device_notify", "lazycat_power")
	}
	return names
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
