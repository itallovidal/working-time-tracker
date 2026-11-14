package mcp

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/domain/issuesync"
	"working-time-tracker/internal/domain/overview"
	"working-time-tracker/internal/domain/permission"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/validate"
)

// As ferramentas das tarefas e das etiquetas. Cada uma confere antes que a pessoa alcança o projeto ou a tarefa
// (reachProject, reachTask), e o resto das regras é dos services: o responsável precisa estar no projeto, o nome é
// obrigatório, a tarefa nova nasce em backlog, a que já é de outra pessoa não se pega. Criar etiqueta pede, além
// disso, o labels.manage no projeto, como a rota da API.

// defaultPerPage é o tamanho de página das listas quando o Claude não pede outro.
const defaultPerPage = 20

// openStatuses são os status de uma tarefa que não terminou: o que list_tasks mostra sem filtro de status.
var openStatuses = []string{"backlog", "in_progress", "awaiting_closure"}

type projectInput struct {
	ProjectID string `json:"project_id" jsonschema:"The project id, from list_projects."`
}

type taskInput struct {
	TaskID string `json:"task_id" jsonschema:"The task id, from list_tasks or my_tasks."`
}

type listTasksInput struct {
	ProjectID  string   `json:"project_id" jsonschema:"The project id, from list_projects."`
	Query      string   `json:"query,omitempty" jsonschema:"Part of the task name, case-insensitive."`
	Statuses   []string `json:"statuses,omitempty" jsonschema:"Only tasks in one of these statuses. Without it, only the open ones (every status but closed)."`
	Priorities []string `json:"priorities,omitempty" jsonschema:"Only tasks with one of these priorities."`
	Assignee   string   `json:"assignee,omitempty" jsonschema:"me (yours), none (no assignee), any (someone's) or others (someone else's). Without it, everyone's."`
	Labels     []string `json:"labels,omitempty" jsonschema:"Only tasks with any of these labels, by name."`
	Page       int      `json:"page,omitempty" jsonschema:"Page number, from 1."`
	PerPage    int      `json:"per_page,omitempty" jsonschema:"Tasks per page, up to 100. Defaults to 20."`
}

type myTasksInput struct {
	State   string `json:"state,omitempty" jsonschema:"open (default: not closed) or closed."`
	Page    int    `json:"page,omitempty" jsonschema:"Page number, from 1."`
	PerPage int    `json:"per_page,omitempty" jsonschema:"Tasks per page, up to 100. Defaults to 20."`
}

type createTaskInput struct {
	ProjectID   string   `json:"project_id" jsonschema:"The project id, from list_projects."`
	Name        string   `json:"name" jsonschema:"The task name."`
	Description string   `json:"description,omitempty" jsonschema:"The description, in Markdown."`
	Priority    string   `json:"priority,omitempty" jsonschema:"Defaults to none."`
	Deadline    string   `json:"deadline,omitempty" jsonschema:"YYYY-MM-DD. Without it, the deadline is 7 days from now."`
	Labels      []string `json:"labels,omitempty" jsonschema:"Label names of the project (see list_labels)."`
	AssignToMe  bool     `json:"assign_to_me,omitempty" jsonschema:"Make the person the assignee. Without it the task has no assignee (clocking in on it later makes it theirs)."`
}

type updateTaskInput struct {
	TaskID        string    `json:"task_id" jsonschema:"The task id."`
	Name          string    `json:"name,omitempty" jsonschema:"New name. Without it, the name stays."`
	Description   *string   `json:"description,omitempty" jsonschema:"New description, in Markdown; an empty string clears it. Without it, it stays."`
	Status        string    `json:"status,omitempty" jsonschema:"New status."`
	Priority      string    `json:"priority,omitempty" jsonschema:"New priority."`
	Deadline      string    `json:"deadline,omitempty" jsonschema:"New deadline, YYYY-MM-DD."`
	ClearDeadline bool      `json:"clear_deadline,omitempty" jsonschema:"Remove the deadline."`
	Labels        *[]string `json:"labels,omitempty" jsonschema:"Replaces the labels, by name; an empty list removes them all. Without it, they stay. Not with add_labels or remove_labels."`
	AddLabels     []string  `json:"add_labels,omitempty" jsonschema:"Adds these labels, by name, and keeps the others."`
	RemoveLabels  []string  `json:"remove_labels,omitempty" jsonschema:"Removes these labels, by name, and keeps the others."`
	Assignee      string    `json:"assignee,omitempty" jsonschema:"me to make the person the assignee, none to leave the task without one."`
}

