package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// Customer é o cliente que contrata a organização. Um cliente pode ter vários
// projetos. O nome não é Client porque o Ent reserva esse identificador.
type Customer struct {
	ent.Schema
}

func (Customer) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("organization_id", uuid.UUID{}),
		field.String("name"),
		// O país do cliente (código ISO 3166-1: BR, US, DE...), que começa no da organização. Decide a regra do documento: o
		// CNPJ no Brasil e o EIN nos EUA são conferidos; nos outros países o documento é texto livre. Quem decide é
		// internal/country.
		field.String("country").Default("BR"),
		// O documento fiscal do cliente, sem máscara quando o país tem regra (CNPJ, EIN).
		field.String("document").Optional(),
		field.String("contact_name").Optional(),
		field.String("contact_email").Optional(),
		field.String("contact_phone").Optional(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (Customer) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("organization", Organization.Type).Ref("customers").Field("organization_id").Unique().Required(),
		edge.To("projects", Project.Type),
	}
}
