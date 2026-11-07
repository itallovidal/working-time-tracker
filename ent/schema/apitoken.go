package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// APIToken é um token pessoal de acesso: o que a pessoa gera no perfil para o Claude (o servidor MCP, em /mcp) agir
// em nome dela. Como a sessão, o banco guarda só o sha256 do token; o token em claro aparece uma vez, na criação.
// Ele só vale no /mcp: a API continua só com o cookie.
type APIToken struct {
	ent.Schema
}

func (APIToken) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("person_id", uuid.UUID{}),
		// O rótulo que a pessoa dá ao token ("Claude Code no notebook"), para reconhecê-lo na lista.
		field.String("name"),
		field.String("token_hash").Unique().Sensitive(),
		// O começo do token (wtt_ e alguns caracteres), para a lista mostrar qual é sem guardar o token.
		field.String("prefix"),
		field.Time("expires_at"),
		// A última vez que o token entrou, gravada no máximo uma vez por minuto. Nulo: nunca foi usado.
		field.Time("last_used_at").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

// Indexes: a lista, o limite por pessoa, a troca de senha e o apagar da pessoa procuram os tokens por ela.
func (APIToken) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("person_id"),
	}
}

func (APIToken) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("person", Person.Type).Ref("api_tokens").Field("person_id").Unique().Required(),
	}
}
