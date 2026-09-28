package task

import (
	"context"

	"github.com/google/uuid"

	"working-time-tracker/ent"
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
		SetAssigneeID(t.AssigneeID).
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

func (s *Store) ListByProject(projectID string) ([]Task, error) {
	uid, err := uuid.Parse(projectID)
	if err != nil {
		return nil, err
	}
	tasks, err := s.client.Task.Query().
		Where(task.ProjectIDEQ(uid)).
		WithAssignee().
		Order(ent.Desc(task.FieldCreatedAt)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	return toDomainTasks(tasks), nil
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
		SetAssigneeID(t.AssigneeID).
		SetDeadline(t.Deadline)
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
		ID:              e.ID,
		ProjectID:       e.ProjectID,
		Name:            e.Name,
		Description:     e.Description,
		AssigneeID:      e.AssigneeID,
		Deadline:        e.Deadline,
		ExternalItemID:  e.ExternalItemID,
		ExternalItemURL: e.ExternalItemURL,
		CreatedAt:       e.CreatedAt,
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
