package task

import (
	"context"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	"working-time-tracker/ent/issuesync"
	entlabel "working-time-tracker/ent/label"
	entperson "working-time-tracker/ent/person"
	entproject "working-time-tracker/ent/project"
	"working-time-tracker/ent/task"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/projectaccess"
)

type Store struct {
	client *ent.Client
	changeHook
}

func NewStore(client *ent.Client) *Store {
	return &Store{client: client}
}

func (s *Store) Create(t *Task) error {
	q := s.client.Task.Create().
		SetProjectID(t.ProjectID).
		SetName(t.Name).
		SetDescription(t.Description).
		SetPriority(priorityOf(t.Priority)).
		SetStatus(statusOf(t.Status)).
		AddLabelIDs(labelIDs(t.Labels)...).
		SetNillableAssigneeID(t.AssigneeID).
		SetDeadline(t.Deadline)
	// A tarefa importada nasce com a data de criação do item, e não com a de agora.
	if !t.CreatedAt.IsZero() {
		q = q.SetCreatedAt(t.CreatedAt)
	}
	created, err := q.Save(context.Background())
	if err != nil {
		return err
	}
	t.ID = created.ID
	t.CreatedAt = created.CreatedAt
	return nil
}

// noDeadline é o limite abaixo do qual um prazo conta como "sem prazo": uma
// tarefa antiga sem prazo, depois de editada, guarda o tempo zero do Go.
var noDeadline = time.Date(1971, 1, 1, 0, 0, 0, 0, time.UTC)

// filtered monta a consulta das tarefas do projeto que passam pelos filtros.
func (s *Store) filtered(projectID uuid.UUID, f ListFilter) *ent.TaskQuery {
	return applyFilter(s.client.Task.Query().Where(task.ProjectIDEQ(projectID)), f)
}

// applyFilter aplica os filtros a uma consulta de tarefas, seja qual for o escopo dela.
func applyFilter(q *ent.TaskQuery, f ListFilter) *ent.TaskQuery {
	if f.Query != "" {
		q = q.Where(task.NameContainsFold(f.Query))
	}
	if f.Unassigned {
		q = q.Where(task.AssigneeIDIsNil())
	} else if f.Assigned {
		q = q.Where(task.AssigneeIDNotNil())
	} else if f.OthersOf != nil {
		q = q.Where(task.AssigneeIDNotNil(), task.AssigneeIDNEQ(*f.OthersOf))
	} else if f.AssigneeID != nil {
		q = q.Where(task.AssigneeIDEQ(*f.AssigneeID))
	}
	if f.DeadlineTo != nil {
		q = q.Where(task.DeadlineLTE(*f.DeadlineTo), task.DeadlineGTE(noDeadline))
	}
	if len(f.Priorities) > 0 {
		ps := make([]task.Priority, len(f.Priorities))
		for i, p := range f.Priorities {
			ps[i] = priorityOf(p)
		}
		q = q.Where(task.PriorityIn(ps...))
	}
	if len(f.Statuses) > 0 {
		ss := make([]task.Status, len(f.Statuses))
		for i, st := range f.Statuses {
			ss[i] = statusOf(st)
		}
		q = q.Where(task.StatusIn(ss...))
	}
	if len(f.LabelIDs) > 0 {
		q = q.Where(task.HasLabelsWith(entlabel.IDIn(f.LabelIDs...)))
	}
	if f.InProjectsOf != nil {
		q = q.Where(task.HasProjectWith(projectaccess.ProjectsOf(*f.InProjectsOf)))
	}
	return q
}

// priorityOf converte a prioridade do domínio para a do Ent; vazio é sem prioridade.
func priorityOf(p string) task.Priority {
	if p == "" {
		return task.PriorityNone
	}
	return task.Priority(p)
}

// statusOf converte o status do domínio para o do Ent; vazio é backlog.
func statusOf(s string) task.Status {
	if s == "" {
		return task.StatusBacklog
	}
	return task.Status(s)
}

func labelIDs(ls []Label) []uuid.UUID {
	ids := make([]uuid.UUID, len(ls))
	for i, l := range ls {
		ids[i] = l.ID
	}
	return ids
}

// withLabels carrega as etiquetas da tarefa em ordem alfabética.
func withLabels(q *ent.LabelQuery) {
	q.Order(ent.Asc(entlabel.FieldName))
}

// withLinks carrega os vínculos da tarefa com a integração de cada um, do mais antigo para o mais novo.
func withLinks(q *ent.IssueSyncQuery) {
	q.WithIntegration().Order(ent.Asc(issuesync.FieldCreatedAt, issuesync.FieldID))
}

// ListByProject lista as tarefas do projeto, da mais nova para a mais antiga.
// Com f.Page maior que zero, devolve só aquela página.
func (s *Store) ListByProject(projectID string, f ListFilter) ([]Task, error) {
	uid, err := uuid.Parse(projectID)
	if err != nil {
		return nil, err
	}
	// O id desempata tarefas criadas no mesmo instante, para a ordem não mudar
	// de uma página para a outra.
	// Os vínculos vêm junto, com a integração, para a lista saber de que plataformas são os itens.
	q := s.filtered(uid, f).
		WithAssignee().
		WithIssueSyncs(withLinks).
		WithLabels(withLabels).
		Order(ent.Desc(task.FieldCreatedAt, task.FieldID))
	if f.Page > 0 {
		q = q.Limit(f.PerPage).Offset((f.Page - 1) * f.PerPage)
	}
	tasks, err := q.All(context.Background())
	if err != nil {
		return nil, err
	}
	return toDomainTasks(tasks), nil
}