type createLabelInput struct {
	ProjectID string `json:"project_id" jsonschema:"The project id, from list_projects."`
	Name      string `json:"name" jsonschema:"The label name, up to 50 characters."`
}

// TasksOut é uma página de tarefas.
type TasksOut struct {
	Items   []TaskOut `json:"items"`
	Total   int       `json:"total"`
	Page    int       `json:"page"`
	PerPage int       `json:"per_page"`
}

// MyTaskOut é uma tarefa da pessoa, com o projeto dela.
type MyTaskOut struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
	Deadline string `json:"deadline"`
	Project  Ref    `json:"project"`
	URL      string `json:"url,omitempty"`
}

// MyTasksOut é uma página das tarefas da pessoa.
type MyTasksOut struct {
	Items   []MyTaskOut `json:"items"`
	Total   int         `json:"total"`
	Page    int         `json:"page"`
	PerPage int         `json:"per_page"`
}

// LabelsOut são as etiquetas de um projeto.
type LabelsOut struct {
	Labels []string `json:"labels"`
}

// LabelOut é a etiqueta que create_label devolve: a nova ou, com created false, a que o projeto já tinha com o mesmo
// nome.
type LabelOut struct {
	Name    string `json:"name"`
	Created bool   `json:"created"`
}

// ProjectSyncOut é o que a sincronização do projeto fez, somando as integrações dele.
type ProjectSyncOut struct {
	Created  int  `json:"created"`
	Updated  int  `json:"updated"`
	Closed   int  `json:"closed"`
	Pushed   int  `json:"pushed"`
	Unmapped int  `json:"unmapped"`
	Errors   int  `json:"errors"`
	Partial  bool `json:"partial"`
}

// SyncOut é o que a sincronização da tarefa fez, e a tarefa depois dela.
type SyncOut struct {
	Created  int     `json:"created"`
	Updated  int     `json:"updated"`
	Closed   int     `json:"closed"`
	Pushed   int     `json:"pushed"`
	Unmapped int     `json:"unmapped"`
	Errors   int     `json:"errors"`
	Partial  bool    `json:"partial"`
	Problem  string  `json:"problem,omitempty"`
	Task     TaskOut `json:"task"`
}

func (s *server) addTaskTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_labels",
		Description: "The labels of a project, by name. create_task and update_task take these names; create_label adds a new one.",
		Annotations: readOnly("List labels"),
	}, s.listLabels)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_tasks",
		Description: "The tasks of a project, newest first, one page at a time. Without statuses it lists only open tasks. The query looks at the name only; to search every project, the description or by issue, use find_tasks.",
		InputSchema: schemaFor[listTasksInput](map[string][]string{
			"statuses": task.Statuses, "priorities": task.Priorities, "assignee": {"me", "none", "any", "others"},
		}),
		Annotations: readOnly("List tasks"),
	}, s.listTasks)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "my_tasks",
		Description: "The person's tasks across all projects: open ones by deadline (then priority), or closed ones, newest first.",
		InputSchema: schemaFor[myTasksInput](map[string][]string{"state": {overview.StateOpen, overview.StateClosed}}),
		Annotations: readOnly("My tasks"),
	}, s.myTasks)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_task",
		Description: "One task, with its Markdown description and the GitHub, GitLab or Trello items linked to it.",
		Annotations: readOnly("Get task"),
	}, s.getTask)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "create_task",
		Description: "Create a task in a project. It starts in backlog. If the project's integrations post new tasks automatically, it is posted there too.",
		InputSchema: schemaFor[createTaskInput](map[string][]string{"priority": task.Priorities}),
		Annotations: write("Create task", false),
	}, s.createTask)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "update_task",
		Description: "Change a task: name, description, status, priority, deadline, labels or assignee (me or none). Only what is sent changes. add_labels and remove_labels change some labels; labels replaces them all. A synced task also changes on GitHub, GitLab or Trello.",
		InputSchema: schemaFor[updateTaskInput](map[string][]string{
			"status": task.Statuses, "priority": task.Priorities, "assignee": {"me", "none"},
		}),
		Annotations: write("Update task", true),
	}, s.updateTask)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "claim_task",
		Description: "Take a task without assignee for the person, without clocking in. Fails if someone else already has it.",
		Annotations: write("Claim task", true),
	}, s.claimTask)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "sync_task",
		Description: "Sync the task now with the GitHub, GitLab or Trello items linked to it (both ways), like the Sync button of the task.",
		Annotations: write("Sync task", true),
	}, s.syncTask)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "sync_project",
		Description: "Sync the whole project now with its GitHub, GitLab or Trello integrations (both ways), like the Sync button of the task list: " +
			"open items that are not here yet become tasks, and the linked ones are updated. Closed items are not imported. " +
			"Only integrations with the sync on take part. It can take a while on a big repository.",
		Annotations: write("Sync project", true),
	}, s.syncProject)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "create_label",
		Description: "Create a label in a project, for those who manage its labels. If the project already has a label with that name " +
			"(case-insensitive), it comes back with created false.",
		Annotations: write("Create label", true),
	}, s.createLabel)
}

