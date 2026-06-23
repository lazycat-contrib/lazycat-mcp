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
	appIDs := index.AppIDs()
	appsByID := a.lazycatAppsByID(r)

	out := make([]appOption, 0, len(appIDs))
	for _, appID := range appIDs {
		if appID == "system" || strings.HasPrefix(appID, ".") {
			continue
		}
		if !appIDPattern.MatchString(appID) {
			continue
		}
		info := appsByID[appID]
		item := appOption{
			AppID:               appID,
			Title:               appID,
			HasMCP:              len(index.MCPByApp[appID]) > 0,
			HasSkills:           len(index.SkillsByApp[appID]) > 0,
			DefaultSlug:         appID,
			DefaultEndpoint:     index.DefaultMCPEndpoint(appID),
			DefaultMCPResource:  index.DefaultMCPResourceID(appID),
			MCPProviders:        index.MCPByApp[appID],
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
	tokens, err := a.tokens.List(r.Context())
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
	token, err := a.tokens.Create(r.Context(), req.Name, expiresAt)
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
	providers, err := a.providers.List(r.Context())
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	aggregated := a.aggregatedSlugs()
	errs := a.aggregateErrors()
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
	writeJSON(w, http.StatusOK, map[string]any{"providers": providers})
}

func (a *App) handleCreateProvider(w http.ResponseWriter, r *http.Request) {
	var input ProviderInput
	if err := readJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
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
		provider, err := a.providers.Update(r.Context(), id, input)
		if err != nil {
			writeAPIError(w, statusFromProviderError(err), err.Error())
			return
		}
		a.refreshUpstreamToolsBestEffort(r.Context())
		writeJSON(w, http.StatusOK, map[string]any{"provider": provider})
	case http.MethodDelete:
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
