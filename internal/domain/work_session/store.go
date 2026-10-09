package work_session

import (
	"context"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	entproject "working-time-tracker/ent/project"
	"working-time-tracker/ent/worksession"
	"working-time-tracker/ent/worksessiontask"
	"working-time-tracker/internal/database"
)

type Store struct {
	client *ent.Client
}

func NewStore(client *ent.Client) *Store {
	return &Store{client: client}
}

// withTasks carrega as tarefas da sessão, na ordem em que entraram, cada uma com o nome.
func withTasks(q *ent.WorkSessionTaskQuery) {
	q.WithTask().Order(ent.Asc(worksessiontask.FieldFromAt), ent.Asc(worksessiontask.FieldCreatedAt))
}

// Create grava a sessão e a primeira tarefa dela, que entra no início, de uma vez só: a
// sessão nunca existe sem tarefa.
func (s *Store) Create(session *WorkSession, taskID uuid.UUID) error {
	ctx := context.Background()
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return err
	}
	q := tx.WorkSession.Create().
		SetProjectID(session.ProjectID).
		SetPersonID(session.PersonID).
		SetStartAt(session.StartAt).
		SetNillablePayRateCents(session.PayRateCents).
		SetNillableBillRateCents(session.BillRateCents).
		SetOwnerHours(session.OwnerHours)
	if session.EndAt != nil {
		q = q.SetEndAt(*session.EndAt)
	}
	created, err := q.Save(ctx)
	if err != nil {
		return rollback(tx, err)
	}
	link, err := tx.WorkSessionTask.Create().
		SetSessionID(created.ID).
		SetTaskID(taskID).
		SetFromAt(session.StartAt).
		SetNillableUntilAt(session.EndAt).
		Save(ctx)
	if err != nil {
		return rollback(tx, err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	session.ID = created.ID
	session.CreatedAt = created.CreatedAt
	session.Tasks = []SessionTask{{ID: link.ID, TaskID: taskID, FromAt: link.FromAt, UntilAt: link.UntilAt}}
	return nil
}

// rollback desfaz a transação e devolve o erro que a motivou.
func rollback(tx *ent.Tx, err error) error {
	if rerr := tx.Rollback(); rerr != nil {
		return rerr
	}
	return err
}

func (s *Store) GetActiveByPerson(personID string) (*WorkSession, error) {
	uid, err := uuid.Parse(personID)
	if err != nil {
		return nil, err
	}
	session, err := s.client.WorkSession.Query().
		Where(worksession.PersonIDEQ(uid), worksession.EndAtIsNil()).
		WithTaskLinks(withTasks).
		Only(context.Background())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, database.ErrNotFound
		}
		return nil, err
	}
	return toDomainSession(session), nil
}

// GetByID devolve a sessão com as tarefas e a pessoa, ou database.ErrNotFound.
func (s *Store) GetByID(id string) (*WorkSession, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, database.ErrNotFound
	}
	session, err := s.client.WorkSession.Query().
		Where(worksession.IDEQ(uid)).
		WithTaskLinks(withTasks).
		WithPerson().
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

// ListByProject devolve as sessões do projeto, da mais recente para a mais antiga, cada uma
// com todas as suas tarefas. Com taskID, só as sessões em que a tarefa esteve.
func (s *Store) ListByProject(projectID string, taskID, personID *string) ([]WorkSession, error) {
	prjid, err := uuid.Parse(projectID)
	if err != nil {
		return nil, err
	}

	q := s.client.WorkSession.Query().
		Where(worksession.ProjectIDEQ(prjid))

	if taskID != nil && *taskID != "" {
		tuid, _ := uuid.Parse(*taskID)
		q = q.Where(worksession.HasTaskLinksWith(worksessiontask.TaskIDEQ(tuid)))
	}
	if personID != nil && *personID != "" {
		puid, _ := uuid.Parse(*personID)
		q = q.Where(worksession.PersonIDEQ(puid))
	}

	sessions, err := q.
		WithTaskLinks(withTasks).
		WithPerson().
		Order(ent.Desc(worksession.FieldStartAt)).
		All(context.Background())
	if err != nil {
		return nil, err
	}

	return toDomainSessions(sessions), nil
}