// CountByProject conta as tarefas do projeto que passam pelos filtros.
func (s *Store) CountByProject(projectID string, f ListFilter) (int, error) {
	uid, err := uuid.Parse(projectID)
	if err != nil {
		return 0, err
	}
	return s.filtered(uid, f).Count(context.Background())
}

// assigned monta a consulta das tarefas de uma pessoa em todos os projetos da organização que passam
// pelos filtros. O responsável vem do argumento, não do filtro.
func (s *Store) assigned(orgID, personID uuid.UUID, f ListFilter) *ent.TaskQuery {
	q := s.client.Task.Query().Where(
		task.AssigneeIDEQ(personID),
		task.HasProjectWith(entproject.OrganizationIDEQ(orgID)),
	)
	return applyFilter(q, f)
}

// ListAssigned lista as tarefas da pessoa em todos os projetos da organização, da mais nova para a
// mais antiga, cada uma com o nome do projeto. Com f.Page maior que zero, devolve só aquela página.
func (s *Store) ListAssigned(orgID, personID string, f ListFilter) ([]Assigned, error) {
	oid, pid, err := parseOrgPerson(orgID, personID)
	if err != nil {
		return nil, err
	}
	q := s.assigned(oid, pid, f).
		WithProject().
		Order(ent.Desc(task.FieldCreatedAt, task.FieldID))
	if f.Page > 0 {
		q = q.Limit(f.PerPage).Offset((f.Page - 1) * f.PerPage)
	}
	tasks, err := q.All(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]Assigned, len(tasks))
	for i, e := range tasks {
		out[i] = Assigned{Task: *toDomainTask(e)}
		if e.Edges.Project != nil {
			out[i].ProjectName = e.Edges.Project.Name
		}
	}
	return out, nil
}

// CountAssigned conta as tarefas da pessoa na organização que passam pelos filtros.
func (s *Store) CountAssigned(orgID, personID string, f ListFilter) (int, error) {
	oid, pid, err := parseOrgPerson(orgID, personID)
	if err != nil {
		return 0, err
	}
	return s.assigned(oid, pid, f).Count(context.Background())
}

// AssignedByStatus conta as tarefas da pessoa na organização que passam pelos filtros (o que importa é
// f.InProjectsOf) por status, numa consulta só. Um status sem tarefas não aparece no mapa.
func (s *Store) AssignedByStatus(orgID, personID string, f ListFilter) (map[string]int, error) {
	oid, pid, err := parseOrgPerson(orgID, personID)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Status string `json:"status"`
		Count  int    `json:"count"`
	}
	if err := s.assigned(oid, pid, f).
		GroupBy(task.FieldStatus).
		Aggregate(ent.Count()).
		Scan(context.Background(), &rows); err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(rows))
	for _, r := range rows {
		counts[r.Status] = r.Count
	}
	return counts, nil
}

func parseOrgPerson(orgID, personID string) (org, person uuid.UUID, err error) {
	if org, err = uuid.Parse(orgID); err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	if person, err = uuid.Parse(personID); err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	return org, person, nil
}

// AssigneesByProject lista, sem repetir e por nome, quem é responsável por
// alguma tarefa do projeto, esteja ou não nos times.
func (s *Store) AssigneesByProject(projectID string) ([]Person, error) {
	uid, err := uuid.Parse(projectID)
	if err != nil {
		return nil, err
	}
	persons, err := s.client.Person.Query().
		Where(entperson.HasTasksWith(task.ProjectIDEQ(uid))).
		Order(ent.Asc(entperson.FieldName)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	result := make([]Person, len(persons))
	for i, p := range persons {
		result[i] = Person{ID: p.ID, Name: p.Name, Email: p.Email}
	}
	return result, nil
}

func (s *Store) GetByID(id string) (*Task, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	t, err := s.client.Task.Query().
		Where(task.IDEQ(uid)).
		WithAssignee().
		WithIssueSyncs(withLinks).
		WithLabels(withLabels).
		Only(context.Background())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, database.ErrNotFound
		}
		return nil, err
	}
	return toDomainTask(t), nil
}

func (s *Store) Update(t *Task) error {
	q := s.client.Task.UpdateOneID(t.ID).
		SetName(t.Name).
		SetDescription(t.Description).
		SetPriority(priorityOf(t.Priority)).
		SetStatus(statusOf(t.Status)).
		ClearLabels().
		AddLabelIDs(labelIDs(t.Labels)...).
		SetDeadline(t.Deadline)
	// Sem responsável, a coluna fica nula: Clear* é a única forma de desvincular.
	if t.AssigneeID != nil {
		q = q.SetAssigneeID(*t.AssigneeID)
	} else {
		q = q.ClearAssigneeID()
	}
	if _, err := q.Save(context.Background()); err != nil {
		return err
	}
	s.changed(t.ID)
	return nil
}

