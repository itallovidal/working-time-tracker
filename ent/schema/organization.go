package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type Organization struct {
	ent.Schema
}

func (Organization) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.String("name"),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (Organization) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("persons", Person.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("projects", Project.Type),
		edge.To("invites", Invite.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}
