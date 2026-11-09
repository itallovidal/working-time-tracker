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

// OAuthGrant é um app conectado pelo OAuth do servidor MCP: o claude.ai (que vale também no Desktop e no celular) ou o
// Claude Code de uma máquina, autorizado pela pessoa na tela de consentimento. A linha nasce com o código de
// autorização, que o app troca uma vez pelo token de renovação; depois guarda o token de renovação, que gira a cada uso.
// Como nas sessões, o banco guarda só o sha256 de cada código e token.
type OAuthGrant struct {
	ent.Schema
}

func (OAuthGrant) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "oauth_grants"}}
}

func (OAuthGrant) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("person_id", uuid.UUID{}),
		// O client_id do app: a URL do documento de identidade dele (CIMD), e o nome que o documento dá.
		field.String("client_id").Immutable(),
		field.String("client_name"),
		// O endereço de volta usado na autorização, exato (com a porta, no Claude Code): a troca do código o confere.
		field.String("redirect_uri").Immutable(),
		field.String("scope").Immutable(),
		// O recurso (o endereço do /mcp) para o qual os tokens valem, na forma canônica.
		field.String("resource").Immutable(),
		// O código de autorização. Continua guardado depois da troca, para um segundo uso ser reconhecido e derrubar
		// a concessão inteira.
		field.String("code_hash").Unique().Sensitive().Immutable(),
		field.String("code_challenge").Immutable(),
		field.Time("code_expires_at").Immutable(),
		field.Time("code_used_at").Optional().Nillable(),
		// O token de renovação em uso. Nulo até a troca do código.
		field.String("refresh_hash").Optional().Nillable().Unique().Sensitive(),
		// O token de renovação anterior, e quando ele foi trocado: dentro da janela curta, quem o manda de novo (dois
		// processos renovando ao mesmo tempo) recebe o mesmo sucessor, guardado cifrado; depois dela, é reuso.
		field.String("prev_refresh_hash").Optional().Nillable().Unique().Sensitive(),
		field.String("refresh_replay_enc").Optional().Nillable().Sensitive(),
		field.Time("rotated_at").Optional().Nillable(),
		// Quando a conexão cai sem uso: cada renovação empurra a data.
		field.Time("refresh_expires_at").Optional().Nillable(),
		// O último uso, gravado no máximo uma vez por minuto. Nulo: nunca foi usado.
		field.Time("last_used_at").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

// Indexes: a lista do perfil, o limite por pessoa, a troca de senha e o apagar da pessoa procuram por ela.
func (OAuthGrant) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("person_id"),
	}
}

func (OAuthGrant) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("person", Person.Type).Ref("oauth_grants").Field("person_id").Unique().Required(),
		edge.To("access_tokens", OAuthAccessToken.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}