func (s *server) listLabels(_ context.Context, req *mcp.CallToolRequest, in projectInput) (*mcp.CallToolResult, LabelsOut, error) {
	id, err := me(req)
	if err != nil {
		return nil, LabelsOut{}, s.fail(err)
	}
	if err := s.reachProject(id, in.ProjectID); err != nil {
		return nil, LabelsOut{}, err
	}
	labels, err := s.Tasks.ListLabels(in.ProjectID)
	if err != nil {
		return nil, LabelsOut{}, s.fail(err)
	}
	out := LabelsOut{Labels: []string{}}
	for _, l := range labels {
		out.Labels = append(out.Labels, l.Name)
	}
	return nil, out, nil
}

func (s *server) listTasks(_ context.Context, req *mcp.CallToolRequest, in listTasksInput) (*mcp.CallToolResult, TasksOut, error) {
	id, err := me(req)
	if err != nil {
		return nil, TasksOut{}, s.fail(err)
	}
	if err := s.reachProject(id, in.ProjectID); err != nil {
		return nil, TasksOut{}, err
	}
	f := task.ListFilter{
		Query: strings.TrimSpace(in.Query), Priorities: in.Priorities, Statuses: in.Statuses,
		Page: max(in.Page, 1), PerPage: in.PerPage,
	}
	if len([]rune(f.Query)) > validate.MaxSearch {
		return nil, TasksOut{}, s.fail(task.ErrQueryTooLong.With("max", validate.MaxSearch, "field", "query"))
	}
	if f.PerPage <= 0 {
		f.PerPage = defaultPerPage
	}
	if len(f.Statuses) == 0 {
		f.Statuses = openStatuses
	}
	if err := assigneeFilter(&f, in.Assignee, id); err != nil {
		return nil, TasksOut{}, s.fail(err)
	}
	if len(in.Labels) > 0 {
		ids, err := s.labelIDs(in.ProjectID, in.Labels)
		if err != nil {
			return nil, TasksOut{}, err
		}
		for _, raw := range ids {
			f.LabelIDs = append(f.LabelIDs, uuid.MustParse(raw))
		}
	}
	page, err := s.Tasks.ListPage(in.ProjectID, f)
	if err != nil {
		return nil, TasksOut{}, s.fail(err)
	}
	loc := s.location(id)
	out := TasksOut{Items: []TaskOut{}, Total: page.Total, Page: page.Page, PerPage: page.PerPage}
	for i := range page.Items {
		out.Items = append(out.Items, s.taskOut(&page.Items[i], id, loc, false))
	}
	return nil, out, nil
}

