package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// Session é uma sessão de login. O cookie guarda o token em claro; o banco só
// guarda o sha256 dele.
type Session struct {
	ent.Schema
}

func (Session) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.String("token_hash").Unique().Sensitive(),
		field.UUID("person_id", uuid.UUID{}),
		field.Time("expires_at"),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (Session) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("person", Person.Type).Ref("sessions").Field("person_id").Unique().Required(),
	}
}
