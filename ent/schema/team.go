package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type Team struct {
	ent.Schema
}

func (Team) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.String("name"),
		field.UUID("project_id", uuid.UUID{}),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (Team) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("project", Project.Type).Ref("teams").Field("project_id").Unique().Required(),
		edge.To("memberships", TeamMembership.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
		// Um convite que levava a pessoa a este time continua valendo, só que sem o time.
		edge.To("invites", Invite.Type).Annotations(entsql.OnDelete(entsql.SetNull)),
	}
}
