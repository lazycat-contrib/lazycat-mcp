package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"lazycat-mcp/ent"
	"lazycat-mcp/ent/upstreamprovider"
)

var (
	ErrProviderNotFound = errors.New("provider not found")
	ErrTicketMissing    = errors.New("lazycat user ticket is missing")
	headerNamePattern   = regexp.MustCompile("^[!#$%&'*+\\-.^_`|~0-9A-Za-z]+$")
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

	target, headers, err := p.prepareUpstreamRequest(provider, r, rest)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, ErrTicketMissing) {
			status = http.StatusPreconditionRequired
		}
		writeProxyError(w, status, err.Error())
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
	if err != nil {
		writeProxyError(w, http.StatusBadGateway, err.Error())
		return
	}
	req.Header = headers

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

func (p *Proxy) prepareUpstreamRequest(provider *ent.UpstreamProvider, r *http.Request, rest string) (string, http.Header, error) {
	switch provider.ProviderType {
	case upstreamprovider.ProviderTypeLazycat:
		ticket, ok := p.tickets.Get()
		if !ok {
			return "", nil, ErrTicketMissing
		}
		target, err := lazyCatTargetURL(provider.AppID, provider.Endpoint, rest, r.URL.RawQuery)
		if err != nil {
			return "", nil, err
		}
		return target, HeadersForLazyCatUpstream(r.Header, ticket), nil
	case upstreamprovider.ProviderTypeCustom:
		target, err := customTargetURL(provider.BaseURL, provider.Endpoint, rest, r.URL.RawQuery)
		if err != nil {
			return "", nil, err
		}
		headers, err := HeadersForCustomUpstream(r.Header, provider.Headers)
		if err != nil {
			return "", nil, err
		}
		return target, headers, nil
	default:
		return "", nil, fmt.Errorf("unsupported provider type: %s", provider.ProviderType)
	}
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

func lazyCatTargetURL(appID string, endpoint string, rest string, requestQuery string) (string, error) {
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

func LazyCatTargetURL(appID string, endpoint string, rest string, requestQuery string) (string, error) {
	return lazyCatTargetURL(appID, endpoint, rest, requestQuery)
}

func targetURL(appID string, endpoint string, rest string, requestQuery string) (string, error) {
	return lazyCatTargetURL(appID, endpoint, rest, requestQuery)
}

func customTargetURL(baseURL *string, endpoint string, rest string, requestQuery string) (string, error) {
	if baseURL == nil || strings.TrimSpace(*baseURL) == "" {
		return "", fmt.Errorf("custom provider service url is missing")
	}
	base, err := url.Parse(strings.TrimRight(strings.TrimSpace(*baseURL), "/"))
	if err != nil {
		return "", fmt.Errorf("invalid custom provider service url: %w", err)
	}
	if !base.IsAbs() || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") {
		return "", fmt.Errorf("custom provider service url must use http or https")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("invalid provider endpoint: %w", err)
	}
	if parsed.IsAbs() || parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/") {
		return "", fmt.Errorf("provider endpoint must be an absolute path")
	}

	target := *base
	target.Path = joinURLPath(base.Path, parsed.Path, rest)
	target.RawQuery = parsed.RawQuery
	if requestQuery != "" {
		if target.RawQuery != "" {
			target.RawQuery += "&" + requestQuery
		} else {
			target.RawQuery = requestQuery
		}
	}
	return target.String(), nil
}

func CustomTargetURL(baseURL *string, endpoint string, rest string, requestQuery string) (string, error) {
	return customTargetURL(baseURL, endpoint, rest, requestQuery)
}

func joinURLPath(parts ...string) string {
	cleanParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || part == "/" {
			continue
		}
		cleanParts = append(cleanParts, part)
	}
	if len(cleanParts) == 0 {
		return "/"
	}
	joined := path.Join(cleanParts...)
	if !strings.HasPrefix(joined, "/") {
		joined = "/" + joined
	}
	return joined
}

func HeadersForLazyCatUpstream(in http.Header, ticket string) http.Header {
	out := make(http.Header)
	for _, key := range forwardedHeaders() {
		for _, value := range in.Values(key) {
			out.Add(key, value)
		}
	}
	out.Set("X-HC-USER-TICKET", ticket)
	return out
}

func HeadersForUpstream(in http.Header, ticket string) http.Header {
	return HeadersForLazyCatUpstream(in, ticket)
}

func HeadersForCustomUpstream(in http.Header, rawHeaders string) (http.Header, error) {
	out := make(http.Header)
	for _, key := range forwardedHeaders() {
		for _, value := range in.Values(key) {
			out.Add(key, value)
		}
	}
	headers, err := configuredHeaders(rawHeaders)
	if err != nil {
		return nil, err
	}
	for _, header := range headers {
		out.Set(header.Name, header.Value)
	}
	return out, nil
}

type configuredHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func configuredHeaders(raw string) ([]configuredHeader, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var headers []configuredHeader
	if err := json.Unmarshal([]byte(raw), &headers); err != nil {
		return nil, fmt.Errorf("invalid custom provider headers: %w", err)
	}
	for _, header := range headers {
		if strings.TrimSpace(header.Name) == "" || !headerNamePattern.MatchString(header.Name) {
			return nil, fmt.Errorf("invalid custom provider header name")
		}
		if isReservedConfiguredHeader(header.Name) {
			return nil, fmt.Errorf("reserved custom provider header cannot be configured")
		}
	}
	return headers, nil
}

func isReservedConfiguredHeader(name string) bool {
	switch strings.ToLower(name) {
	case "host", "content-length", "connection", "keep-alive", "proxy-authenticate",
		"proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade",
		"x-mcp-token":
		return true
	default:
		return false
	}
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
