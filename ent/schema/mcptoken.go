package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// MCPToken stores a hashed access token for public MCP endpoints.
type MCPToken struct {
	ent.Schema
}

// Fields of the MCPToken.
func (MCPToken) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty().MaxLen(80),
		field.String("token_hash").Sensitive().NotEmpty().Unique(),
		field.String("prefix").NotEmpty().MaxLen(16),
		field.Bool("enabled").Default(true),
		field.Time("expires_at").Optional().Nillable(),
		field.Time("last_used_at").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Indexes of the MCPToken.
func (MCPToken) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("token_hash").Unique(),
		index.Fields("enabled"),
	}
}
