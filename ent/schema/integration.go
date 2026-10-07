package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type Integration struct {
	ent.Schema
}

func (Integration) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("project_id", uuid.UUID{}),
		field.String("type"),
		field.String("display_name"),
		// credentials guarda o token criptografado; metadata, os campos próprios de
		// cada plataforma (repositório, quadro), em claro.
		field.JSON("credentials", map[string]interface{}{}).Optional().Sensitive(),
		field.JSON("metadata", map[string]interface{}{}).Optional(),
		field.Bool("enabled").Default(true),
		// A sincronização das issues com as tarefas (só o GitHub). sync_cursor é de onde a próxima
		// rodada incremental pede as issues mexidas; last_sync_error é o código do erro da última rodada.
		field.Bool("sync_issues").Default(false),
		field.Time("sync_cursor").Optional().Nillable(),
		field.Time("last_synced_at").Optional().Nillable(),
		field.String("last_sync_error").Default(""),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (Integration) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("project", Project.Type).Ref("integrations").Field("project_id").Unique().Required(),
		edge.To("tasks", Task.Type),
		// Apagar a integração apaga o vínculo das issues; as tarefas ficam.
		edge.To("issue_syncs", IssueSync.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}
