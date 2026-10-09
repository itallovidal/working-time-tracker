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

		// Contato
		field.String("website").Optional(),
		field.String("contact_email").Optional(),
		field.String("linkedin_url").Optional(),

		// Dados jurídicos. O país é o código (BR, US) e decide qual documento vale: o CNPJ é do Brasil e o EIN é dos EUA,
		// sem máscara. Trocar o país não apaga o documento do outro, só o esconde. Quem decide as regras de cada país é
		// internal/country.
		field.String("legal_name").Optional(),
		field.String("cnpj").Optional(),
		field.String("ein").Optional(),
		field.String("address_line1").Optional(),
		field.String("address_line2").Optional(),
		field.String("country").Default("BR"),

		// Como a organização trabalha. Sprint, daily e weekly ficam no projeto, porque
		// projetos diferentes podem trabalhar de formas diferentes.
		field.String("work_mode").Optional(),
		field.String("timezone").Default("America/Sao_Paulo"),
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
