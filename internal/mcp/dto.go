package mcp

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/work_session"
)

// As saídas das ferramentas. Os ids e as datas vão como texto (RFC 3339 no fuso da organização; o prazo, só a data),
// e nenhuma tem dinheiro.

// Ref é um projeto ou uma tarefa pelo nome.
type Ref struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// TaskOut é uma tarefa.
type TaskOut struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Priority  string `json:"priority"`
	// Deadline é o dia do prazo (AAAA-MM-DD), ou vazio quando a tarefa não tem prazo.
	Deadline string       `json:"deadline"`
	Assignee *AssigneeOut `json:"assignee"`
	Labels   []string     `json:"labels"`
	Links    []LinkOut    `json:"links"`
	// Description é a descrição em Markdown; só a get_task a traz.
	Description string `json:"description,omitempty"`
	// URL é a página da tarefa no sistema, quando o endereço público está configurado.
	URL       string `json:"url,omitempty"`
	CreatedAt string `json:"created_at"`
}

// AssigneeOut é o responsável pela tarefa. IsYou marca quando é a própria pessoa do token.
type AssigneeOut struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	IsYou bool   `json:"is_you"`
}

// LinkOut é o item de uma plataforma ligado à tarefa (a issue do GitHub, o cartão do Trello).
type LinkOut struct {
	Platform string `json:"platform"`
	ItemID   string `json:"item_id"`
	URL      string `json:"url"`
	// LastError é o aviso que a última sincronização deixou no vínculo; vazio quando está tudo em ordem.
	LastError string `json:"last_error,omitempty"`
}

// SessionOut é uma sessão de ponto da pessoa.
type SessionOut struct {
	ID        string `json:"id"`
	Project   Ref    `json:"project"`
	StartedAt string `json:"started_at"`
	// EndedAt é vazio enquanto a sessão está aberta.
	EndedAt        string `json:"ended_at,omitempty"`
	Open           bool   `json:"open"`
	ElapsedSeconds int    `json:"elapsed_seconds"`
	Elapsed        string `json:"elapsed"`
	// Tasks são os trechos de cada tarefa na sessão, na ordem em que entraram.
	Tasks []SessionTaskOut `json:"tasks"`
}

// SessionTaskOut é o trecho de uma tarefa na sessão. Running diz que ele não tem fim (a tarefa está sendo
// trabalhada agora, numa sessão aberta).
type SessionTaskOut struct {
	TaskID  string `json:"task_id"`
	Name    string `json:"name"`
	From    string `json:"from"`
	Until   string `json:"until,omitempty"`
	Running bool   `json:"running"`
	Seconds int    `json:"seconds"`
}

func (s *server) taskOut(t *task.Task, me *auth.Identity, loc *time.Location, withDescription bool) TaskOut {
	out := TaskOut{
		ID:        t.ID.String(),
		ProjectID: t.ProjectID.String(),
		Name:      t.Name,
		Status:    t.Status,
		Priority:  t.Priority,
		Labels:    []string{},
		Links:     []LinkOut{},
		CreatedAt: t.CreatedAt.In(loc).Format(time.RFC3339),
	}
	if t.HasDeadline() {
		out.Deadline = t.Deadline.In(loc).Format(time.DateOnly)
	}
	if t.AssigneeID != nil {
		a := &AssigneeOut{ID: t.AssigneeID.String(), IsYou: *t.AssigneeID == me.PersonID}
		if t.Assignee != nil {
			a.Name = t.Assignee.Name
		}
		out.Assignee = a
	}
	for _, l := range t.Labels {
		out.Labels = append(out.Labels, l.Name)
	}
	for _, l := range t.Links {
		link := LinkOut{ItemID: l.ItemID, URL: l.URL, LastError: l.LastError}
		if l.Integration != nil {
			link.Platform = l.Integration.Type
		}
		out.Links = append(out.Links, link)
	}
	if withDescription {
		out.Description = t.Description
	}
	out.URL = s.taskURL(out.ID)
	return out
}

// taskURL é a página da tarefa no sistema, ou vazio quando o endereço público não está configurado.
func (s *server) taskURL(taskID string) string {
	if s.PublicURL == "" {
		return ""
	}
	return s.PublicURL + "/tasks/" + taskID
}

func sessionOut(ws *work_session.WorkSession, projectName string, loc *time.Location, now time.Time) SessionOut {
	end := now
	if ws.EndAt != nil {
		end = *ws.EndAt
	}
	elapsed := int(math.Max(0, end.Sub(ws.StartAt).Seconds()))
	out := SessionOut{
		ID:             ws.ID.String(),
		Project:        Ref{ID: ws.ProjectID.String(), Name: projectName},
		StartedAt:      ws.StartAt.In(loc).Format(time.RFC3339),
		Open:           ws.EndAt == nil,
		ElapsedSeconds: elapsed,
		Elapsed:        duration(elapsed),
		Tasks:          []SessionTaskOut{},
	}
	if ws.EndAt != nil {
		out.EndedAt = ws.EndAt.In(loc).Format(time.RFC3339)
	}
	for _, st := range ws.Tasks {
		item := SessionTaskOut{
			TaskID:  st.TaskID.String(),
			From:    st.FromAt.In(loc).Format(time.RFC3339),
			Running: ws.EndAt == nil && st.UntilAt == nil,
			Seconds: int(st.Seconds),
		}
		if st.Task != nil {
			item.Name = st.Task.Name
		}
		if st.UntilAt != nil {
			item.Until = st.UntilAt.In(loc).Format(time.RFC3339)
		}
		out.Tasks = append(out.Tasks, item)
	}
	return out
}

// duration escreve segundos como "2h05m".
func duration(seconds int) string {
	h, m := seconds/3600, (seconds%3600)/60
	if h == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dh%02dm", h, m)
}

// location é o fuso da organização da pessoa, que decide o dia de um prazo e a hora que volta nas saídas. Ele vem
// com a identidade (lida a cada chamada, junto com o token), então nada se consulta aqui. Sem fuso vale o padrão das
// organizações; um que não se lê vale UTC.
func (s *server) location(me *auth.Identity) *time.Location {
	name := me.OrganizationTimezone
	if name == "" {
		name = organization.DefaultTimezone
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
}

// parseDeadline lê o prazo: um dia (AAAA-MM-DD) vale às 23:59 do relógio dele no fuso da organização, como a tela
// grava (também no dia em que o horário de verão começa ou termina); um instante RFC 3339 vale como veio.
func parseDeadline(raw string, loc *time.Location) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if d, err := time.ParseInLocation(time.DateOnly, raw, loc); err == nil {
		t := time.Date(d.Year(), d.Month(), d.Day(), 23, 59, 0, 0, loc)
		return &t, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return &t, nil
	}
	return nil, ErrInvalidDate.With("field", "deadline")
}

// schemaFor é o esquema de entrada do tipo, com os valores que cada campo aceita (enum). Num campo que é uma lista, os
// valores valem para cada item.
func schemaFor[T any](enums map[string][]string) *jsonschema.Schema {
	sc, err := jsonschema.For[T](nil)
	if err != nil {
		panic(err)
	}
	for field, values := range enums {
		prop, ok := sc.Properties[field]
		if !ok {
			panic("mcp: no field " + field)
		}
		if prop.Items != nil {
			prop = prop.Items
		}
		for _, v := range values {
			prop.Enum = append(prop.Enum, v)
		}
	}
	return sc
}
