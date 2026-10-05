package work_session

import (
	"context"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	"working-time-tracker/ent/task"
	"working-time-tracker/ent/worksession"
	"working-time-tracker/internal/database"
)

type Store struct {
	client *ent.Client
}

func NewStore(client *ent.Client) *Store {
	return &Store{client: client}
}

func (s *Store) Create(session *WorkSession) error {
	q := s.client.WorkSession.Create().
		SetTaskID(session.TaskID).
		SetPersonID(session.PersonID).
		SetStartAt(session.StartAt).
		SetNillablePayRateCents(session.PayRateCents).
		SetNillableBillRateCents(session.BillRateCents)
	if session.EndAt != nil {
		q = q.SetEndAt(*session.EndAt)
	}
	created, err := q.Save(context.Background())
	if err != nil {
		return err
	}
	session.ID = created.ID
	session.CreatedAt = created.CreatedAt
	return nil
}

func (s *Store) GetActiveByPerson(personID string) (*WorkSession, error) {
	uid, err := uuid.Parse(personID)
	if err != nil {
		return nil, err
	}
	session, err := s.client.WorkSession.Query().
		Where(worksession.PersonIDEQ(uid), worksession.EndAtIsNil()).
		WithTask().
		Only(context.Background())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, database.ErrNotFound
		}
		return nil, err
	}
	return toDomainSession(session), nil
}

// PersonInProjectOrganization diz se a pessoa existe e é da organização do projeto.
func (s *Store) PersonInProjectOrganization(personID, projectID string) (bool, error) {
	puid, err := uuid.Parse(personID)
	if err != nil {
		return false, nil
	}
	prjid, err := uuid.Parse(projectID)
	if err != nil {
		return false, nil
	}
	ctx := context.Background()
	p, err := s.client.Person.Get(ctx, puid)
	if ent.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	prj, err := s.client.Project.Get(ctx, prjid)
	if ent.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return p.OrganizationID == prj.OrganizationID, nil
}

// Update grava o início e o fim. Os valores por hora não mudam depois do clock-in.
func (s *Store) Update(session *WorkSession) error {
	q := s.client.WorkSession.UpdateOneID(session.ID).
		SetStartAt(session.StartAt)
	if session.EndAt != nil {
		q = q.SetEndAt(*session.EndAt)
	} else {
		q = q.ClearEndAt()
	}
	_, err := q.Save(context.Background())
	return err
}

func (s *Store) ListByProject(projectID string, taskID, personID *string) ([]WorkSession, error) {
	prjid, err := uuid.Parse(projectID)
	if err != nil {
		return nil, err
	}

	q := s.client.WorkSession.Query().
		Where(worksession.HasTaskWith(task.ProjectIDEQ(prjid)))

	if taskID != nil && *taskID != "" {
		tuid, _ := uuid.Parse(*taskID)
		q = q.Where(worksession.TaskIDEQ(tuid))
	}
	if personID != nil && *personID != "" {
		puid, _ := uuid.Parse(*personID)
		q = q.Where(worksession.PersonIDEQ(puid))
	}

	sessions, err := q.
		WithTask().
		WithPerson().
		Order(ent.Desc(worksession.FieldStartAt)).
		All(context.Background())
	if err != nil {
		return nil, err
	}

	return toDomainSessions(sessions), nil
}

func toDomainSession(e *ent.WorkSession) *WorkSession {
	if e == nil {
		return nil
	}
	s := &WorkSession{
		ID:        e.ID,
		TaskID:    e.TaskID,
		PersonID:  e.PersonID,
		StartAt:   e.StartAt,
		EndAt:     e.EndAt,
		CreatedAt: e.CreatedAt,

		PayRateCents:  e.PayRateCents,
		BillRateCents: e.BillRateCents,
	}
	if e.Edges.Task != nil {
		s.Task = &Task{
			ID:        e.Edges.Task.ID,
			Name:      e.Edges.Task.Name,
			ProjectID: e.Edges.Task.ProjectID,
		}
	}
	if e.Edges.Person != nil {
		s.Person = &Person{
			ID:   e.Edges.Person.ID,
			Name: e.Edges.Person.Name,
		}
	}
	return s
}

func toDomainSessions(es []*ent.WorkSession) []WorkSession {
	result := make([]WorkSession, len(es))
	for i, e := range es {
		result[i] = *toDomainSession(e)
	}
	return result
}
