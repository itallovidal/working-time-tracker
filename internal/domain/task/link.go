package task

import (
	"context"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	"working-time-tracker/ent/issuesync"
	"working-time-tracker/internal/database"
)

// O vínculo da tarefa com os itens externos mora em issue_syncs (ver ent/schema/issuesync.go): uma linha por
// item, com a tarefa e a integração. Quem liga à mão cria a linha "pending", e a sincronização a adota na
// primeira rodada em que o item está aberto.

// IntegrationInfo devolve o projeto dono da integração e o tipo dela, ou database.ErrNotFound.
func (s *Store) IntegrationInfo(integrationID uuid.UUID) (projectID uuid.UUID, kind string, err error) {
	it, err := s.client.Integration.Get(context.Background(), integrationID)
	if ent.IsNotFound(err) {
		return uuid.Nil, "", database.ErrNotFound
	}
	if err != nil {
		return uuid.Nil, "", err
	}
	return it.ProjectID, it.Type, nil
}

// LinkItem liga a tarefa ao item da integração. A tarefa tem no máximo um item por integração
// (ErrAlreadyLinked) e o item serve a uma tarefa só (ErrItemTaken). O item que já foi descartado (a tarefa
// que o tinha foi excluída ou desligada dele) é religado, sem o acordo antigo: a primeira rodada o adota
// como a uma ligação nova. Avisa o gancho de mudança, para a adoção não esperar a rodada de fundo.
func (s *Store) LinkItem(taskID, integrationID uuid.UUID, itemID, url string) error {
	ctx := context.Background()
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return err
	}
	if err := linkItem(ctx, tx, taskID, integrationID, itemID, url); err != nil {
		_ = tx.Rollback()
		if ent.IsConstraintError(err) {
			return ErrItemTaken
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.changed(taskID)
	return nil
}

func linkItem(ctx context.Context, tx *ent.Tx, taskID, integrationID uuid.UUID, itemID, url string) error {
	has, err := tx.IssueSync.Query().
		Where(issuesync.TaskIDEQ(taskID), issuesync.IntegrationIDEQ(integrationID)).Exist(ctx)
	if err != nil {
		return err
	}
	if has {
		return ErrAlreadyLinked
	}
	row, err := tx.IssueSync.Query().
		Where(issuesync.IntegrationIDEQ(integrationID), issuesync.ItemIDEQ(itemID)).
		ForUpdate().Only(ctx)
	switch {
	case ent.IsNotFound(err):
		return tx.IssueSync.Create().
			SetIntegrationID(integrationID).SetTaskID(taskID).SetItemID(itemID).SetURL(url).
			SetState(issuesync.StatePending).SetSyncedAt(time.Now()).
			Exec(ctx)
	case err != nil:
		return err
	case row.TaskID != nil:
		return ErrItemTaken
	}
	return tx.IssueSync.UpdateOneID(row.ID).
		SetTaskID(taskID).SetURL(url).SetState(issuesync.StatePending).SetSyncedAt(time.Now()).
		SetTitle("").SetBody("").ClearLabels().ClearAssigneeLogins().
		SetMappedLogin("").ClearMappedPersonID().ClearDeadline().SetStuckSig("").SetLastError("").
		Exec(ctx)
}

// UnlinkItem solta a tarefa do item da integração. O vínculo que a sincronização já adotou vira item
// descartado (o item não volta como tarefa nova); o que nunca foi adotado some, como se não tivesse existido.
func (s *Store) UnlinkItem(taskID, integrationID uuid.UUID) error {
	ctx := context.Background()
	if _, err := s.client.IssueSync.Delete().
		Where(issuesync.TaskIDEQ(taskID), issuesync.IntegrationIDEQ(integrationID), issuesync.StateEQ(issuesync.StatePending)).
		Exec(ctx); err != nil {
		return err
	}
	_, err := s.client.IssueSync.Update().
		Where(issuesync.TaskIDEQ(taskID), issuesync.IntegrationIDEQ(integrationID)).
		ClearTaskID().Save(ctx)
	return err
}
