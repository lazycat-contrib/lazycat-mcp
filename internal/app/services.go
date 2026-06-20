package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"

	"lazycat-mcp/ent"
	"lazycat-mcp/ent/mcptoken"
	"lazycat-mcp/ent/upstreamprovider"
	"lazycat-mcp/internal/proxy"
)

var (
	errTokenMissing      = errors.New("mcp token is required")
	errTokenInvalid      = errors.New("mcp token is invalid")
	errTokenExpired      = errors.New("mcp token is expired")
	errProviderInvalid   = errors.New("provider is invalid")
	appIDPattern         = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]{0,178}[a-z0-9])?$`)
	providerSlugPattern  = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]{0,178}[a-z0-9])?$`)
	resourceIDPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,79}$`)
	headerNamePattern    = regexp.MustCompile("^[!#$%&'*+\\-.^_`|~0-9A-Za-z]+$")
	tokenNameMaxLen      = 80
	providerNameMaxLen   = 120
	providerDescMaxLen   = 300
	headerValueMaxLen    = 4096
	headerMaxCount       = 20
	defaultTokenByteSize = 32
)

type TokenService struct {
	db *ent.Client
}

type TokenDTO struct {
	ID         int        `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Enabled    bool       `json:"enabled"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	Token      string     `json:"token,omitempty"`
}

func NewTokenService(db *ent.Client) *TokenService {
	return &TokenService{db: db}
}

func (s *TokenService) List(ctx context.Context) ([]TokenDTO, error) {
	rows, err := s.db.MCPToken.Query().Order(mcptoken.ByCreatedAt(sql.OrderDesc())).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]TokenDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, tokenDTO(row, ""))
	}
	return out, nil
}

func (s *TokenService) Create(ctx context.Context, name string, expiresAt *time.Time) (TokenDTO, error) {
	name = normalizeName(name, "MCP Token", tokenNameMaxLen)
	plain, err := newPlainToken()
	if err != nil {
		return TokenDTO{}, err
	}
	hash := tokenHash(plain)
	prefix := tokenPrefix(plain)
	row, err := s.db.MCPToken.Create().
		SetName(name).
		SetTokenHash(hash).
		SetPrefix(prefix).
		SetNillableExpiresAt(expiresAt).
		Save(ctx)
	if err != nil {
		return TokenDTO{}, err
	}
	return tokenDTO(row, plain), nil
}

func (s *TokenService) Update(ctx context.Context, id int, name *string, enabled *bool, expiresAt *time.Time, clearExpires bool) (TokenDTO, error) {
	update := s.db.MCPToken.UpdateOneID(id)
	if name != nil {
		update.SetName(normalizeName(*name, "MCP Token", tokenNameMaxLen))
	}
	if enabled != nil {
		update.SetEnabled(*enabled)
	}
	if clearExpires {
		update.ClearExpiresAt()
	} else if expiresAt != nil {
		update.SetExpiresAt(*expiresAt)
	}
	row, err := update.Save(ctx)
	if err != nil {
		return TokenDTO{}, err
	}
	return tokenDTO(row, ""), nil
}

func (s *TokenService) Delete(ctx context.Context, id int) error {
	return s.db.MCPToken.DeleteOneID(id).Exec(ctx)
}

func (s *TokenService) Validate(ctx context.Context, rawToken string) error {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return errTokenMissing
	}
	row, err := s.db.MCPToken.Query().
		Where(mcptoken.TokenHashEQ(tokenHash(rawToken))).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return errTokenInvalid
		}
		return err
	}
	if !row.Enabled {
		return errTokenInvalid
	}
	if row.ExpiresAt != nil && time.Now().After(*row.ExpiresAt) {
		return errTokenExpired
	}
	now := time.Now()
	_, _ = s.db.MCPToken.UpdateOneID(row.ID).SetLastUsedAt(now).Save(context.WithoutCancel(ctx))
	return nil
}

