package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type Project struct {
	ent.Schema
}

func (Project) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("organization_id", uuid.UUID{}),
		field.String("name"),
		field.String("description").Optional(),
		field.String("github_repo_url").Optional().Nillable(),
		field.String("gitlab_repo_url").Optional().Nillable(),
		field.Int("sprint_duration_days").Default(14),
		field.String("daily_time").Optional().Nillable(),
		field.String("weekly_sync_day").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (Project) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("organization", Organization.Type).Ref("projects").Field("organization_id").Unique().Required(),
		edge.To("teams", Team.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("tasks", Task.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("integrations", Integration.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}
