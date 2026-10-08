package issuesync

import (
	"context"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	"working-time-tracker/ent/issuesync"
	"working-time-tracker/ent/project"
	"working-time-tracker/internal/database"
)

// Estados de uma issue no vínculo.
const (
	stateOpen   = "open"
	stateClosed = "closed"
	stateGone   = "gone"
)

// Row é o vínculo de uma issue com uma tarefa e o snapshot do último acordo.
type Row struct {
	ID            uuid.UUID
	IntegrationID uuid.UUID
	// TaskID é nulo na issue descartada: a tarefa foi excluída aqui e a issue não volta.
	TaskID *uuid.UUID
	Number int
	// State é o estado da issue na última rodada: aberta, fechada ou sumida (apagada ou transferida).
	State string

	Snapshot
	StuckSig  string
	LastError string
	SyncedAt  time.Time
}

// snapshotState traz o estado fechado do vínculo para dentro do snapshot.
func (r *Row) snapshot() Snapshot {
	s := r.Snapshot
	s.Closed = r.State == stateClosed
	return s
}

type Store struct {
	client *ent.Client
}

func NewStore(client *ent.Client) *Store { return &Store{client: client} }

func toRow(e *ent.IssueSync) *Row {
	return &Row{
		ID:            e.ID,
		IntegrationID: e.IntegrationID,
		TaskID:        e.TaskID,
		Number:        e.IssueNumber,
		State:         string(e.State),
		Snapshot: Snapshot{
			Title:        e.Title,
			Body:         e.Body,
			Closed:       e.State == issuesync.StateClosed,
			Labels:       append([]string{}, e.Labels...),
			Logins:       append([]string{}, e.AssigneeLogins...),
			MappedLogin:  e.MappedLogin,
			MappedPerson: e.MappedPersonID,
		},
		StuckSig:  e.StuckSig,
		LastError: e.LastError,
		SyncedAt:  e.SyncedAt,
	}
}

// ByIntegration lista os vínculos da integração, pelo número da issue.
func (s *Store) ByIntegration(integrationID uuid.UUID) (map[int]*Row, error) {
	rows, err := s.client.IssueSync.Query().
		Where(issuesync.IntegrationIDEQ(integrationID)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	out := make(map[int]*Row, len(rows))
	for _, e := range rows {
		out[e.IssueNumber] = toRow(e)
	}
	return out, nil
}

// ByTask acha o vínculo da tarefa, ou nil.
func (s *Store) ByTask(taskID uuid.UUID) (*Row, error) {
	e, err := s.client.IssueSync.Query().Where(issuesync.TaskIDEQ(taskID)).Only(context.Background())
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toRow(e), nil
}

// ByTasks acha os vínculos destas tarefas.
func (s *Store) ByTasks(taskIDs []uuid.UUID) ([]*Row, error) {
	if len(taskIDs) == 0 {
		return nil, nil
	}
	rows, err := s.client.IssueSync.Query().Where(issuesync.TaskIDIn(taskIDs...)).All(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]*Row, len(rows))
	for i, e := range rows {
		out[i] = toRow(e)
	}
	return out, nil
}

// Create grava um vínculo novo. O índice único (integração, número) barra duas rodadas criando o mesmo.
func (s *Store) Create(r *Row) error {
	created, err := s.client.IssueSync.Create().
		SetIntegrationID(r.IntegrationID).
		SetNillableTaskID(r.TaskID).
		SetIssueNumber(r.Number).
		SetState(issuesync.State(r.State)).
		SetTitle(r.Title).
		SetBody(r.Body).
		SetLabels(r.Labels).
		SetAssigneeLogins(r.Logins).
		SetMappedLogin(r.MappedLogin).
		SetNillableMappedPersonID(r.MappedPerson).
		SetStuckSig(r.StuckSig).
		SetLastError(r.LastError).
		SetSyncedAt(r.SyncedAt).
		Save(context.Background())
	if err != nil {
		return err
	}
	r.ID = created.ID
	return nil
}

// Save grava o vínculo como está, tarefa e snapshot.
func (s *Store) Save(r *Row) error {
	q := s.client.IssueSync.UpdateOneID(r.ID).
		SetState(issuesync.State(r.State)).
		SetTitle(r.Title).
		SetBody(r.Body).
		SetLabels(r.Labels).
		SetAssigneeLogins(r.Logins).
		SetMappedLogin(r.MappedLogin).
		SetStuckSig(r.StuckSig).
		SetLastError(r.LastError).
		SetSyncedAt(r.SyncedAt)
	if r.TaskID != nil {
		q = q.SetTaskID(*r.TaskID)
	} else {
		q = q.ClearTaskID()
	}
	if r.MappedPerson != nil {
		q = q.SetMappedPersonID(*r.MappedPerson)
	} else {
		q = q.ClearMappedPersonID()
	}
	_, err := q.Save(context.Background())
	return err
}

// OrganizationOf devolve a organização do projeto: o e-mail de quem é responsável no GitHub só vale
// para as pessoas dela.
func (s *Store) OrganizationOf(projectID uuid.UUID) (uuid.UUID, error) {
	p, err := s.client.Project.Query().Where(project.IDEQ(projectID)).Only(context.Background())
	if ent.IsNotFound(err) {
		return uuid.Nil, database.ErrNotFound
	}
	if err != nil {
		return uuid.Nil, err
	}
	return p.OrganizationID, nil
}