func tokenDTO(row *ent.MCPToken, plain string) TokenDTO {
	return TokenDTO{
		ID:         row.ID,
		Name:       row.Name,
		Prefix:     row.Prefix,
		Enabled:    row.Enabled,
		ExpiresAt:  row.ExpiresAt,
		LastUsedAt: row.LastUsedAt,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
		Token:      plain,
	}
}

func newPlainToken() (string, error) {
	buf := make([]byte, defaultTokenByteSize)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "lcmcp_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func tokenPrefix(token string) string {
	if len(token) <= 14 {
		return token
	}
	return token[:14]
}

type ProviderService struct {
	db *ent.Client
}

type ProviderInput struct {
	Type        string           `json:"type"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Slug        string           `json:"slug"`
	AppID       string           `json:"app_id"`
	DeployID    string           `json:"deploy_id"`
	AppTitle    string           `json:"app_title"`
	ResourceID  string           `json:"resource_id"`
	BaseURL     string           `json:"base_url"`
	Endpoint    string           `json:"endpoint"`
	Headers     []ProviderHeader `json:"headers"`
	Transport   string           `json:"transport"`
	Enabled     *bool            `json:"enabled"`
	headersJSON string
}

type ProviderHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ProviderDTO struct {
	ID             int        `json:"id"`
	Type           string     `json:"type"`
	Name           string     `json:"name"`
	Description    string     `json:"description,omitempty"`
	Slug           string     `json:"slug"`
	AppID          string     `json:"app_id"`
	DeployID       string     `json:"deploy_id,omitempty"`
	AppTitle       string     `json:"app_title,omitempty"`
	ResourceID     string     `json:"resource_id,omitempty"`
	BaseURL        string     `json:"base_url,omitempty"`
	Endpoint       string     `json:"endpoint"`
	Transport      string     `json:"transport"`
	Enabled        bool       `json:"enabled"`
	PublicEndpoint string     `json:"public_endpoint"`
	HeaderNames    []string   `json:"header_names,omitempty"`
	HeaderCount    int        `json:"header_count"`
	LastUsedAt     *time.Time `json:"last_used_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type PublicProviderDTO struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Endpoint    string `json:"endpoint"`
	Transport   string `json:"transport"`
	ToolPrefix  string `json:"tool_prefix,omitempty"`
}

func NewProviderService(db *ent.Client) *ProviderService {
	return &ProviderService{db: db}
}

func (s *ProviderService) List(ctx context.Context) ([]ProviderDTO, error) {
	rows, err := s.db.UpstreamProvider.Query().Order(upstreamprovider.ByCreatedAt(sql.OrderDesc())).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ProviderDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, providerDTO(row))
	}
	return out, nil
}

func (s *ProviderService) EnabledPublic(ctx context.Context) ([]PublicProviderDTO, error) {
	rows, err := s.db.UpstreamProvider.Query().
		Where(upstreamprovider.EnabledEQ(true)).
		Order(upstreamprovider.BySlug()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PublicProviderDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, publicProviderDTO(row))
	}
	return out, nil
}

func (s *ProviderService) Enabled(ctx context.Context) ([]*ent.UpstreamProvider, error) {
	return s.db.UpstreamProvider.Query().
		Where(upstreamprovider.EnabledEQ(true)).
		Order(upstreamprovider.BySlug()).
		All(ctx)
}

func (s *ProviderService) Create(ctx context.Context, input ProviderInput) (ProviderDTO, error) {
	normalized, err := normalizeProviderInput(input, true)
	if err != nil {
		return ProviderDTO{}, err
	}
	create := s.db.UpstreamProvider.Create().
		SetProviderType(upstreamprovider.ProviderType(normalized.Type)).
		SetName(normalized.Name).
		SetSlug(normalized.Slug).
		SetEndpoint(normalized.Endpoint).
		SetHeaders(normalized.headersJSON).
		SetTransport(upstreamprovider.Transport(normalized.Transport)).
		SetEnabled(normalized.enabledValue(true))
	if normalized.Description != "" {
		create.SetDescription(normalized.Description)
	}
	if normalized.Type == upstreamprovider.ProviderTypeLazycat.String() {
		create.SetAppID(normalized.AppID)
		setLazyCatProviderFields(create, normalized)
	}
	if normalized.Type == upstreamprovider.ProviderTypeCustom.String() {
		create.SetBaseURL(normalized.BaseURL)
	}
	row, err := create.Save(ctx)
	if err != nil {
		return ProviderDTO{}, err
	}
	return providerDTO(row), nil
}