func (s *server) myTasks(_ context.Context, req *mcp.CallToolRequest, in myTasksInput) (*mcp.CallToolResult, MyTasksOut, error) {
	id, err := me(req)
	if err != nil {
		return nil, MyTasksOut{}, s.fail(err)
	}
	state := in.State
	if state == "" {
		state = overview.StateOpen
	}
	if state != overview.StateOpen && state != overview.StateClosed {
		return nil, MyTasksOut{}, s.fail(overview.ErrInvalidTaskState)
	}
	perPage := in.PerPage
	if perPage <= 0 {
		perPage = defaultPerPage
	}
	page, err := s.Overview.MyTasks(id.OrganizationID.String(), id.PersonID.String(), state, !id.IsAdmin(), max(in.Page, 1), perPage)
	if err != nil {
		return nil, MyTasksOut{}, s.fail(err)
	}
	loc := s.location(id)
	out := MyTasksOut{Items: []MyTaskOut{}, Total: page.Total, Page: page.Page, PerPage: page.PerPage}
	for _, t := range page.Items {
		item := MyTaskOut{
			ID: t.ID.String(), Name: t.Name, Status: t.Status, Priority: t.Priority,
			Project: Ref{ID: t.Project.ID.String(), Name: t.Project.Name},
		}
		if t.Deadline != nil {
			item.Deadline = t.Deadline.In(loc).Format(time.DateOnly)
		}
		item.URL = s.taskURL(item.ID)
		out.Items = append(out.Items, item)
	}
	return nil, out, nil
}

func (s *server) getTask(_ context.Context, req *mcp.CallToolRequest, in taskInput) (*mcp.CallToolResult, TaskOut, error) {
	id, err := me(req)
	if err != nil {
		return nil, TaskOut{}, s.fail(err)
	}
	t, err := s.reachTask(id, in.TaskID)
	if err != nil {
		return nil, TaskOut{}, err
	}
	return nil, s.taskOut(t, id, s.location(id), true), nil
}

func (s *server) createTask(_ context.Context, req *mcp.CallToolRequest, in createTaskInput) (*mcp.CallToolResult, TaskOut, error) {
	id, err := me(req)
	if err != nil {
		return nil, TaskOut{}, s.fail(err)
	}
	if err := s.reachProject(id, in.ProjectID); err != nil {
		return nil, TaskOut{}, err
	}
	loc := s.location(id)
	var deadline *time.Time
	if in.Deadline != "" {
		if deadline, err = parseDeadline(in.Deadline, loc); err != nil {
			return nil, TaskOut{}, s.fail(err)
		}
	}
	attrs := task.Attrs{}
	if in.Priority != "" {
		attrs.Priority = &in.Priority
	}
	if len(in.Labels) > 0 {
		ids, err := s.labelIDs(in.ProjectID, in.Labels)
		if err != nil {
			return nil, TaskOut{}, err
		}
		attrs.LabelIDs = &ids
	}
	assignee := ""
	if in.AssignToMe {
		assignee = id.PersonID.String()
	}
	t, err := s.Tasks.CreateAs(id.PersonID.String(), in.ProjectID, in.Name, in.Description, assignee, deadline, attrs)
	if err != nil {
		return nil, TaskOut{}, s.fail(err)
	}
	return nil, s.taskOut(t, id, loc, true), nil
}

