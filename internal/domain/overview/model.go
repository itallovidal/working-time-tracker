// Package overview é a visão geral de um projeto para admins: junta numa só
// resposta quem está nele, o tempo registrado, quanto esse tempo custou e
// rendeu, as tarefas e as integrações. Só lê o que os outros domínios guardam;
// não tem tabela própria.
package overview

import (
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/domain/project"
)

type Overview struct {
	Project      Project      `json:"project"`
	People       People       `json:"people"`
	Teams        Teams        `json:"teams"`
	Time         Time         `json:"time"`
	Money        Money        `json:"money"`
	Tasks        Tasks        `json:"tasks"`
	Integrations Integrations `json:"integrations"`
	// ByPerson traz só quem tem sessão no projeto, de quem mais trabalhou para
	// quem menos. A soma das linhas é igual aos totais de Time e Money.
	ByPerson []PersonTotal `json:"by_person"`
	// GeneratedAt é o instante da conta: as sessões abertas, a idade do projeto e
	// as janelas de 7 e 30 dias foram medidas até aqui.
	GeneratedAt time.Time `json:"generated_at"`
}

type Project struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	// CreatedAt é o início do projeto: o dia em que ele foi cadastrado.
	CreatedAt time.Time `json:"created_at"`
	Age       Age       `json:"age"`
	// Customer e BillRateCents são nil em projetos internos.
	Customer      *project.CustomerRef `json:"customer"`
	BillRateCents *int                 `json:"bill_rate_cents"`
}

// Age é há quanto tempo o projeto existe, em três unidades independentes, cada
// uma arredondada para baixo: 95 dias são 13 semanas e 3 meses.
type Age struct {
	Days   int `json:"days"`
	Weeks  int `json:"weeks"`
	Months int `json:"months"`
}

// People conta os colaboradores do projeto: quem tem valor por hora nele ou
// está em algum time dele, sem repetir.
type People struct {
	Total       int `json:"total"`
	WithoutTeam int `json:"without_team"`
	// WithoutRate são os que estão num time e ainda não têm valor por hora: não
	// conseguem bater ponto.
	WithoutRate int `json:"without_rate"`
	// WorkingNow são as pessoas com uma sessão aberta no projeto.
	WorkingNow int `json:"working_now"`
}

type Teams struct {
	Total int `json:"total"`
}

type Time struct {
	TotalSeconds float64 `json:"total_seconds"`
	SessionCount int     `json:"session_count"`
	// As duas janelas contam só o trecho de cada sessão que caiu dentro delas.
	Last7DaysSeconds  float64 `json:"last_7_days_seconds"`
	Last30DaysSeconds float64 `json:"last_30_days_seconds"`
	// FirstSessionAt e LastSessionAt são o início da sessão mais antiga e o da
	// mais recente; nil enquanto ninguém bateu ponto.
	FirstSessionAt *time.Time `json:"first_session_at"`
	LastSessionAt  *time.Time `json:"last_session_at"`
}

// Money é o que o tempo registrado custou (o que as pessoas recebem) e rendeu (o
// que o cliente paga). Cada valor é nil quando nenhuma sessão tem aquele valor
// por hora; a margem, quando não há receita.
type Money struct {
	PayAmountCents  *int `json:"pay_amount_cents"`
	BillAmountCents *int `json:"bill_amount_cents"`
	MarginCents     *int `json:"margin_cents"`
}

type Tasks struct {
	Total int `json:"total"`
	// Overdue são as tarefas com prazo vencido. Tarefa sem prazo não entra.
	Overdue int `json:"overdue"`
}

type Integrations struct {
	Total   int               `json:"total"`
	Enabled int               `json:"enabled"`
	Items   []IntegrationItem `json:"items"`
}

// IntegrationItem é o que a visão geral mostra de uma integração: o que ela é e
// se está em uso, sem a credencial nem os campos da plataforma.
type IntegrationItem struct {
	ID          uuid.UUID `json:"id"`
	Type        string    `json:"type"`
	DisplayName string    `json:"display_name"`
	Enabled     bool      `json:"enabled"`
	HasToken    bool      `json:"has_token"`
	CreatedAt   time.Time `json:"created_at"`
}

type Person struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type PersonTotal struct {
	Person Person `json:"person"`
	// InProject é false para quem já saiu do projeto e deixou sessões nele.
	InProject       bool    `json:"in_project"`
	WorkingNow      bool    `json:"working_now"`
	TotalSeconds    float64 `json:"total_seconds"`
	SessionCount    int     `json:"session_count"`
	PayAmountCents  *int    `json:"pay_amount_cents"`
	BillAmountCents *int    `json:"bill_amount_cents"`
}