func (s *ProviderService) Update(ctx context.Context, id int, input ProviderInput) (ProviderDTO, error) {
	normalized, err := normalizeProviderInput(input, false)
	if err != nil {
		return ProviderDTO{}, err
	}
	update := s.db.UpstreamProvider.UpdateOneID(id)
	if normalized.Name != "" {
		update.SetName(normalized.Name)
	}
	if normalized.Description != "" {
		update.SetDescription(normalized.Description)
	}
	if normalized.Slug != "" {
		update.SetSlug(normalized.Slug)
	}
	if normalized.Type != "" {
		update.SetProviderType(upstreamprovider.ProviderType(normalized.Type))
	}
	if normalized.AppID != "" {
		update.SetAppID(normalized.AppID)
	}
	if normalized.BaseURL != "" {
		update.SetBaseURL(normalized.BaseURL)
	}
	if normalized.Endpoint != "" {
		update.SetEndpoint(normalized.Endpoint)
	}
	if input.Headers != nil {
		update.SetHeaders(normalized.headersJSON)
	}
	if normalized.Transport != "" {
		update.SetTransport(upstreamprovider.Transport(normalized.Transport))
	}
	if normalized.Enabled != nil {
		update.SetEnabled(*normalized.Enabled)
	}
	if normalized.DeployID != "" {
		update.SetDeployID(normalized.DeployID)
	}
	if normalized.AppTitle != "" {
		update.SetAppTitle(normalized.AppTitle)
	}
	if normalized.ResourceID != "" {
		update.SetResourceID(normalized.ResourceID)
	}
	row, err := update.Save(ctx)
	if err != nil {
		return ProviderDTO{}, err
	}
	return providerDTO(row), nil
}

func (s *ProviderService) Delete(ctx context.Context, id int) error {
	return s.db.UpstreamProvider.DeleteOneID(id).Exec(ctx)
}

func (s *ProviderService) GetEnabledBySlug(ctx context.Context, slug string) (*ent.UpstreamProvider, error) {
	row, err := s.db.UpstreamProvider.Query().
		Where(upstreamprovider.SlugEQ(slug), upstreamprovider.EnabledEQ(true)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, proxy.ErrProviderNotFound
		}
		return nil, err
	}
	return row, nil
}

func (s *ProviderService) MarkUsed(ctx context.Context, id int) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	now := time.Now()
	_, _ = s.db.UpstreamProvider.UpdateOneID(id).SetLastUsedAt(now).Save(ctx)
}