func (s *server) updateTask(_ context.Context, req *mcp.CallToolRequest, in updateTaskInput) (*mcp.CallToolResult, TaskOut, error) {
	id, err := me(req)
	if err != nil {
		return nil, TaskOut{}, s.fail(err)
	}
	current, err := s.reachTask(id, in.TaskID)
	if err != nil {
		return nil, TaskOut{}, err
	}
	loc := s.location(id)
	// O PATCH da tarefa exige o nome: sem um novo, vai o que ela já tem.
	name := in.Name
	if strings.TrimSpace(name) == "" {
		name = current.Name
	}
	var deadline validate.Optional[time.Time]
	switch {
	case in.ClearDeadline:
		deadline = validate.Optional[time.Time]{Set: true}
	case in.Deadline != "":
		t, err := parseDeadline(in.Deadline, loc)
		if err != nil {
			return nil, TaskOut{}, s.fail(err)
		}
		deadline = validate.Optional[time.Time]{Set: true, Value: t}
	}
	var assignee *string
	switch in.Assignee {
	case "":
	case "me":
		self := id.PersonID.String()
		assignee = &self
	case "none":
		none := ""
		assignee = &none
	default:
		return nil, TaskOut{}, s.fail(ErrInvalidAssignee)
	}
	attrs := task.Attrs{}
	if in.Status != "" {
		attrs.Status = &in.Status
	}
	if in.Priority != "" {
		attrs.Priority = &in.Priority
	}
	changing := len(in.AddLabels) > 0 || len(in.RemoveLabels) > 0
	switch {
	case in.Labels != nil && changing:
		return nil, TaskOut{}, s.fail(ErrLabelsConflict)
	case in.Labels != nil:
		ids, err := s.labelIDs(current.ProjectID.String(), *in.Labels)
		if err != nil {
			return nil, TaskOut{}, err
		}
		attrs.LabelIDs = &ids
	case changing:
		ids, err := s.changedLabels(current, in.AddLabels, in.RemoveLabels)
		if err != nil {
			return nil, TaskOut{}, err
		}
		attrs.LabelIDs = &ids
	}
	t, err := s.Tasks.PatchAs(id.PersonID.String(), current.ID.String(), name, in.Description, assignee, deadline, attrs)
	if err != nil {
		return nil, TaskOut{}, s.fail(err)
	}
	return nil, s.taskOut(t, id, loc, true), nil
}

func (s *server) claimTask(_ context.Context, req *mcp.CallToolRequest, in taskInput) (*mcp.CallToolResult, TaskOut, error) {
	id, err := me(req)
	if err != nil {
		return nil, TaskOut{}, s.fail(err)
	}
	current, err := s.reachTask(id, in.TaskID)
	if err != nil {
		return nil, TaskOut{}, err
	}
	t, err := s.Tasks.Claim(id.PersonID.String(), current.ID.String())
	if err != nil {
		return nil, TaskOut{}, s.fail(err)
	}
	return nil, s.taskOut(t, id, s.location(id), false), nil
}

func (s *server) syncTask(ctx context.Context, req *mcp.CallToolRequest, in taskInput) (*mcp.CallToolResult, SyncOut, error) {
	id, err := me(req)
	if err != nil {
		return nil, SyncOut{}, s.fail(err)
	}
	t, err := s.reachTask(id, in.TaskID)
	if err != nil {
		return nil, SyncOut{}, err
	}
	sum, err := s.Syncer.SyncTaskNow(ctx, t.ID, nil)
	if err != nil {
		return nil, SyncOut{}, s.fail(err)
	}
	after, err := s.Tasks.Get(t.ID.String())
	if err != nil {
		return nil, SyncOut{}, s.fail(err)
	}
	return nil, SyncOut{
		Created: sum.Created, Updated: sum.Updated, Closed: sum.Closed, Pushed: sum.Pushed,
		Unmapped: sum.Unmapped, Errors: sum.Errors, Partial: sum.Partial, Problem: sum.Problem,
		Task: s.taskOut(after, id, s.location(id), false),
	}, nil
}

// syncProject é o botão Sincronizar da lista de tarefas: uma rodada em cada integração do projeto com a sincronização
// ligada. Como na rota da API, basta alcançar o projeto.
func (s *server) syncProject(ctx context.Context, req *mcp.CallToolRequest, in projectInput) (*mcp.CallToolResult, ProjectSyncOut, error) {
	id, err := me(req)
	if err != nil {
		return nil, ProjectSyncOut{}, s.fail(err)
	}
	if err := s.reachProject(id, in.ProjectID); err != nil {
		return nil, ProjectSyncOut{}, err
	}
	projectID, err := uuid.Parse(in.ProjectID)
	if err != nil {
		return nil, ProjectSyncOut{}, s.fail(project.ErrNotFound)
	}
	sum, err := s.Syncer.SyncProject(ctx, projectID)
	if errors.Is(err, issuesync.ErrSyncOff) {
		return nil, ProjectSyncOut{}, s.fail(ErrProjectSyncOff)
	}
	if err != nil {
		return nil, ProjectSyncOut{}, s.fail(err)
	}
	return nil, ProjectSyncOut{
		Created: sum.Created, Updated: sum.Updated, Closed: sum.Closed, Pushed: sum.Pushed,
		Unmapped: sum.Unmapped, Errors: sum.Errors, Partial: sum.Partial,
	}, nil
}

