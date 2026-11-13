package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// PersonAccount é a conta da pessoa numa plataforma (o usuário dela no GitHub ou no GitLab), ou a resposta de que
// ela não tem uma. A sincronização liga o responsável da issue à pessoa por aqui, antes de tentar o e-mail público.
type PersonAccount struct {
	ent.Schema
}

func (PersonAccount) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("person_id", uuid.UUID{}),
		// A organização da pessoa, repetida aqui para o login ser único dentro dela.
		field.UUID("organization_id", uuid.UUID{}),
		field.Enum("platform").Values("github", "gitlab"),
		// O login na plataforma, como ela o devolve. Vazio quando a pessoa disse que não tem conta.
		field.String("username").Default(""),
		// O e-mail que a plataforma devolveu ao conectar, só para mostrar. Vazio quando ela não deu um.
		field.String("email").Default(""),
		// A pessoa disse que não tem conta nesta plataforma: o modal de pegar a tarefa não pergunta de novo.
		field.Bool("no_account").Default(false),
		// De onde veio o vínculo: a própria pessoa autorizou (oauth), conectou uma integração (integration), um admin
		// escolheu a pessoa de um login (admin), o e-mail público casou (email) ou ela disse que não tem conta (self).
		// O oauth vence os outros.
		field.Enum("source").Values("oauth", "integration", "admin", "email", "self"),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (PersonAccount) Indexes() []ent.Index {
	return []ent.Index{
		// Uma conta por pessoa em cada plataforma.
		index.Fields("person_id", "platform").Unique(),
		// Um login liga a uma pessoa só dentro da organização.
		index.Fields("organization_id", "platform", "username").
			Unique().
			StorageKey("one_person_per_platform_login").
			Annotations(entsql.IndexWhere("((username)::text <> ''::text)")),
	}
}

func (PersonAccount) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("person", Person.Type).Ref("accounts").Field("person_id").Unique().Required(),
		edge.From("organization", Organization.Type).Ref("person_accounts").Field("organization_id").Unique().Required(),
	}
}
