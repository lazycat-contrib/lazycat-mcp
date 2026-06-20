package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// MCPCallLog stores metadata for one MCP call event.
type MCPCallLog struct {
	ent.Schema
}

// Fields of the MCPCallLog.
func (MCPCallLog) Fields() []ent.Field {
	return []ent.Field{
		field.Enum("source").Values("local", "upstream").Default("local"),
		field.Enum("transport").Values("streamable_http", "sse", "http").Default("streamable_http"),
		field.String("method").NotEmpty().MaxLen(180),
		field.String("target").NotEmpty().MaxLen(240),
		field.String("provider_slug").Optional().Nillable().MaxLen(180),
		field.String("token_prefix").Optional().Nillable().MaxLen(32),
		field.String("session_id").Optional().Nillable().MaxLen(180),
		field.String("request_id").Optional().Nillable().MaxLen(120),
		field.Enum("status").Values("success", "error").Default("success"),
		field.Int("status_code").Optional().Nillable().NonNegative(),
		field.Int64("duration_ms").Default(0).NonNegative(),
		field.String("error").Optional().Nillable().MaxLen(500),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

// Indexes of the MCPCallLog.
func (MCPCallLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("created_at"),
		index.Fields("source"),
		index.Fields("status"),
		index.Fields("provider_slug"),
	}
}
