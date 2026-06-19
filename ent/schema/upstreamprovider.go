package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UpstreamProvider stores one user-facing MCP gateway route.
type UpstreamProvider struct {
	ent.Schema
}

// Fields of the UpstreamProvider.
func (UpstreamProvider) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty().MaxLen(120),
		field.String("description").Optional().Nillable().MaxLen(300),
		field.String("slug").NotEmpty().MaxLen(180).Unique(),
		field.Enum("provider_type").StorageKey("type").Values("lazycat", "custom").Default("lazycat"),
		field.String("app_id").Default("").MaxLen(180),
		field.String("deploy_id").Optional().Nillable().MaxLen(180),
		field.String("app_title").Optional().Nillable().MaxLen(180),
		field.String("resource_id").Optional().Nillable().MaxLen(80),
		field.String("base_url").Optional().Nillable().MaxLen(2048),
		field.String("endpoint").Default("/mcp").NotEmpty().MaxLen(240),
		field.Text("headers").Default("[]").Sensitive(),
		field.Enum("transport").Values("streamable_http", "sse").Default("streamable_http"),
		field.Bool("enabled").Default(true),
		field.Time("last_used_at").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Indexes of the UpstreamProvider.
func (UpstreamProvider) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("slug").Unique(),
		index.Fields("provider_type"),
		index.Fields("app_id"),
		index.Fields("enabled"),
	}
}