func providerDTO(row *ent.UpstreamProvider) ProviderDTO {
	dto := ProviderDTO{
		ID:             row.ID,
		Type:           row.ProviderType.String(),
		Name:           row.Name,
		Slug:           row.Slug,
		AppID:          row.AppID,
		Endpoint:       row.Endpoint,
		Transport:      row.Transport.String(),
		Enabled:        row.Enabled,
		PublicEndpoint: "/mcp/apps/" + row.Slug,
		LastUsedAt:     row.LastUsedAt,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
	if row.Description != nil {
		dto.Description = *row.Description
	}
	if row.DeployID != nil {
		dto.DeployID = *row.DeployID
	}
	if row.AppTitle != nil {
		dto.AppTitle = *row.AppTitle
	}
	if row.ResourceID != nil {
		dto.ResourceID = *row.ResourceID
	}
	if row.BaseURL != nil {
		dto.BaseURL = *row.BaseURL
	}
	headers := decodeProviderHeaders(row.Headers)
	dto.HeaderCount = len(headers)
	for _, header := range headers {
		dto.HeaderNames = append(dto.HeaderNames, header.Name)
	}
	return dto
}

func publicProviderDTO(row *ent.UpstreamProvider) PublicProviderDTO {
	dto := PublicProviderDTO{
		Name:       row.Name,
		Endpoint:   "/mcp/apps/" + row.Slug,
		Transport:  row.Transport.String(),
		ToolPrefix: sanitizeToolNamePart(row.Slug) + "__",
	}
	if row.Description != nil {
		dto.Description = *row.Description
	}
	return dto
}

func setLazyCatProviderFields(create *ent.UpstreamProviderCreate, input ProviderInput) {
	if input.DeployID != "" {
		create.SetDeployID(input.DeployID)
	}
	if input.AppTitle != "" {
		create.SetAppTitle(input.AppTitle)
	}
	if input.ResourceID != "" {
		create.SetResourceID(input.ResourceID)
	}
}

func normalizeProviderInput(input ProviderInput, requireAll bool) (ProviderInput, error) {
	out := ProviderInput{
		Type:        strings.TrimSpace(input.Type),
		Name:        strings.TrimSpace(input.Name),
		Description: strings.TrimSpace(input.Description),
		Slug:        strings.TrimSpace(input.Slug),
		AppID:       strings.TrimSpace(input.AppID),
		DeployID:    strings.TrimSpace(input.DeployID),
		AppTitle:    strings.TrimSpace(input.AppTitle),
		ResourceID:  strings.TrimSpace(input.ResourceID),
		BaseURL:     strings.TrimSpace(input.BaseURL),
		Endpoint:    strings.TrimSpace(input.Endpoint),
		Headers:     input.Headers,
		Transport:   strings.TrimSpace(input.Transport),
		Enabled:     input.Enabled,
	}
	if out.Type == "" && requireAll {
		out.Type = upstreamprovider.ProviderTypeLazycat.String()
	}
	if out.Type != "" {
		if err := upstreamprovider.ProviderTypeValidator(upstreamprovider.ProviderType(out.Type)); err != nil {
			return out, err
		}
	}
	if out.Transport == "" && requireAll {
		out.Transport = upstreamprovider.TransportStreamableHTTP.String()
	}
	if out.Description != "" {
		out.Description = normalizeName(out.Description, "", providerDescMaxLen)
	}
	if out.Name == "" && requireAll {
		switch out.Type {
		case upstreamprovider.ProviderTypeLazycat.String():
			out.Name = out.AppTitle
			if out.Name == "" {
				out.Name = out.AppID
			}
		case upstreamprovider.ProviderTypeCustom.String():
			out.Name = out.BaseURL
		}
	}
	if out.Name != "" {
		out.Name = normalizeName(out.Name, "MCP Provider", providerNameMaxLen)
	}
	if out.Slug != "" && !providerSlugPattern.MatchString(out.Slug) {
		return out, fmt.Errorf("%w: invalid subpath", errProviderInvalid)
	}
	if requireAll && out.Slug == "" {
		return out, fmt.Errorf("%w: subpath is required", errProviderInvalid)
	}
	if out.AppID != "" && !appIDPattern.MatchString(out.AppID) {
		return out, fmt.Errorf("%w: invalid app id", errProviderInvalid)
	}
	if out.ResourceID != "" && !resourceIDPattern.MatchString(out.ResourceID) {
		return out, fmt.Errorf("%w: invalid resource id", errProviderInvalid)
	}
	if out.BaseURL != "" {
		baseURL, err := normalizeBaseURL(out.BaseURL)
		if err != nil {
			return out, err
		}
		out.BaseURL = baseURL
	}
	if out.Endpoint != "" {
		endpoint, err := normalizeEndpoint(out.Endpoint)
		if err != nil {
			return out, err
		}
		out.Endpoint = endpoint
	}
	if requireAll && out.Endpoint == "" {
		return out, fmt.Errorf("%w: endpoint is required", errProviderInvalid)
	}
	if out.Transport != "" {
		if err := upstreamprovider.TransportValidator(upstreamprovider.Transport(out.Transport)); err != nil {
			return out, err
		}
	}
	headers, err := normalizeProviderHeaders(input.Headers)
	if err != nil {
		return out, err
	}
	out.Headers = headers
	headersJSON, err := encodeProviderHeaders(headers)
	if err != nil {
		return out, err
	}
	out.headersJSON = headersJSON
	if requireAll {
		switch out.Type {
		case upstreamprovider.ProviderTypeLazycat.String():
			if out.AppID == "" {
				return out, fmt.Errorf("%w: app id is required", errProviderInvalid)
			}
			if out.BaseURL != "" {
				return out, fmt.Errorf("%w: custom service url is not valid for lazycat providers", errProviderInvalid)
			}
			if len(out.Headers) > 0 {
				return out, fmt.Errorf("%w: custom headers are not valid for lazycat providers", errProviderInvalid)
			}
		case upstreamprovider.ProviderTypeCustom.String():
			if out.BaseURL == "" {
				return out, fmt.Errorf("%w: service url is required", errProviderInvalid)
			}
			if out.AppID != "" {
				return out, fmt.Errorf("%w: app id is not valid for custom providers", errProviderInvalid)
			}
		}
	}
	return out, nil
}

func normalizeBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("%w: invalid service url", errProviderInvalid)
	}
	if !parsed.IsAbs() || parsed.Host == "" {
		return "", fmt.Errorf("%w: service url must include scheme and host", errProviderInvalid)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("%w: service url must use http or https", errProviderInvalid)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("%w: service url must not include credentials, query, or fragment", errProviderInvalid)
	}
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	parsed.Path = path.Clean(parsed.Path)
	if parsed.Path == "." {
		parsed.Path = "/"
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func normalizeEndpoint(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("%w: invalid endpoint", errProviderInvalid)
	}
	if parsed.IsAbs() || parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/") {
		return "", fmt.Errorf("%w: endpoint must be an absolute path", errProviderInvalid)
	}
	if parsed.Fragment != "" {
		return "", fmt.Errorf("%w: endpoint must not include fragment", errProviderInvalid)
	}
	parsed.Path = path.Clean(parsed.Path)
	if parsed.Path == "." {
		parsed.Path = "/"
	}
	return parsed.String(), nil
}

