package task

import (
	"time"

	"github.com/google/uuid"
)

type Person struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Email string    `json:"email"`
}

type Integration struct {
	ID   uuid.UUID `json:"id"`
	Type string    `json:"type"`
}

// Priorities são as prioridades de uma tarefa, da mais para a menos urgente.
// "none" é a tarefa sem prioridade.
var Priorities = []string{"urgent", "high", "medium", "low", "none"}

func validPriority(p string) bool {
	for _, v := range Priorities {
		if v == p {
			return true
		}
	}
	return false
}

// Statuses são os estados de uma tarefa, na ordem em que ela costuma passar por eles.
// Toda tarefa nasce em "backlog".
var Statuses = []string{"backlog", "in_progress", "awaiting_closure", "closed"}

// StatusBacklog é o estado em que toda tarefa nova é criada.
const StatusBacklog = "backlog"

func validStatus(s string) bool {
	for _, v := range Statuses {
		if v == s {
			return true
		}
	}
	return false
}

// Label é uma etiqueta de tarefa; cada projeto tem as suas.
type Label struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// Attrs são a prioridade, o status e as etiquetas que uma tarefa recebe ao ser criada ou
// editada. Nil mantém o que a tarefa já tem (ou, na criação, o padrão: sem prioridade e
// sem etiquetas); uma lista vazia de etiquetas as tira. O status só vale na edição: a
// tarefa nova é sempre criada em backlog, e na criação o Status é ignorado.
type Attrs struct {
	Priority *string
	Status   *string
	LabelIDs *[]string
}

type Task struct {
	ID                    uuid.UUID    `json:"id"`
	ProjectID             uuid.UUID    `json:"project_id"`
	Name                  string       `json:"name"`
	Description           string       `json:"description"`
	Priority              string       `json:"priority"`
	Status                string       `json:"status"`
	Labels                []Label      `json:"labels"`
	AssigneeID            *uuid.UUID   `json:"assignee_id"`
	Assignee              *Person      `json:"assignee,omitempty"`
	Deadline              time.Time    `json:"deadline"`
	ExternalIntegrationID *uuid.UUID   `json:"external_integration_id,omitempty"`
	ExternalIntegration   *Integration `json:"external_integration,omitempty"`
	ExternalItemID        *string      `json:"external_item_id,omitempty"`
	ExternalItemURL       *string      `json:"external_item_url,omitempty"`
	CreatedAt             time.Time    `json:"created_at"`
}

// ListFilter restringe a lista de tarefas de um projeto. O valor zero lista tudo.
type ListFilter struct {
	Query      string      // trecho do nome, sem diferenciar maiúsculas
	AssigneeID *uuid.UUID  // só as tarefas desta pessoa
	Unassigned bool        // só as tarefas sem responsável; vale no lugar de AssigneeID
	Assigned   bool        // só as tarefas que têm responsável, de qualquer pessoa; vale no lugar de AssigneeID e perde para Unassigned
	OthersOf   *uuid.UUID  // só as tarefas que têm responsável e não é esta pessoa; vale no lugar de AssigneeID e perde para Unassigned e Assigned
	DeadlineTo *time.Time  // só as com prazo até este instante, inclusive
	Priorities []string    // só as com uma destas prioridades
	Statuses   []string    // só as com um destes status
	LabelIDs   []uuid.UUID // só as que têm alguma destas etiquetas
	Page       int         // a partir de 1; zero é a lista inteira
	PerPage    int
}

// Page é uma página da lista de tarefas. Total conta tudo o que passa pelos
// filtros, e Assignees traz quem é responsável por alguma tarefa do projeto,
// para o filtro alcançar também quem já saiu dos times.
type Page struct {
	Items     []Task   `json:"items"`
	Total     int      `json:"total"`
	Page      int      `json:"page"`
	PerPage   int      `json:"per_page"`
	Assignees []Person `json:"assignees"`
}
