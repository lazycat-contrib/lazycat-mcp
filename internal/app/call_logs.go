package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"

	"lazycat-mcp/ent"
	"lazycat-mcp/ent/mcpcalllog"
)

const (
	defaultCallLogLimit = 100
	maxCallLogLimit     = 500
	callLogErrorMaxLen  = 500
)

type MCPCallLogService struct {
	db            *ent.Client
	retentionDays int
}

type MCPCallLogInput struct {
	Source       string
	Transport    string
	Method       string
	Target       string
	ProviderSlug string
	TokenPrefix  string
	SessionID    string
	RequestID    string
	Status       string
	StatusCode   *int
	Duration     time.Duration
	Error        string
	CreatedAt    *time.Time
}

type MCPCallLogFilter struct {
	Limit        int
	Source       string
	Status       string
	ProviderSlug string
}

type MCPCallLogDTO struct {
	ID           int       `json:"id"`
	Source       string    `json:"source"`
	Transport    string    `json:"transport"`
	Method       string    `json:"method"`
	Target       string    `json:"target"`
	ProviderSlug string    `json:"provider_slug,omitempty"`
	TokenPrefix  string    `json:"token_prefix,omitempty"`
	SessionID    string    `json:"session_id,omitempty"`
	RequestID    string    `json:"request_id,omitempty"`
	Status       string    `json:"status"`
	StatusCode   *int      `json:"status_code,omitempty"`
	DurationMs   int64     `json:"duration_ms"`
	Error        string    `json:"error,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

func NewMCPCallLogService(db *ent.Client, retentionDays int) *MCPCallLogService {
	if retentionDays < 0 {
		retentionDays = 0
	}
	return &MCPCallLogService{db: db, retentionDays: retentionDays}
}

func (s *MCPCallLogService) Record(ctx context.Context, input MCPCallLogInput) (MCPCallLogDTO, error) {
	normalized := normalizeCallLogInput(input)
	create := s.db.MCPCallLog.Create().
		SetSource(mcpcalllog.Source(normalized.Source)).
		SetTransport(mcpcalllog.Transport(normalized.Transport)).
		SetMethod(normalized.Method).
		SetTarget(normalized.Target).
		SetStatus(mcpcalllog.Status(normalized.Status)).
		SetDurationMs(durationMillis(normalized.Duration)).
		SetNillableStatusCode(normalized.StatusCode).
		SetNillableProviderSlug(optionalString(normalized.ProviderSlug)).
		SetNillableTokenPrefix(optionalString(normalized.TokenPrefix)).
		SetNillableSessionID(optionalString(normalized.SessionID)).
		SetNillableRequestID(optionalString(normalized.RequestID)).
		SetNillableError(optionalString(normalized.Error)).
		SetNillableCreatedAt(normalized.CreatedAt)
	row, err := create.Save(ctx)
	if err != nil {
		return MCPCallLogDTO{}, err
	}
	return callLogDTO(row), nil
}

func (s *MCPCallLogService) List(ctx context.Context, filter MCPCallLogFilter) ([]MCPCallLogDTO, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = defaultCallLogLimit
	}
	if limit > maxCallLogLimit {
		limit = maxCallLogLimit
	}
	query := s.db.MCPCallLog.Query().
		Order(mcpcalllog.ByCreatedAt(sql.OrderDesc()), mcpcalllog.ByID(sql.OrderDesc())).
		Limit(limit)
	if filter.Source != "" {
		source := mcpcalllog.Source(strings.TrimSpace(filter.Source))
		if err := mcpcalllog.SourceValidator(source); err != nil {
			return nil, err
		}
		query.Where(mcpcalllog.SourceEQ(source))
	}
	if filter.Status != "" {
		status := mcpcalllog.Status(strings.TrimSpace(filter.Status))
		if err := mcpcalllog.StatusValidator(status); err != nil {
			return nil, err
		}
		query.Where(mcpcalllog.StatusEQ(status))
	}
	if filter.ProviderSlug != "" {
		query.Where(mcpcalllog.ProviderSlugEQ(strings.TrimSpace(filter.ProviderSlug)))
	}
	rows, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]MCPCallLogDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, callLogDTO(row))
	}
	return out, nil
}

func (s *MCPCallLogService) Clear(ctx context.Context) (int, error) {
	return s.db.MCPCallLog.Delete().Exec(ctx)
}

func (s *MCPCallLogService) Cleanup(ctx context.Context, now time.Time) (int, error) {
	if s.retentionDays <= 0 {
		return 0, nil
	}
	cutoff := now.AddDate(0, 0, -s.retentionDays)
	return s.db.MCPCallLog.Delete().
		Where(mcpcalllog.CreatedAtLT(cutoff)).
		Exec(ctx)
}

func normalizeCallLogInput(input MCPCallLogInput) MCPCallLogInput {
	input.Source = strings.TrimSpace(input.Source)
	if input.Source == "" {
		input.Source = mcpcalllog.SourceLocal.String()
	}
	if err := mcpcalllog.SourceValidator(mcpcalllog.Source(input.Source)); err != nil {
		input.Source = mcpcalllog.SourceLocal.String()
	}
	input.Transport = strings.TrimSpace(input.Transport)
	if input.Transport == "" {
		input.Transport = mcpcalllog.TransportStreamableHTTP.String()
	}
	if err := mcpcalllog.TransportValidator(mcpcalllog.Transport(input.Transport)); err != nil {
		input.Transport = mcpcalllog.TransportStreamableHTTP.String()
	}
	input.Method = truncateLogString(strings.TrimSpace(input.Method), 180)
	if input.Method == "" {
		input.Method = "unknown"
	}
	input.Target = truncateLogString(strings.TrimSpace(input.Target), 240)
	if input.Target == "" {
		input.Target = input.Method
	}
	input.ProviderSlug = truncateLogString(strings.TrimSpace(input.ProviderSlug), 180)
	input.TokenPrefix = truncateLogString(strings.TrimSpace(input.TokenPrefix), 32)
	input.SessionID = truncateLogString(strings.TrimSpace(input.SessionID), 180)
	input.RequestID = truncateLogString(strings.TrimSpace(input.RequestID), 120)
	input.Status = strings.TrimSpace(input.Status)
	if input.Status == "" {
		input.Status = mcpcalllog.StatusSuccess.String()
	}
	if err := mcpcalllog.StatusValidator(mcpcalllog.Status(input.Status)); err != nil {
		input.Status = mcpcalllog.StatusError.String()
	}
	input.Error = truncateLogString(strings.TrimSpace(input.Error), callLogErrorMaxLen)
	if input.Status == mcpcalllog.StatusError.String() && input.Error == "" && input.StatusCode != nil {
		input.Error = fmt.Sprintf("status %d", *input.StatusCode)
	}
	return input
}

func callLogDTO(row *ent.MCPCallLog) MCPCallLogDTO {
	dto := MCPCallLogDTO{
		ID:         row.ID,
		Source:     row.Source.String(),
		Transport:  row.Transport.String(),
		Method:     row.Method,
		Target:     row.Target,
		Status:     row.Status.String(),
		StatusCode: row.StatusCode,
		DurationMs: row.DurationMs,
		CreatedAt:  row.CreatedAt,
	}
	if row.ProviderSlug != nil {
		dto.ProviderSlug = *row.ProviderSlug
	}
	if row.TokenPrefix != nil {
		dto.TokenPrefix = *row.TokenPrefix
	}
	if row.SessionID != nil {
		dto.SessionID = *row.SessionID
	}
	if row.RequestID != nil {
		dto.RequestID = *row.RequestID
	}
	if row.Error != nil {
		dto.Error = *row.Error
	}
	return dto
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func durationMillis(duration time.Duration) int64 {
	if duration < 0 {
		return 0
	}
	return duration.Milliseconds()
}

func truncateLogString(value string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxLen {
		return value
	}
	return string(runes[:maxLen])
}