func normalizeProviderHeaders(headers []ProviderHeader) ([]ProviderHeader, error) {
	if len(headers) > headerMaxCount {
		return nil, fmt.Errorf("%w: too many headers", errProviderInvalid)
	}
	out := make([]ProviderHeader, 0, len(headers))
	for _, header := range headers {
		name := strings.TrimSpace(header.Name)
		value := strings.TrimSpace(header.Value)
		if name == "" && value == "" {
			continue
		}
		if name == "" || !headerNamePattern.MatchString(name) {
			return nil, fmt.Errorf("%w: invalid header name", errProviderInvalid)
		}
		if isReservedProviderHeader(name) {
			return nil, fmt.Errorf("%w: reserved header cannot be configured", errProviderInvalid)
		}
		if len(value) > headerValueMaxLen {
			return nil, fmt.Errorf("%w: header value is too long", errProviderInvalid)
		}
		out = append(out, ProviderHeader{Name: name, Value: value})
	}
	return out, nil
}

func encodeProviderHeaders(headers []ProviderHeader) (string, error) {
	if len(headers) == 0 {
		return "[]", nil
	}
	data, err := json.Marshal(headers)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeProviderHeaders(raw string) []ProviderHeader {
	var headers []ProviderHeader
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &headers); err != nil {
		return nil
	}
	normalized, err := normalizeProviderHeaders(headers)
	if err != nil {
		return nil
	}
	return normalized
}

func isReservedProviderHeader(name string) bool {
	switch strings.ToLower(name) {
	case "host", "content-length", "connection", "keep-alive", "proxy-authenticate",
		"proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade",
		"x-mcp-token":
		return true
	default:
		return false
	}
}

func normalizeName(value string, fallback string, maxLen int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	if len(value) > maxLen {
		value = value[:maxLen]
	}
	return value
}

func (input ProviderInput) enabledValue(fallback bool) bool {
	if input.Enabled == nil {
		return fallback
	}
	return *input.Enabled
}
