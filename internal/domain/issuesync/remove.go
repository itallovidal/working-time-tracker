package issuesync

import (
	"context"
	"errors"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/domain/task"
)

// Remove tira da plataforma os itens de uma tarefa que acabou de ser excluída: apaga a issue do GitHub (ou só a
// fecha, se a conta não é admin do repositório) e arquiva o cartão do Trello. A tarefa já saiu quando isto roda,
// e o vínculo ficou como item descartado, então a sincronização não o trata como tarefa nem o importa de novo;
// por isso uma falha aqui só vira o aviso do resultado, e cada item é tratado sem depender dos outros.
//
// Espera a vez de cada integração (uma rodada em andamento termina antes) e vale as mesmas regras de escrita da
// postagem: a integração ligada e com a sincronização ativa, e um token que escreve.
func (s *Syncer) Remove(ctx context.Context, items []task.RemoteItem) []task.RemoteResult {
	out := make([]task.RemoteResult, 0, len(items))
	for _, it := range items {
		res := task.RemoteResult{IntegrationID: it.IntegrationID, Provider: it.Provider}
		outcome, err := s.removeItem(ctx, it)
		if err != nil {
			s.cfg.Logger.Warn("issue sync: could not remove an item of a deleted task", "integration", it.IntegrationID, "item", it.ItemID, "error", err)
			res.Problem = problemOf(err)
		} else {
			res.Outcome = string(outcome)
			if outcome == adapter.RemoveClosed {
				res.Problem = ErrRemoveOnlyClosed
			}
		}
		out = append(out, res)
	}
	return out
}

// removeItem trata um item.
func (s *Syncer) removeItem(ctx context.Context, it task.RemoteItem) (adapter.RemoveOutcome, error) {
	lock := s.lock(it.IntegrationID)
	lock.Lock()
	defer lock.Unlock()

	r, err := s.newRun(ctx, it.IntegrationID, false)
	if err != nil {
		return "", err
	}
	if r.readOnly {
		return "", ErrRemoveReadOnly
	}
	id := it.ItemID
	if key, ok := r.k.Key(id); ok {
		id = key
	}

	if rm, ok := r.src.(adapter.ItemRemover); ok {
		return rm.RemoveItem(ctx, r.conn, id)
	}
	// Um tipo que não sabe apagar nem arquivar: o mais perto disso é fechar o item.
	closed := "closed"
	if _, err := r.src.UpdateIssue(ctx, r.conn, id, adapter.IssuePatch{State: &closed}); err != nil {
		if errors.Is(err, adapter.ErrIssueGone) {
			return adapter.RemoveGone, nil
		}
		return "", err
	}
	return adapter.RemoveClosed, nil
}

// problemOf é o erro como a resposta o mostra: o código e os parâmetros. O que não é um erro da API (uma falha do
// banco) vira o erro interno, sem texto técnico.
func problemOf(err error) *apperr.Error {
	var e *apperr.Error
	if errors.As(err, &e) {
		return e
	}
	return apperr.ErrInternal
}
