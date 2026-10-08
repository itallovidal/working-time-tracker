package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// IssueSync é o vínculo de um item da plataforma (uma issue do GitHub, um cartão do Trello) com uma
// tarefa, e o único lugar onde esse vínculo mora. Uma tarefa tem no máximo um vínculo por integração, então
// pode estar ao mesmo tempo numa issue e num cartão. Guarda também o snapshot do último acordo entre os dois
// lados (título, corpo, etiquetas, responsáveis, prazo): é contra ele que se vê quem mudou o quê desde a
// última rodada.
//
// task_id nulo é um item descartado: a tarefa foi excluída aqui (ou desligada do item), e a linha fica para o
// item não ser importado de novo.
//
// state "pending" é o vínculo feito à mão (ou republicado) que ainda não foi adotado pela sincronização:
// ainda não há acordo, e a primeira rodada em que o item está aberto grava o snapshot.
type IssueSync struct {
	ent.Schema
}

func (IssueSync) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("integration_id", uuid.UUID{}),
		field.UUID("task_id", uuid.UUID{}).Optional().Nillable(),
		// A chave do item na plataforma, em texto: o número da issue, o link curto do cartão.
		field.String("item_id"),
		// O endereço do item na plataforma (a página da issue, do cartão), para o cartão da tarefa abrir mesmo
		// antes de buscar os detalhes. Vazio quando ninguém o informou.
		field.String("url").Default(""),
		// O estado da issue na última rodada. "gone" é a issue que sumiu do repositório (apagada ou
		// transferida): a tarefa fica como estava. "pending" é o vínculo ainda não adotado.
		field.Enum("state").Values("open", "closed", "gone", "pending").Default("open"),

		// O snapshot do último acordo.
		field.String("title").Default(""),
		field.String("body").Default(""),
		field.JSON("labels", []string{}).Optional(),
		field.JSON("assignee_logins", []string{}).Optional(),
		// O login do GitHub que o responsável da tarefa representa, e a pessoa que ele era quando se
		// ligou. Sem chave estrangeira: a pessoa pode sair, e o snapshot só serve de comparação.
		field.String("mapped_login").Default(""),
		field.UUID("mapped_person_id", uuid.UUID{}).Optional().Nillable(),
		// O prazo do último acordo; nulo é sem prazo (e todo tipo que não espelha o prazo).
		field.Time("deadline").Optional().Nillable(),

		// stuck_sig é a assinatura do último empurrão que o GitHub descartou sem erro; igual à do
		// empurrão de agora, não se tenta de novo (senão a rodada nunca acaba de empurrar). last_error
		// é o código do erro que o cartão mostra.
		field.String("stuck_sig").Default(""),
		field.String("last_error").Default(""),
		field.Time("synced_at").Default(time.Now),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (IssueSync) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("integration", Integration.Type).Ref("issue_syncs").Field("integration_id").Unique().Required(),
		// Uma tarefa tem no máximo um item por integração; excluir a tarefa só solta o vínculo (task_id fica nulo).
		edge.From("task", Task.Type).Ref("issue_syncs").Field("task_id").Unique(),
	}
}

func (IssueSync) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("integration_id", "item_id").Unique(),
		// NULL não conta como igual no Postgres: as linhas descartadas (task_id nulo) não se atrapalham.
		index.Fields("task_id", "integration_id").Unique(),
	}
}
