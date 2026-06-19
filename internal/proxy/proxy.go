package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"lazycat-mcp/ent"
)

var (
	ErrProviderNotFound = errors.New("provider not found")
	ErrTicketMissing    = errors.New("lazycat user ticket is missing")
)

type ProviderResolver interface {
	GetEnabledBySlug(ctx context.Context, slug string) (*ent.UpstreamProvider, error)
	MarkUsed(ctx context.Context, id int)
}

type TicketSource interface {
	Get() (string, bool)
}

type Proxy struct {
	resolver ProviderResolver
	tickets  TicketSource
	client   *http.Client
}

func New(resolver ProviderResolver, tickets TicketSource) *Proxy {
	return &Proxy{
		resolver: resolver,
		tickets:  tickets,
		client: &http.Client{
			Timeout: 0,
		},
	}
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	slug, rest, ok := routeParts(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	provider, err := p.resolver.GetEnabledBySlug(r.Context(), slug)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, ErrProviderNotFound) {
			status = http.StatusNotFound
		}
		writeProxyError(w, status, err.Error())
		return
	}

	ticket, ok := p.tickets.Get()
	if !ok {
		writeProxyError(w, http.StatusPreconditionRequired, ErrTicketMissing.Error())
		return
	}

	target, err := targetURL(provider.AppID, provider.Endpoint, rest, r.URL.RawQuery)
	if err != nil {
		writeProxyError(w, http.StatusBadGateway, err.Error())
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
	if err != nil {
		writeProxyError(w, http.StatusBadGateway, err.Error())
		return
	}
	req.Header = HeadersForUpstream(r.Header, ticket)

	p.resolver.MarkUsed(context.WithoutCancel(r.Context()), provider.ID)

	resp, err := p.client.Do(req)
	if err != nil {
		writeProxyError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()

	copyResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func routeParts(requestPath string) (slug string, rest string, ok bool) {
	const prefix = "/mcp/apps/"
	if !strings.HasPrefix(requestPath, prefix) {
		return "", "", false
	}
	remaining := strings.TrimPrefix(requestPath, prefix)
	if remaining == "" {
		return "", "", false
	}
	parts := strings.SplitN(remaining, "/", 2)
	if parts[0] == "" || strings.Contains(parts[0], "..") {
		return "", "", false
	}
	if len(parts) == 2 {
		rest = "/" + parts[1]
	}
	return parts[0], rest, true
}

func targetURL(appID string, endpoint string, rest string, requestQuery string) (string, error) {
	if appID == "" || strings.ContainsAny(appID, "/:@") {
		return "", fmt.Errorf("invalid app id")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("invalid provider endpoint: %w", err)
	}
	if parsed.IsAbs() || parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/") {
		return "", fmt.Errorf("provider endpoint must be an absolute path")
	}

	basePath := parsed.Path
	if rest != "" {
		basePath = path.Join(basePath, rest)
		if !strings.HasPrefix(basePath, "/") {
			basePath = "/" + basePath
		}
	}
	parsed.Path = basePath
	if requestQuery != "" {
		if parsed.RawQuery != "" {
			parsed.RawQuery += "&" + requestQuery
		} else {
			parsed.RawQuery = requestQuery
		}
	}
	parsed.Scheme = "http"
	parsed.Host = "app." + appID + ".lzcx"
	return parsed.String(), nil
}

func HeadersForUpstream(in http.Header, ticket string) http.Header {
	out := make(http.Header)
	for _, key := range forwardedHeaders() {
		for _, value := range in.Values(key) {
			out.Add(key, value)
		}
	}
	out.Set("X-HC-USER-TICKET", ticket)
	return out
}

func forwardedHeaders() []string {
	return []string{
		"Accept",
		"Accept-Encoding",
		"Accept-Language",
		"Cache-Control",
		"Content-Type",
		"Last-Event-ID",
		"Mcp-Protocol-Version",
		"Mcp-Session-Id",
		"User-Agent",
	}
}

func copyResponseHeaders(dst http.Header, src http.Header) {
	for key, values := range src {
		if isHopByHopHeader(key) {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func isHopByHopHeader(key string) bool {
	switch strings.ToLower(key) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization",
		"te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

func writeProxyError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"error":{"message":%q,"status":%d},"time":%q}`+"\n", message, status, time.Now().Format(time.RFC3339))
}
