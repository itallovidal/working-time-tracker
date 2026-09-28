package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// Invite é um link de convite para entrar numa organização. É de uso único e
// expira; o banco só guarda o sha256 do token.
type Invite struct {
	ent.Schema
}

func (Invite) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("organization_id", uuid.UUID{}),
		field.String("token_hash").Unique().Sensitive(),
		// Quando preenchido, só esse email pode aceitar o convite.
		field.String("email").Optional().Nillable(),
		field.Enum("role").Values("admin", "member").Default("member"),
		field.UUID("created_by_id", uuid.UUID{}).Optional().Nillable(),
		field.Time("expires_at"),
		field.Time("accepted_at").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (Invite) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("organization", Organization.Type).Ref("invites").Field("organization_id").Unique().Required(),
		edge.From("created_by", Person.Type).Ref("created_invites").Field("created_by_id").Unique(),
	}
}
