package issuesync

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	"working-time-tracker/ent/issuesync"
	"working-time-tracker/ent/project"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/task"
)

// Estados de uma issue no vínculo.
const (
	stateOpen   = "open"
	stateClosed = "closed"
	stateGone   = "gone"
	// statePending é o vínculo feito à mão (ou o item recém-criado por Publish) que a sincronização ainda não
	// adotou: não há acordo, e o snapshot está vazio.
	statePending = "pending"
)

// errLinkDropped diz que o vínculo deixou de ser da tarefa no meio da rodada (alguém a desligou do item): o
// que a rodada ia gravar nele não vale mais.
var errLinkDropped = errors.New("issue sync: the link was dropped")

// Row é o vínculo de uma issue (ou de um cartão) com uma tarefa e o snapshot do último acordo.
type Row struct {
	ID            uuid.UUID
	IntegrationID uuid.UUID
	// TaskID é nulo na issue descartada: a tarefa foi excluída aqui e a issue não volta.
	TaskID *uuid.UUID
	// ItemID é a chave do item na plataforma (adapter.Issue.ID): o número da issue, o link curto do cartão.
	ItemID string
	// State é o estado da issue na última rodada: aberta, fechada, sumida (apagada ou transferida) ou
	// pendente (ainda não adotada).
	State string
	// URL é o endereço do item na plataforma.
	URL string

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
		ItemID:        e.ItemID,
		State:         string(e.State),
		URL:           e.URL,
		Snapshot: Snapshot{
			Title:        e.Title,
			Body:         e.Body,
			Closed:       e.State == issuesync.StateClosed,
			Labels:       append([]string{}, e.Labels...),
			Logins:       append([]string{}, e.AssigneeLogins...),
			MappedLogin:  e.MappedLogin,
			MappedPerson: e.MappedPersonID,
			Deadline:     deref(e.Deadline),
		},
		StuckSig:  e.StuckSig,
		LastError: e.LastError,
		SyncedAt:  e.SyncedAt,
	}
}

// ByIntegration lista os vínculos da integração, pela chave do item.
func (s *Store) ByIntegration(integrationID uuid.UUID) (map[string]*Row, error) {
	rows, err := s.client.IssueSync.Query().
		Where(issuesync.IntegrationIDEQ(integrationID)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	out := make(map[string]*Row, len(rows))
	for _, e := range rows {
		out[e.ItemID] = toRow(e)
	}
	return out, nil
}

// ByID acha o vínculo, ou nil.
func (s *Store) ByID(id uuid.UUID) (*Row, error) {
	e, err := s.client.IssueSync.Get(context.Background(), id)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toRow(e), nil
}

// ByTaskIntegration acha o vínculo da tarefa com a integração, ou nil.
func (s *Store) ByTaskIntegration(taskID, integrationID uuid.UUID) (*Row, error) {
	e, err := s.client.IssueSync.Query().
		Where(issuesync.TaskIDEQ(taskID), issuesync.IntegrationIDEQ(integrationID)).
		Only(context.Background())
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toRow(e), nil
}

// CountByTask diz a quantos itens a tarefa está ligada.
func (s *Store) CountByTask(taskID uuid.UUID) (int, error) {
	return s.client.IssueSync.Query().Where(issuesync.TaskIDEQ(taskID)).Count(context.Background())
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

// Create grava um vínculo novo. O índice único (integração, item) barra duas rodadas criando o mesmo.
func (s *Store) Create(r *Row) error {
	created, err := s.client.IssueSync.Create().
		SetIntegrationID(r.IntegrationID).
		SetNillableTaskID(r.TaskID).
		SetItemID(r.ItemID).
		SetURL(r.URL).
		SetState(issuesync.State(r.State)).
		SetTitle(r.Title).
		SetBody(r.Body).
		SetLabels(r.Labels).
		SetAssigneeLogins(r.Logins).
		SetMappedLogin(r.MappedLogin).
		SetNillableMappedPersonID(r.MappedPerson).
		SetNillableDeadline(ptr(r.Deadline)).
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

// Save grava o snapshot e o estado do vínculo de uma tarefa, e só se ele ainda é dela: quem desligou a
// tarefa do item no meio da rodada não é desfeito (errLinkDropped).
func (s *Store) Save(r *Row) error {
	if r.TaskID == nil {
		return errLinkDropped
	}
	q := s.client.IssueSync.Update().
		Where(issuesync.IDEQ(r.ID), issuesync.TaskIDEQ(*r.TaskID)).
		SetState(issuesync.State(r.State)).
		SetURL(r.URL).
		SetTitle(r.Title).
		SetBody(r.Body).
		SetLabels(r.Labels).
		SetAssigneeLogins(r.Logins).
		SetMappedLogin(r.MappedLogin).
		SetStuckSig(r.StuckSig).
		SetLastError(r.LastError).
		SetSyncedAt(r.SyncedAt)
	if r.MappedPerson != nil {
		q = q.SetMappedPersonID(*r.MappedPerson)
	} else {
		q = q.ClearMappedPersonID()
	}
	if d := ptr(r.Deadline); d != nil {
		q = q.SetDeadline(*d)
	} else {
		q = q.ClearDeadline()
	}
	n, err := q.Save(context.Background())
	if err != nil {
		return err
	}
	if n == 0 {
		return errLinkDropped
	}
	return nil
}

// Release solta a tarefa do vínculo (o item vira descartado), se ele ainda é dela.
func (s *Store) Release(r *Row) error {
	if r.TaskID == nil {
		return nil
	}
	_, err := s.client.IssueSync.Update().
		Where(issuesync.IDEQ(r.ID), issuesync.TaskIDEQ(*r.TaskID)).
		ClearTaskID().Save(context.Background())
	return err
}

// CreateWithTask cria a tarefa de um item e o vínculo dela numa transação só: uma queda no meio não deixa uma
// tarefa sem vínculo, que viraria uma segunda tarefa do mesmo item na rodada seguinte.
func (s *Store) CreateWithTask(ctx context.Context, in task.Imported, r *Row) (*task.Task, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	created, err := task.NewStore(tx.Client()).CreateImported(in)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	r.TaskID = &created.ID
	if err := (&Store{client: tx.Client()}).Create(r); err != nil {
		_ = tx.Rollback()
		r.TaskID = nil
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		r.TaskID = nil
		return nil, err
	}
	// Relê com o vínculo, que a leitura de dentro da transação ainda não tinha.
	return task.NewStore(s.client).GetByID(created.ID.String())
}

// ptr guarda o prazo como nulo quando é o "sem prazo" (o tempo zero), e deref faz o caminho de volta.
func ptr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func deref(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
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
