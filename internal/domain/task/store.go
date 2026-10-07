package task

import (
	"context"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/ent"
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
	} else if f.AssigneeID != nil {
		q = q.Where(task.AssigneeIDEQ(*f.AssigneeID))
	}
	if f.DeadlineTo != nil {
		q = q.Where(task.DeadlineLTE(*f.DeadlineTo), task.DeadlineGTE(noDeadline))
	}
	return q
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
	_, err := s.client.Task.Update().
		Where(task.IDEQ(taskID), task.AssigneeIDIsNil()).
		SetAssigneeID(personID).
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
