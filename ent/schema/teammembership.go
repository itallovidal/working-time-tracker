package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type TeamMembership struct {
	ent.Schema
}

func (TeamMembership) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("person_id", uuid.UUID{}),
		field.UUID("team_id", uuid.UUID{}),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (TeamMembership) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("person", Person.Type).Ref("team_memberships").Field("person_id").Unique().Required(),
		edge.From("team", Team.Type).Ref("memberships").Field("team_id").Unique().Required(),
	}
}

func (TeamMembership) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("person_id", "team_id").Unique(),
	}
}
