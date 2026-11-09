package mcp

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
)

// As ferramentas de quem é a pessoa, dos projetos dela e das horas dela.

// noInput é a entrada das ferramentas que não pedem nada.
type noInput struct{}

// WhoAmIOut é a pessoa do token.
type WhoAmIOut struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	// Role é owner (o dono da organização), admin ou member.
	Role         string `json:"role"`
	Organization OrgOut `json:"organization"`
	// OpenSessions são as sessões de ponto abertas, uma por projeto no máximo; vazia com o ponto fechado.
	OpenSessions []SessionOut `json:"open_sessions"`
	// TokenExpiresAt é quando o token desta conexão vence; depois disso a pessoa gera outro no perfil.
	TokenExpiresAt string `json:"token_expires_at"`
}

// OrgOut é a organização da pessoa.
type OrgOut struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Solo     bool   `json:"solo"`
	Timezone string `json:"timezone"`
}

// ProjectsOut é a lista de projetos da pessoa.
type ProjectsOut struct {
	Projects []ProjectOut `json:"projects"`
}

// ProjectOut é um projeto. Customer é vazio num projeto interno.
type ProjectOut struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Customer  string `json:"customer"`
	TaskCount int    `json:"task_count"`
}

type hoursInput struct {
	Timezone string `json:"timezone,omitempty" jsonschema:"IANA time zone that decides where today and the week begin, like America/Sao_Paulo. Defaults to the organization's."`
}

// HoursOut são as horas da pessoa: hoje, a semana (de segunda a domingo) e para onde elas foram.
type HoursOut struct {
	Timezone  string `json:"timezone"`
	WeekStart string `json:"week_start"`
	// WeeklyHours é a jornada semanal combinada; null quando ninguém informou.
	WeeklyHours  *int          `json:"weekly_hours"`
	TodaySeconds int           `json:"today_seconds"`
	Today        string        `json:"today"`
	WeekSeconds  int           `json:"week_seconds"`
	Week         string        `json:"week"`
	Days         []DayOut      `json:"days"`
	Projects     []ProjectTime `json:"projects"`
	Tasks        TaskCounts    `json:"tasks"`
	WorkingNow   bool          `json:"working_now"`
	WorkingOn    []WorkingOut  `json:"working_on"`
}

// DayOut é o tempo de um dia da semana.
type DayOut struct {
	Date    string `json:"date"`
	Seconds int    `json:"seconds"`
	Hours   string `json:"hours"`
}

// ProjectTime é o tempo da semana num projeto.
type ProjectTime struct {
	Project Ref    `json:"project"`
	Seconds int    `json:"seconds"`
	Hours   string `json:"hours"`
}

// TaskCounts conta as tarefas da pessoa.
type TaskCounts struct {
	Open            int `json:"open"`
	Backlog         int `json:"backlog"`
	InProgress      int `json:"in_progress"`
	AwaitingClosure int `json:"awaiting_closure"`
	Closed          int `json:"closed"`
	Overdue         int `json:"overdue"`
	DueThisWeek     int `json:"due_this_week"`
}

// WorkingOut é uma tarefa em que a pessoa trabalha agora.
type WorkingOut struct {
	Task    Ref `json:"task"`
	Project Ref `json:"project"`
}

func (s *server) addAccountTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "whoami",
		Description: "Who you are acting as: the person, their role, their organization, the open work session (if any) and when the token expires.",
		Annotations: readOnly("Who am I"),
	}, s.whoami)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_projects",
		Description: "The projects the person works in (admins see all of the organization's). Use the ids with the task tools.",
		Annotations: readOnly("List projects"),
	}, s.listProjects)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "my_hours",
		Description: "How much the person worked today and this week (Monday to Sunday), day by day and by project, plus counts of their tasks and what they are working on now.",
		Annotations: readOnly("My hours"),
	}, s.myHours)
}

func (s *server) whoami(_ context.Context, req *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, WhoAmIOut, error) {
	id, err := me(req)
	if err != nil {
		return nil, WhoAmIOut{}, s.fail(err)
	}
	loc := s.location(id)
	out := WhoAmIOut{
		ID: id.PersonID.String(), Name: id.Name, Email: id.Email, Role: roleOf(id),
		Organization: OrgOut{ID: id.OrganizationID.String(), Name: id.OrganizationName, Solo: id.OrganizationSolo, Timezone: loc.String()},
	}
	if req.Extra.TokenInfo != nil && !req.Extra.TokenInfo.Expiration.IsZero() {
		out.TokenExpiresAt = req.Extra.TokenInfo.Expiration.In(loc).Format(time.RFC3339)
	}
	open, err := s.Sessions.Open(id.PersonID.String())
	if err != nil {
		return nil, WhoAmIOut{}, s.fail(err)
	}
	out.OpenSessions = make([]SessionOut, 0, len(open))
	for i := range open {
		out.OpenSessions = append(out.OpenSessions, s.sessionOut(&open[i], id))
	}
	return nil, out, nil
}

func roleOf(id *auth.Identity) string {
	if id.IsOwner {
		return "owner"
	}
	return id.Role
}