// ListByOrganization devolve as sessões de todos os projetos da organização, da mais recente
// para a mais antiga, sem as tarefas nem a pessoa: serve a quem soma o tempo e os valores de
// tudo e não precisa do que cada sessão trabalhou.
func (s *Store) ListByOrganization(orgID string) ([]WorkSession, error) {
	uid, err := uuid.Parse(orgID)
	if err != nil {
		return nil, err
	}
	sessions, err := s.client.WorkSession.Query().
		Where(worksession.HasProjectWith(entproject.OrganizationIDEQ(uid))).
		Order(ent.Desc(worksession.FieldStartAt)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	return toDomainSessions(sessions), nil
}

// ListByOrganizationSince devolve as sessões dos projetos da organização que ainda corriam em since ou depois (a
// aberta sempre entra), da mais recente para a mais antiga, sem as tarefas: serve a quem soma o tempo de todos a
// partir de um instante sem ler a história inteira.
func (s *Store) ListByOrganizationSince(orgID string, since time.Time) ([]WorkSession, error) {
	uid, err := uuid.Parse(orgID)
	if err != nil {
		return nil, err
	}
	sessions, err := s.client.WorkSession.Query().
		Where(
			worksession.HasProjectWith(entproject.OrganizationIDEQ(uid)),
			worksession.Or(worksession.EndAtIsNil(), worksession.EndAtGTE(since)),
		).
		Order(ent.Desc(worksession.FieldStartAt)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	return toDomainSessions(sessions), nil
}

// ListByPersonSince devolve as sessões da pessoa nos projetos da organização que ainda corriam em
// since ou depois (as que terminaram antes ficam de fora; a aberta sempre entra), da mais recente
// para a mais antiga, sem as tarefas: serve a quem soma o tempo da pessoa a partir de um instante.
func (s *Store) ListByPersonSince(orgID, personID string, since time.Time) ([]WorkSession, error) {
	oid, err := uuid.Parse(orgID)
	if err != nil {
		return nil, err
	}
	pid, err := uuid.Parse(personID)
	if err != nil {
		return nil, err
	}
	sessions, err := s.client.WorkSession.Query().
		Where(
			worksession.PersonIDEQ(pid),
			worksession.HasProjectWith(entproject.OrganizationIDEQ(oid)),
			worksession.Or(worksession.EndAtIsNil(), worksession.EndAtGTE(since)),
		).
		Order(ent.Desc(worksession.FieldStartAt)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	return toDomainSessions(sessions), nil
}

// ListOpenByOrganization devolve as sessões abertas dos projetos da organização, da que
// começou primeiro para a última, cada uma com as suas tarefas (os intervalos, na ordem em
// que entraram, com o nome). Serve a quem mostra quem está trabalhando agora e em quê.
func (s *Store) ListOpenByOrganization(orgID string) ([]WorkSession, error) {
	uid, err := uuid.Parse(orgID)
	if err != nil {
		return nil, err
	}
	sessions, err := s.client.WorkSession.Query().
		Where(worksession.EndAtIsNil(), worksession.HasProjectWith(entproject.OrganizationIDEQ(uid))).
		WithTaskLinks(withTasks).
		Order(ent.Asc(worksession.FieldStartAt)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	return toDomainSessions(sessions), nil
}

// AddTask põe a tarefa na sessão pelo intervalo dado e devolve o intervalo criado.
func (s *Store) AddTask(sessionID, taskID uuid.UUID, from time.Time, until *time.Time) (uuid.UUID, error) {
	link, err := s.client.WorkSessionTask.Create().
		SetSessionID(sessionID).
		SetTaskID(taskID).
		SetFromAt(from).
		SetNillableUntilAt(until).
		Save(context.Background())
	if err != nil {
		return uuid.Nil, err
	}
	return link.ID, nil
}

// UpdateTask troca o intervalo de uma tarefa da sessão; until nulo é "até o fim da sessão".
func (s *Store) UpdateTask(linkID uuid.UUID, from time.Time, until *time.Time) error {
	q := s.client.WorkSessionTask.UpdateOneID(linkID).SetFromAt(from)
	if until != nil {
		q = q.SetUntilAt(*until)
	} else {
		q = q.ClearUntilAt()
	}
	return q.Exec(context.Background())
}

// RemoveTask tira um intervalo da sessão. A sessão e as horas ficam.
func (s *Store) RemoveTask(linkID uuid.UUID) error {
	return s.client.WorkSessionTask.DeleteOneID(linkID).Exec(context.Background())
}

func toDomainSession(e *ent.WorkSession) *WorkSession {
	if e == nil {
		return nil
	}
	s := &WorkSession{
		ID:        e.ID,
		ProjectID: e.ProjectID,
		PersonID:  e.PersonID,
		StartAt:   e.StartAt,
		EndAt:     e.EndAt,
		CreatedAt: e.CreatedAt,

		OwnerHours:    e.OwnerHours,
		PayRateCents:  e.PayRateCents,
		BillRateCents: e.BillRateCents,

		Tasks: make([]SessionTask, 0, len(e.Edges.TaskLinks)),
	}
	for _, l := range e.Edges.TaskLinks {
		link := SessionTask{ID: l.ID, TaskID: l.TaskID, FromAt: l.FromAt, UntilAt: l.UntilAt}
		if l.Edges.Task != nil {
			link.Task = &Task{ID: l.Edges.Task.ID, Name: l.Edges.Task.Name, ProjectID: l.Edges.Task.ProjectID}
		}
		s.Tasks = append(s.Tasks, link)
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
