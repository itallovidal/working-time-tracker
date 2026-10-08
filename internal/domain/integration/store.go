package integration

import (
	"context"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	"working-time-tracker/ent/integration"
	"working-time-tracker/ent/issuesync"
	"working-time-tracker/internal/database"
)

type Store struct {
	client *ent.Client
}

func NewStore(client *ent.Client) *Store {
	return &Store{client: client}
}

func (s *Store) Create(it *Integration) error {
	created, err := s.client.Integration.Create().
		SetProjectID(it.ProjectID).
		SetType(it.Type).
		SetDisplayName(it.DisplayName).
		SetCredentials(it.Credentials).
		SetMetadata(it.Metadata).
		SetEnabled(it.Enabled).
		Save(context.Background())
	if err != nil {
		return err
	}
	it.ID = created.ID
	it.CreatedAt = created.CreatedAt
	return nil
}

func (s *Store) ListByProject(projectID string) ([]Integration, error) {
	uid, err := uuid.Parse(projectID)
	if err != nil {
		return nil, err
	}
	integrations, err := s.client.Integration.Query().
		Where(integration.ProjectIDEQ(uid)).
		Order(ent.Desc(integration.FieldCreatedAt)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	return toDomainIntegrations(integrations), nil
}

func (s *Store) GetByID(id string) (*Integration, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	it, err := s.client.Integration.Get(context.Background(), uid)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, database.ErrNotFound
		}
		return nil, err
	}
	return toDomainIntegration(it), nil
}

func (s *Store) Update(it *Integration) error {
	_, err := s.client.Integration.UpdateOneID(it.ID).
		SetDisplayName(it.DisplayName).
		SetCredentials(it.Credentials).
		SetMetadata(it.Metadata).
		SetEnabled(it.Enabled).
		SetSyncIssues(it.SyncIssues).
		Save(context.Background())
	return err
}

// ListSyncing lista as integrações ativas com a sincronização das issues ligada, de todos os projetos.
func (s *Store) ListSyncing() ([]Integration, error) {
	rows, err := s.client.Integration.Query().
		Where(integration.SyncIssues(true), integration.Enabled(true)).
		Order(ent.Asc(integration.FieldCreatedAt)).
		All(context.Background())
	if err != nil {
		return nil, err
	}
	return toDomainIntegrations(rows), nil
}

// SyncResult é o que uma rodada de sincronização deixa na integração.
type SyncResult struct {
	At time.Time
	// Cursor é de onde a próxima rodada incremental continua; nulo mantém o que há (a rodada que parou
	// no meio não o avança).
	Cursor *time.Time
	// Err é o código do erro que parou ou marcou a rodada; vazio é uma rodada sem erro.
	Err string
}

// SetSyncResult grava o resultado de uma rodada. Fica separado do Update de propósito: uma edição da
// integração não pode desfazer o cursor que uma rodada acabou de gravar.
func (s *Store) SetSyncResult(id uuid.UUID, r SyncResult) error {
	q := s.client.Integration.UpdateOneID(id).SetLastSyncedAt(r.At).SetLastSyncError(r.Err)
	if r.Cursor != nil {
		q = q.SetSyncCursor(*r.Cursor)
	}
	_, err := q.Save(context.Background())
	if ent.IsNotFound(err) {
		return database.ErrNotFound
	}
	return err
}

// ResetSync esquece a sincronização: o cursor, o último resultado e o vínculo de cada issue. As
// tarefas ficam como estão. Serve a quem troca o repositório, que faz as issues ligadas serem de outro lugar.
func (s *Store) ResetSync(id uuid.UUID) error {
	ctx := context.Background()
	if _, err := s.client.IssueSync.Delete().Where(issuesync.IntegrationIDEQ(id)).Exec(ctx); err != nil {
		return err
	}
	_, err := s.client.Integration.UpdateOneID(id).ClearSyncCursor().ClearLastSyncedAt().SetLastSyncError("").Save(ctx)
	return err
}

// Unmatched conta as issues abertas e ligadas a uma tarefa que têm responsável no GitHub sem
// ninguém daqui que corresponda a ele (sem e-mail público, ou o e-mail não é de ninguém do projeto).
func (s *Store) Unmatched(id uuid.UUID) (int, error) {
	rows, err := s.client.IssueSync.Query().
		Where(issuesync.IntegrationIDEQ(id), issuesync.StateEQ(issuesync.StateOpen),
			issuesync.TaskIDNotNil(), issuesync.MappedLoginEQ("")).
		Select(issuesync.FieldAssigneeLogins).
		All(context.Background())
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		if len(r.AssigneeLogins) > 0 {
			n++
		}
	}
	return n, nil
}

func (s *Store) Delete(id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	return s.client.Integration.DeleteOneID(uid).Exec(context.Background())
}

func toDomainIntegration(e *ent.Integration) *Integration {
	if e == nil {
		return nil
	}
	// Uma linha de antes do metadata tem a coluna nula: a resposta leva um objeto vazio.
	metadata := e.Metadata
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	return &Integration{
		ID:          e.ID,
		ProjectID:   e.ProjectID,
		Type:        e.Type,
		DisplayName: e.DisplayName,
		Credentials: e.Credentials,
		Metadata:    metadata,
		Enabled:     e.Enabled,
		SyncIssues:  e.SyncIssues,
		SyncCursor:  e.SyncCursor,

		LastSyncedAt:  e.LastSyncedAt,
		LastSyncError: e.LastSyncError,
		CreatedAt:     e.CreatedAt,
	}
}

func toDomainIntegrations(es []*ent.Integration) []Integration {
	result := make([]Integration, len(es))
	for i, e := range es {
		result[i] = *toDomainIntegration(e)
	}
	return result
}