func (s *server) listProjects(_ context.Context, req *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, ProjectsOut, error) {
	id, err := me(req)
	if err != nil {
		return nil, ProjectsOut{}, s.fail(err)
	}
	// O mesmo filtro da lista da API: quem não é admin vê só os projetos em que está.
	var member = &id.PersonID
	if id.IsAdmin() {
		member = nil
	}
	projects, err := s.Projects.ListByOrg(id.OrganizationID.String(), member)
	if err != nil {
		return nil, ProjectsOut{}, s.fail(err)
	}
	out := ProjectsOut{Projects: []ProjectOut{}}
	for _, p := range projects {
		item := ProjectOut{ID: p.ID.String(), Name: p.Name, TaskCount: p.TaskCount}
		if p.Customer != nil {
			item.Customer = p.Customer.Name
		}
		out.Projects = append(out.Projects, item)
	}
	return nil, out, nil
}

func (s *server) myHours(_ context.Context, req *mcp.CallToolRequest, in hoursInput) (*mcp.CallToolResult, HoursOut, error) {
	id, err := me(req)
	if err != nil {
		return nil, HoursOut{}, s.fail(err)
	}
	tz := in.Timezone
	if tz == "" {
		tz = s.location(id).String()
	}
	m, err := s.Overview.Mine(id.OrganizationID.String(), id.PersonID.String(), tz, !id.IsAdmin())
	if err != nil {
		return nil, HoursOut{}, s.fail(err)
	}
	loc, err := time.LoadLocation(m.Timezone)
	if err != nil {
		loc = time.UTC
	}
	out := HoursOut{
		Timezone:     m.Timezone,
		WeekStart:    m.WeekStart.In(loc).Format(time.DateOnly),
		WeeklyHours:  m.WeeklyHours,
		TodaySeconds: int(m.Hours.TodaySeconds),
		Today:        duration(int(m.Hours.TodaySeconds)),
		WeekSeconds:  int(m.Hours.WeekSeconds),
		Week:         duration(int(m.Hours.WeekSeconds)),
		Days:         []DayOut{},
		Projects:     []ProjectTime{},
		Tasks: TaskCounts{
			Open: m.Tasks.Open, Backlog: m.Tasks.Backlog, InProgress: m.Tasks.InProgress,
			AwaitingClosure: m.Tasks.AwaitingClosure, Closed: m.Tasks.Closed, Overdue: m.Tasks.Overdue,
			DueThisWeek: m.Tasks.DueThisWeek,
		},
		WorkingNow: m.WorkingNow,
		WorkingOn:  []WorkingOut{},
	}
	for _, d := range m.Hours.Days {
		out.Days = append(out.Days, DayOut{Date: d.Date, Seconds: int(d.Seconds), Hours: duration(int(d.Seconds))})
	}
	for _, p := range m.Hours.Projects {
		out.Projects = append(out.Projects, ProjectTime{
			Project: Ref{ID: p.Project.ID.String(), Name: p.Project.Name}, Seconds: int(p.Seconds), Hours: duration(int(p.Seconds)),
		})
	}
	for _, w := range m.WorkingOn {
		out.WorkingOn = append(out.WorkingOn, WorkingOut{
			Task: Ref{ID: w.Task.ID.String(), Name: w.Task.Name}, Project: Ref{ID: w.Project.ID.String(), Name: w.Project.Name},
		})
	}
	return nil, out, nil
}

// reachProject confere que a pessoa alcança o projeto, com a regra do RequireOrg. Quem não alcança recebe
// project.not_found, como na API.
func (s *server) reachProject(id *auth.Identity, projectID string) error {
	ok, err := s.Resolver.Reach(id, auth.KindProject, projectID)
	if err != nil {
		return s.fail(err)
	}
	if !ok {
		return s.fail(project.ErrNotFound)
	}
	return nil
}

// reachTask lê a tarefa e confere que a pessoa alcança o projeto dela (o que inclui a organização), com a regra do
// RequireOrg. Quem não alcança, e um id que não é de tarefa nenhuma, recebe task.not_found, como na API. A tarefa que
// volta tem o id na forma canônica: quem chama usa t.ID, e não o texto que o Claude mandou.
func (s *server) reachTask(id *auth.Identity, rawID string) (*task.Task, error) {
	taskID, err := uuid.Parse(rawID)
	if err != nil {
		return nil, s.fail(task.ErrNotFound)
	}
	t, err := s.Tasks.Get(taskID.String())
	if errors.Is(err, database.ErrNotFound) {
		return nil, s.fail(task.ErrNotFound)
	}
	if err != nil {
		return nil, s.fail(err)
	}
	ok, err := s.Resolver.Reach(id, auth.KindProject, t.ProjectID.String())
	if err != nil {
		return nil, s.fail(err)
	}
	if !ok {
		return nil, s.fail(task.ErrNotFound)
	}
	return t, nil
}

// projectName é o nome do projeto, para as saídas; vazio quando não se acha.
func (s *server) projectName(projectID string) string {
	p, err := s.Projects.Get(projectID)
	if err != nil {
		return ""
	}
	return p.Name
}
