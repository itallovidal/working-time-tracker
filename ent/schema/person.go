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

type Person struct {
	ent.Schema
}

func (Person) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.String("name"),
		field.String("email"),
		field.UUID("organization_id", uuid.UUID{}),
		// Pessoas criadas antes do login existir não têm senha e não conseguem entrar.
		field.String("password_hash").Optional().Nillable().Sensitive(),
		field.Enum("role").Values("admin", "member").Default("member"),
		// O dono da organização: quem a criou. É sempre admin, é um só por organização
		// e as horas dele valem o valor cobrado, sem custo.
		field.Bool("is_owner").Default(false),
		// As permissões da organização que o dono liberou a quem não é admin (criar
		// projetos, cuidar dos clientes e das pessoas). Admins têm todas.
		field.Strings("permissions").Optional(),
		// Jornada semanal combinada com a pessoa, em horas. Vale para a organização
		// toda, e não por projeto.
		field.Int("weekly_hours").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (Person) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("organization", Organization.Type).Ref("persons").Field("organization_id").Unique().Required(),
		edge.To("tasks", Task.Type).Annotations(entsql.OnDelete(entsql.NoAction)),
		edge.To("team_memberships", TeamMembership.Type),
		edge.To("work_sessions", WorkSession.Type),
		edge.To("sessions", Session.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("created_invites", Invite.Type),
		edge.To("allocations", Allocation.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (Person) Indexes() []ent.Index {
	return []ent.Index{
		// O email identifica a conta no login, então é único no sistema todo.
		index.Fields("email").Unique(),
		// No máximo um dono por organização.
		index.Fields("organization_id").
			Unique().
			StorageKey("one_owner_per_organization").
			Annotations(entsql.IndexWhere("is_owner")),
	}
}
