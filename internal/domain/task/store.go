package task

import (
	"context"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	entlabel "working-time-tracker/ent/label"
	entperson "working-time-tracker/ent/person"
	"working-time-tracker/ent/task"
	"working-time-tracker/internal/database"
)

type Store struct {
	client *ent.Client
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
	if t.ExternalIntegrationID != nil {
		q = q.SetExternalIntegrationID(*t.ExternalIntegrationID)
	}
	if t.ExternalItemID != nil {
		q = q.SetExternalItemID(*t.ExternalItemID)
	}
	if t.ExternalItemURL != nil {
		q = q.SetExternalItemURL(*t.ExternalItemURL)
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
	q := s.client.Task.Query().Where(task.ProjectIDEQ(projectID))
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

// ListByProject lista as tarefas do projeto, da mais nova para a mais antiga.
// Com f.Page maior que zero, devolve só aquela página.
func (s *Store) ListByProject(projectID string, f ListFilter) ([]Task, error) {
	uid, err := uuid.Parse(projectID)
	if err != nil {
		return nil, err
	}
	// O id desempata tarefas criadas no mesmo instante, para a ordem não mudar
	// de uma página para a outra.
	// A integração vem junto para a lista saber de que plataforma é o item vinculado.
	q := s.filtered(uid, f).
		WithAssignee().
		WithExternalIntegration().
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
		WithExternalIntegration().
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
	if t.ExternalIntegrationID != nil {
		q = q.SetExternalIntegrationID(*t.ExternalIntegrationID)
	} else {
		q = q.ClearExternalIntegrationID()
	}
	// SetNillable* ignora nil, então limpar o vínculo exige Clear* explícito.
	if t.ExternalItemID != nil {
		q = q.SetExternalItemID(*t.ExternalItemID)
	} else {
		q = q.ClearExternalItemID()
	}
	if t.ExternalItemURL != nil {
		q = q.SetExternalItemURL(*t.ExternalItemURL)
	} else {
		q = q.ClearExternalItemURL()
	}
	_, err := q.Save(context.Background())
	return err
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
	return n > 0, err
}

// UpdateAttrs grava só a prioridade, o status e as etiquetas que vierem (nil mantém), sem
// reescrever o resto da tarefa: uma atualização rápida não desfaz uma edição ou um responsável
// que chegaram no meio.
func (s *Store) UpdateAttrs(id uuid.UUID, priority, status *string, labels *[]Label) error {
	q := s.client.Task.UpdateOneID(id)
	if priority != nil {
		q = q.SetPriority(priorityOf(*priority))
	}
	if status != nil {
		q = q.SetStatus(statusOf(*status))
	}
	if labels != nil {
		q = q.ClearLabels().AddLabelIDs(labelIDs(*labels)...)
	}
	_, err := q.Save(context.Background())
	if ent.IsNotFound(err) {
		return database.ErrNotFound
	}
	return err
}

// StartProgress põe a tarefa em progresso, seja qual for o status atual: bater o ponto
// nela é começar (ou retomar) o trabalho. Quem para ou pausa não a tira de lá.
func (s *Store) StartProgress(taskID uuid.UUID) error {
	_, err := s.client.Task.Update().
		Where(task.IDEQ(taskID), task.StatusNEQ(task.StatusInProgress)).
		SetStatus(task.StatusInProgress).
		Save(context.Background())
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
		ID:                    e.ID,
		ProjectID:             e.ProjectID,
		Name:                  e.Name,
		Description:           e.Description,
		Priority:              string(e.Priority),
		Status:                string(e.Status),
		Labels:                []Label{},
		AssigneeID:            e.AssigneeID,
		Deadline:              e.Deadline,
		ExternalIntegrationID: e.ExternalIntegrationID,
		ExternalItemID:        e.ExternalItemID,
		ExternalItemURL:       e.ExternalItemURL,
		CreatedAt:             e.CreatedAt,
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
	if e.Edges.ExternalIntegration != nil {
		t.ExternalIntegration = &Integration{
			ID:   e.Edges.ExternalIntegration.ID,
			Type: e.Edges.ExternalIntegration.Type,
		}
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