// ClaimIfUnassigned passa a tarefa para a pessoa só se ela ainda não tem
// responsável. O UPDATE condicional decide a disputa no banco: de dois pontos
// batidos juntos na mesma tarefa livre, só o primeiro a leva.
func (s *Store) ClaimIfUnassigned(taskID, personID uuid.UUID) error {
	_, err := s.TryClaim(taskID, personID)
	return err
}

// TryClaim é o ClaimIfUnassigned que diz se a pessoa levou a tarefa: false quando ela já
// tinha responsável (de dois pedidos juntos, só um recebe true).
func (s *Store) TryClaim(taskID, personID uuid.UUID) (bool, error) {
	n, err := s.client.Task.Update().
		Where(task.IDEQ(taskID), task.AssigneeIDIsNil()).
		SetAssigneeID(personID).
		Save(context.Background())
	if n > 0 {
		s.changed(taskID)
	}
	return n > 0, err
}

// attrsPatch é o que Store.UpdateAttrs grava: cada campo nil fica como está. O responsável tem três
// estados, então não basta o ponteiro: assignee muda para a pessoa e unassign o tira.
type attrsPatch struct {
	priority, status *string
	labels           *[]Label
	assignee         *uuid.UUID
	unassign         bool
	deadline         *time.Time
}

// UpdateAttrs grava só os campos do patch, sem reescrever o resto da tarefa: uma edição dos detalhes não
// desfaz o nome, a descrição ou um responsável que chegaram no meio.
func (s *Store) UpdateAttrs(id uuid.UUID, p attrsPatch) error {
	q := s.client.Task.UpdateOneID(id)
	if p.priority != nil {
		q = q.SetPriority(priorityOf(*p.priority))
	}
	if p.status != nil {
		q = q.SetStatus(statusOf(*p.status))
	}
	if p.labels != nil {
		q = q.ClearLabels().AddLabelIDs(labelIDs(*p.labels)...)
	}
	if p.assignee != nil {
		q = q.SetAssigneeID(*p.assignee)
	} else if p.unassign {
		q = q.ClearAssigneeID()
	}
	if p.deadline != nil {
		q = q.SetDeadline(*p.deadline)
	}
	_, err := q.Save(context.Background())
	if ent.IsNotFound(err) {
		return database.ErrNotFound
	}
	if err == nil {
		s.changed(id)
	}
	return err
}

// StartProgress põe a tarefa em progresso, seja qual for o status atual: bater o ponto
// nela é começar (ou retomar) o trabalho. Quem para ou pausa não a tira de lá.
func (s *Store) StartProgress(taskID uuid.UUID) error {
	n, err := s.client.Task.Update().
		Where(task.IDEQ(taskID), task.StatusNEQ(task.StatusInProgress)).
		SetStatus(task.StatusInProgress).
		Save(context.Background())
	if n > 0 {
		s.changed(taskID)
	}
	return err
}

// IntegrationProjectID devolve o projeto dono da integração, ou database.ErrNotFound.
func (s *Store) IntegrationProjectID(integrationID uuid.UUID) (uuid.UUID, error) {
	it, err := s.client.Integration.Get(context.Background(), integrationID)
	if ent.IsNotFound(err) {
		return uuid.Nil, database.ErrNotFound
	}
	if err != nil {
		return uuid.Nil, err
	}
	return it.ProjectID, nil
}

func (s *Store) Delete(id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	return s.client.Task.DeleteOneID(uid).Exec(context.Background())
}

func toDomainTask(e *ent.Task) *Task {
	if e == nil {
		return nil
	}
	t := &Task{
		ID:          e.ID,
		ProjectID:   e.ProjectID,
		Name:        e.Name,
		Description: e.Description,
		Priority:    string(e.Priority),
		Status:      string(e.Status),
		Labels:      []Label{},
		AssigneeID:  e.AssigneeID,
		Deadline:    e.Deadline,
		Links:       []Link{},
		CreatedAt:   e.CreatedAt,
	}
	if e.Edges.Assignee != nil {
		t.Assignee = &Person{
			ID:    e.Edges.Assignee.ID,
			Name:  e.Edges.Assignee.Name,
			Email: e.Edges.Assignee.Email,
		}
	}
	for _, l := range e.Edges.Labels {
		t.Labels = append(t.Labels, Label{ID: l.ID, Name: l.Name})
	}
	for _, row := range e.Edges.IssueSyncs {
		link := Link{IntegrationID: row.IntegrationID, ItemID: row.ItemID, URL: row.URL, LastError: row.LastError}
		if in := row.Edges.Integration; in != nil {
			link.Integration = &Integration{ID: in.ID, Type: in.Type, Name: in.DisplayName}
		}
		t.Links = append(t.Links, link)
	}
	return t
}

func toDomainTasks(es []*ent.Task) []Task {
	result := make([]Task, len(es))
	for i, e := range es {
		result[i] = *toDomainTask(e)
	}
	return result
}