// createLabel cria a etiqueta para quem tem o labels.manage no projeto, a mesma checagem da rota da API. Um nome que o
// projeto já tem (sem diferenciar maiúsculas) devolve a que existe, como a tela faz, em vez de label.name_taken.
func (s *server) createLabel(_ context.Context, req *mcp.CallToolRequest, in createLabelInput) (*mcp.CallToolResult, LabelOut, error) {
	id, err := me(req)
	if err != nil {
		return nil, LabelOut{}, s.fail(err)
	}
	if err := s.reachProject(id, in.ProjectID); err != nil {
		return nil, LabelOut{}, err
	}
	projectID, err := uuid.Parse(in.ProjectID)
	if err != nil {
		return nil, LabelOut{}, s.fail(project.ErrNotFound)
	}
	set, err := s.Resolver.ProjectSet(id, projectID)
	if err != nil {
		return nil, LabelOut{}, s.fail(err)
	}
	if !set.Has(permission.LabelsManage) {
		return nil, LabelOut{}, s.fail(auth.ErrPermissionRequired)
	}
	name := strings.TrimSpace(in.Name)
	labels, err := s.Tasks.ListLabels(projectID.String())
	if err != nil {
		return nil, LabelOut{}, s.fail(err)
	}
	for _, l := range labels {
		if strings.EqualFold(l.Name, name) {
			return nil, LabelOut{Name: l.Name}, nil
		}
	}
	l, err := s.Tasks.CreateLabel(projectID.String(), name)
	if err != nil {
		return nil, LabelOut{}, s.fail(err)
	}
	return nil, LabelOut{Name: l.Name, Created: true}, nil
}

// changedLabels parte das etiquetas da tarefa, põe as de add e tira as de remove, pelo nome. Tirar uma que a tarefa
// não tem não faz nada, e um nome nas duas listas sai.
func (s *server) changedLabels(t *task.Task, add, remove []string) ([]string, error) {
	projectID := t.ProjectID.String()
	var addIDs, removeIDs []string
	var err error
	if len(add) > 0 {
		if addIDs, err = s.labelIDs(projectID, add); err != nil {
			return nil, err
		}
	}
	if len(remove) > 0 {
		if removeIDs, err = s.labelIDs(projectID, remove); err != nil {
			return nil, err
		}
	}
	drop := make(map[string]bool, len(removeIDs))
	for _, lid := range removeIDs {
		drop[lid] = true
	}
	ids := make([]string, 0, len(t.Labels)+len(addIDs))
	for _, l := range t.Labels {
		ids = append(ids, l.ID.String())
	}
	ids = append(ids, addIDs...)
	out := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, lid := range ids {
		if !drop[lid] && !seen[lid] {
			seen[lid] = true
			out = append(out, lid)
		}
	}
	return out, nil
}

// labelIDs troca os nomes das etiquetas pelos ids delas no projeto, sem diferenciar maiúsculas. Um nome que o projeto
// não tem é mcp.unknown_label, com os que existem; o texto lembra que quem cuida das etiquetas a cria com create_label.
func (s *server) labelIDs(projectID string, names []string) ([]string, error) {
	labels, err := s.Tasks.ListLabels(projectID)
	if err != nil {
		return nil, s.fail(err)
	}
	byName := make(map[string]string, len(labels))
	available := make([]string, 0, len(labels))
	for _, l := range labels {
		byName[strings.ToLower(l.Name)] = l.ID.String()
		available = append(available, l.Name)
	}
	ids := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, n := range names {
		lid, ok := byName[strings.ToLower(strings.TrimSpace(n))]
		if !ok {
			return nil, s.fail(ErrUnknownLabel.With("label", n, "available", strings.Join(available, ", ")))
		}
		if !seen[lid] {
			seen[lid] = true
			ids = append(ids, lid)
		}
	}
	return ids, nil
}
