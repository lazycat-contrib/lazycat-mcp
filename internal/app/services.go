package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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
	tokenNameMaxLen      = 80
	providerNameMaxLen   = 120
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
	Name       string `json:"name"`
	Slug       string `json:"slug"`
	AppID      string `json:"app_id"`
	DeployID   string `json:"deploy_id"`
	AppTitle   string `json:"app_title"`
	ResourceID string `json:"resource_id"`
	Endpoint   string `json:"endpoint"`
	Transport  string `json:"transport"`
	Enabled    *bool  `json:"enabled"`
}

type ProviderDTO struct {
	ID             int        `json:"id"`
	Name           string     `json:"name"`
	Slug           string     `json:"slug"`
	AppID          string     `json:"app_id"`
	DeployID       string     `json:"deploy_id,omitempty"`
	AppTitle       string     `json:"app_title,omitempty"`
	ResourceID     string     `json:"resource_id,omitempty"`
	Endpoint       string     `json:"endpoint"`
	Transport      string     `json:"transport"`
	Enabled        bool       `json:"enabled"`
	PublicEndpoint string     `json:"public_endpoint"`
	LastUsedAt     *time.Time `json:"last_used_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
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

func (s *ProviderService) Enabled(ctx context.Context) ([]ProviderDTO, error) {
	rows, err := s.db.UpstreamProvider.Query().
		Where(upstreamprovider.EnabledEQ(true)).
		Order(upstreamprovider.BySlug()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ProviderDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, providerDTO(row))
	}
	return out, nil
}

func (s *ProviderService) Create(ctx context.Context, input ProviderInput) (ProviderDTO, error) {
	normalized, err := normalizeProviderInput(input, true)
	if err != nil {
		return ProviderDTO{}, err
	}
	create := s.db.UpstreamProvider.Create().
		SetName(normalized.Name).
		SetSlug(normalized.Slug).
		SetAppID(normalized.AppID).
		SetEndpoint(normalized.Endpoint).
		SetTransport(upstreamprovider.Transport(normalized.Transport)).
		SetEnabled(normalized.enabledValue(true))
	setOptionalProviderFields(create, normalized)
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
	if normalized.Slug != "" {
		update.SetSlug(normalized.Slug)
	}
	if normalized.AppID != "" {
		update.SetAppID(normalized.AppID)
	}
	if normalized.Endpoint != "" {
		update.SetEndpoint(normalized.Endpoint)
	}
	if normalized.Transport != "" {
		update.SetTransport(upstreamprovider.Transport(normalized.Transport))
	}
	if normalized.Enabled != nil {
		update.SetEnabled(*normalized.Enabled)
	}
	if normalized.DeployID != "" {
		update.SetDeployID(normalized.DeployID)
	} else if input.DeployID == "" {
		update.ClearDeployID()
	}
	if normalized.AppTitle != "" {
		update.SetAppTitle(normalized.AppTitle)
	} else if input.AppTitle == "" {
		update.ClearAppTitle()
	}
	if normalized.ResourceID != "" {
		update.SetResourceID(normalized.ResourceID)
	} else if input.ResourceID == "" {
		update.ClearResourceID()
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
	if row.DeployID != nil {
		dto.DeployID = *row.DeployID
	}
	if row.AppTitle != nil {
		dto.AppTitle = *row.AppTitle
	}
	if row.ResourceID != nil {
		dto.ResourceID = *row.ResourceID
	}
	return dto
}

func setOptionalProviderFields(create *ent.UpstreamProviderCreate, input ProviderInput) {
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
		Name:       strings.TrimSpace(input.Name),
		Slug:       strings.TrimSpace(input.Slug),
		AppID:      strings.TrimSpace(input.AppID),
		DeployID:   strings.TrimSpace(input.DeployID),
		AppTitle:   strings.TrimSpace(input.AppTitle),
		ResourceID: strings.TrimSpace(input.ResourceID),
		Endpoint:   strings.TrimSpace(input.Endpoint),
		Transport:  strings.TrimSpace(input.Transport),
		Enabled:    input.Enabled,
	}
	if out.Transport == "" && requireAll {
		out.Transport = upstreamprovider.TransportStreamableHTTP.String()
	}
	if out.Name == "" && requireAll {
		out.Name = out.AppTitle
		if out.Name == "" {
			out.Name = out.AppID
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
	if requireAll && out.AppID == "" {
		return out, fmt.Errorf("%w: app id is required", errProviderInvalid)
	}
	if out.ResourceID != "" && !resourceIDPattern.MatchString(out.ResourceID) {
		return out, fmt.Errorf("%w: invalid resource id", errProviderInvalid)
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
	return out, nil
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
