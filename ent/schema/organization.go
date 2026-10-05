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

		// Identidade
		field.String("summary").Optional(),
		field.Text("description").Optional(),
		field.String("industry").Optional(),
		field.Int("founded_year").Optional().Nillable(),
		field.String("size").Optional(),

		// Contato
		field.String("website").Optional(),
		field.String("contact_email").Optional(),
		field.String("phone").Optional(),
		field.String("linkedin_url").Optional(),
		field.String("instagram_url").Optional(),

		// Dados jurídicos. O CNPJ fica sem máscara.
		field.String("legal_name").Optional(),
		field.String("cnpj").Optional(),
		field.String("address_line1").Optional(),
		field.String("address_line2").Optional(),
		field.String("city").Optional(),
		field.String("state").Optional(),
		field.String("postal_code").Optional(),
		field.String("country").Optional(),

		// Padrões de operação
		field.String("timezone").Default("America/Sao_Paulo"),
		field.Int("weekly_hours").Optional().Nillable(),
		field.Int("default_sprint_days").Optional().Nillable(),
		field.String("currency").Default("BRL"),

		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (Organization) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("persons", Person.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("projects", Project.Type),
		edge.To("customers", Customer.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("invites", Invite.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}
