package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type Person struct {
	ent.Schema
}

func (Person) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.String("name"),
		field.String("email"),
		field.UUID("organization_id", uuid.UUID{}),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (Person) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("organization", Organization.Type).Ref("persons").Field("organization_id").Unique().Required(),
		edge.To("tasks", Task.Type),
		edge.To("team_memberships", TeamMembership.Type),
		edge.To("work_sessions", WorkSession.Type),
	}
}

func (Person) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("organization_id", "email").Unique(),
	}
}
