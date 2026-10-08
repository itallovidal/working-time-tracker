package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type Project struct {
	ent.Schema
}

func (Project) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("organization_id", uuid.UUID{}),
		field.String("name"),
		field.String("description").Optional(),
		field.String("github_repo_url").Optional().Nillable(),
		field.String("gitlab_repo_url").Optional().Nillable(),
		field.Int("sprint_duration_days").Default(14),
		field.String("daily_time").Optional().Nillable(),
		field.String("weekly_sync_day").Optional().Nillable(),
		field.String("weekly_sync_time").Optional().Nillable(),
		// A reunião semanal com o cliente: só faz sentido num projeto com cliente.
		field.String("customer_meeting_day").Optional().Nillable(),
		field.String("customer_meeting_time").Optional().Nillable(),
		// Projeto interno fica sem cliente e sem valor cobrado.
		field.UUID("customer_id", uuid.UUID{}).Optional().Nillable(),
		// O que o cliente paga à organização por hora neste projeto.
		field.Int("bill_rate_cents").Optional().Nillable().NonNegative(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (Project) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("organization", Organization.Type).Ref("projects").Field("organization_id").Unique().Required(),
		edge.From("customer", Customer.Type).Ref("projects").Field("customer_id").Unique(),
		edge.To("teams", Team.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("tasks", Task.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("work_sessions", WorkSession.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("integrations", Integration.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("allocations", Allocation.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
		// Um convite que levava a pessoa a este projeto continua valendo, só para a organização.
		edge.To("invites", Invite.Type).Annotations(entsql.OnDelete(entsql.SetNull)),
		edge.To("labels", Label.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}
