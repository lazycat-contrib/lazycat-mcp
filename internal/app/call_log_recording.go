package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"lazycat-mcp/ent/mcpcalllog"
)

const mcpCallLogWriteTimeout = 2 * time.Second

func (a *App) mcpCallLogToolMiddleware() mcpserver.ToolHandlerMiddleware {
	return func(next mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc {
		return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			start := time.Now()
			result, err := next(ctx, request)
			source := mcpcalllog.SourceLocal.String()
			target := request.Params.Name
			providerSlug := ""
			if ref, ok := a.upstreamToolRef(request.Params.Name); ok {
				source = mcpcalllog.SourceUpstream.String()
				target = ref.UpstreamName
				providerSlug = ref.ProviderSlug
			}
			status := mcpcalllog.StatusSuccess.String()
			errorText := ""
			if err != nil {
				status = mcpcalllog.StatusError.String()
				errorText = err.Error()
			} else if result != nil && result.IsError {
				status = mcpcalllog.StatusError.String()
				errorText = callToolResultError(result)
			}
			a.recordMCPCall(ctx, MCPCallLogInput{
				Source:       source,
				Transport:    mcpcalllog.TransportStreamableHTTP.String(),
				Method:       request.Params.Name,
				Target:       target,
				ProviderSlug: providerSlug,
				TokenPrefix:  tokenPrefixFromHeader(request.Header),
				SessionID:    request.Header.Get("Mcp-Session-Id"),
				RequestID:    firstHeader(request.Header, "X-Request-Id", "X-Correlation-Id"),
				Status:       status,
				Duration:     time.Since(start),
				Error:        errorText,
			})
			return result, err
		}
	}
}

func (a *App) withMCPProxyLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		statusCode := recorder.statusCode()
		status := mcpcalllog.StatusSuccess.String()
		errorText := ""
		if statusCode >= 400 {
			status = mcpcalllog.StatusError.String()
			errorText = fmt.Sprintf("status %d", statusCode)
		}
		slug := providerSlugFromPath(r.URL.Path)
		target := r.URL.Path
		if slug != "" {
			target = "/mcp/apps/" + slug
		}
		a.recordMCPCall(r.Context(), MCPCallLogInput{
			Source:       mcpcalllog.SourceUpstream.String(),
			Transport:    mcpcalllog.TransportHTTP.String(),
			Method:       r.Method,
			Target:       target,
			ProviderSlug: slug,
			TokenPrefix:  tokenPrefixFromRequest(r),
			SessionID:    r.Header.Get("Mcp-Session-Id"),
			RequestID:    firstHeader(r.Header, "X-Request-Id", "X-Correlation-Id"),
			Status:       status,
			StatusCode:   &statusCode,
			Duration:     time.Since(start),
			Error:        errorText,
		})
	})
}

func (a *App) recordMCPCall(ctx context.Context, input MCPCallLogInput) {
	if a == nil || a.mcpLogs == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mcpCallLogWriteTimeout)
	defer cancel()
	if _, err := a.mcpLogs.Record(recordCtx, input); err != nil && a.logger != nil {
		a.logger.Debug().Err(err).Msg("record mcp call log failed")
	}
}

func (a *App) startMCPLogCleanup(ctx context.Context) {
	if a.mcpLogs == nil {
		return
	}
	a.cleanupMCPLogs(ctx)
	loopCtx, cancel := context.WithCancel(context.Background())
	a.cleanupCancel = cancel
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				a.cleanupMCPLogs(loopCtx)
			case <-loopCtx.Done():
				return
			}
		}
	}()
}

func (a *App) cleanupMCPLogs(ctx context.Context) {
	if a == nil || a.mcpLogs == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	deleted, err := a.mcpLogs.Cleanup(cleanupCtx, time.Now())
	if err != nil {
		if a.logger != nil {
			a.logger.Warn().Err(err).Msg("cleanup mcp call logs failed")
		}
		return
	}
	if deleted > 0 && a.logger != nil {
		a.logger.Info().Int("deleted", deleted).Msg("cleaned mcp call logs")
	}
}

func callToolResultError(result *mcp.CallToolResult) string {
	if result == nil {
		return "tool returned error"
	}
	for _, content := range result.Content {
		if text, ok := content.(mcp.TextContent); ok && strings.TrimSpace(text.Text) != "" {
			return text.Text
		}
	}
	return "tool returned error"
}

func tokenPrefixFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	return tokenPrefixFromHeader(r.Header)
}

func tokenPrefixFromHeader(header http.Header) string {
	auth := strings.TrimSpace(header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return tokenPrefix(strings.TrimSpace(auth[7:]))
	}
	token := strings.TrimSpace(header.Get("X-MCP-Token"))
	if token == "" {
		return ""
	}
	return tokenPrefix(token)
}

func firstHeader(header http.Header, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(header.Get(name)); value != "" {
			return value
		}
	}
	return ""
}

func providerSlugFromPath(requestPath string) string {
	const prefix = "/mcp/apps/"
	if !strings.HasPrefix(requestPath, prefix) {
		return ""
	}
	remaining := strings.TrimPrefix(requestPath, prefix)
	if remaining == "" {
		return ""
	}
	parts := strings.SplitN(remaining, "/", 2)
	if parts[0] == "" || strings.Contains(parts[0], "..") {
		return ""
	}
	return parts[0]
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.status != 0 {
		return
	}
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(data)
}

func (r *statusRecorder) Flush() {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

func (r *statusRecorder) Push(target string, opts *http.PushOptions) error {
	pusher, ok := r.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, opts)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (r *statusRecorder) statusCode() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}
