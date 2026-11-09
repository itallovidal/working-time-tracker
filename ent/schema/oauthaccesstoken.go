package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// OAuthAccessToken é um token de acesso de um app conectado (OAuthGrant): o que o app manda em Authorization: Bearer
// para o /mcp. Vale uma hora; o app pega outro com o token de renovação. O banco guarda só o sha256.
type OAuthAccessToken struct {
	ent.Schema
}

func (OAuthAccessToken) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "oauth_access_tokens"}}
}

func (OAuthAccessToken) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("grant_id", uuid.UUID{}),
		field.String("token_hash").Unique().Sensitive().Immutable(),
		field.Time("expires_at").Immutable(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

// Indexes: a limpeza dos vencidos de uma concessão procura por ela e pela validade.
func (OAuthAccessToken) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("grant_id", "expires_at"),
	}
}

func (OAuthAccessToken) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("grant", OAuthGrant.Type).Ref("access_tokens").Field("grant_id").Unique().Required(),
	}
}
