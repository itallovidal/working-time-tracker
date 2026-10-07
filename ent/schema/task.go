package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type Task struct {
	ent.Schema
}

func (Task) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("project_id", uuid.UUID{}),
		field.String("name"),
		field.String("description").Optional(),
		field.Enum("priority").Values("urgent", "high", "medium", "low", "none").Default("none"),
		field.Enum("status").Values("backlog", "in_progress", "awaiting_closure", "closed").Default("backlog"),
		field.UUID("assignee_id", uuid.UUID{}).Optional().Nillable(),
		field.Time("deadline").Optional(),
		field.UUID("external_integration_id", uuid.UUID{}).Optional().Nillable(),
		field.String("external_item_id").Optional().Nillable(),
		field.String("external_item_url").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (Task) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("project", Project.Type).Ref("tasks").Field("project_id").Unique().Required(),
		edge.From("assignee", Person.Type).Ref("tasks").Field("assignee_id").Unique(),
		edge.From("external_integration", Integration.Type).Ref("tasks").Field("external_integration_id").Unique(),
		edge.To("work_sessions", WorkSession.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("labels", Label.Type),
	}
}
