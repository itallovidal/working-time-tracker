package issuesync

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/integration"
)

// Postar sozinha a tarefa criada aqui: nas plataformas que declaram Caps.AutoPublish (o Trello), toda tarefa
// nova de um projeto com a sincronização ligada vira um item, a menos que a pessoa a tenha desmarcado na etapa
// Integrações (skip_publish na criação). Quem cria a tarefa não espera a plataforma: o gancho de criação só
// avisa, e o worker posta pouco depois, com Publish. É por integração: a tarefa que o modal já postou no GitHub
// também vira cartão, e a que já tem um item nesta integração não ganha outro.
//
// O que não sai por um motivo que passa (a plataforma fora do ar, o limite de requisições) fica guardado na
// memória e é tentado de novo pela rotina de fundo e pelo botão Sincronizar, até maxPublishAttempts vezes. Um
// reinício do servidor esquece o que ainda esperava: a tarefa fica sem o item, e Publish (a API) a posta.

// autoPublishTarget acha a integração em que a tarefa nova deve sair: a primeira do projeto, ativa e com a
// sincronização ligada, de um tipo que posta sozinho. Nil se não há.
func (s *Syncer) autoPublishTarget(projectID string) (*integration.Integration, error) {
	list, err := s.d.Integrations.ListByProject(projectID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		it := &list[i]
		if !it.Enabled || !it.SyncIssues {
			continue
		}
		impl, err := adapter.GetIntegration(it.Type)
		if err != nil {
			continue
		}
		if _, ok := impl.(adapter.IssueSyncer); ok && impl.Descriptor().Caps.AutoPublish {
			return it, nil
		}
	}
	return nil, nil
}

// autoPublish posta uma tarefa nova, se há onde. Devolve 1 quando falou com a plataforma, para o Flush contar.
func (s *Syncer) autoPublish(ctx context.Context, taskID uuid.UUID) int {
	t, err := s.d.Tasks.GetByID(taskID.String())
	if err != nil {
		if !errors.Is(err, database.ErrNotFound) {
			s.cfg.Logger.Error("issue sync: reading a new task", "task", taskID, "error", err)
		}
		s.queue.release(taskID)
		return 0
	}
	target, err := s.autoPublishTarget(t.ProjectID.String())
	if err != nil {
		s.cfg.Logger.Error("issue sync: finding where to post a new task", "task", taskID, "error", err)
		return 0
	}
	// Não há onde postar, ou a pessoa desmarcou esta integração na etapa Integrações (e não se cai para outra:
	// desmarcar o Trello não pode postar em um segundo quadro), ou a tarefa já tem um item nela (alguém a
	// ligou à mão): não há o que postar. Um item em outra integração (a issue que o modal postou) não impede.
	if target == nil || slices.Contains(s.queue.skipOf(taskID), target.ID) || t.LinkFor(target.ID.String()) != nil {
		s.queue.release(taskID)
		return 0
	}
	if s.backedOff(target.ID) {
		s.park(taskID)
		return 0
	}

	_, err = s.Publish(ctx, taskID, target.ID)
	switch {
	case err == nil:
		s.afterRun(target.ID, nil)
		s.queue.release(taskID)
	case errors.Is(err, ErrAlreadyLinked), errors.Is(err, ErrSyncOff), errors.Is(err, database.ErrNotFound), errors.Is(err, integration.ErrNotFound):
		s.queue.release(taskID)
	case errors.Is(err, context.Canceled):
		// O servidor está parando: a fila é da memória, e a tarefa se posta pela API.
	case adapter.StopsSync(err):
		// A plataforma recusou o token, caiu ou pediu para esperar: a integração espera, e a tarefa fica para depois.
		s.afterRun(target.ID, err)
		s.record(target.ID, nil, err, "")
		s.park(taskID)
	default:
		// O que a plataforma recusou por si (o quadro sem lista, o token que não escreve) o aviso da integração
		// mostra; tentar de novo não muda nada.
		s.cfg.Logger.Warn("issue sync: posting a new task", "task", taskID, "integration", target.ID, "error", err)
		s.record(target.ID, nil, err, "")
		s.queue.release(taskID)
	}
	return 1
}

// park guarda a tarefa para uma próxima tentativa; passado o limite de tentativas, desiste dela.
func (s *Syncer) park(taskID uuid.UUID) {
	if !s.queue.park(taskID) {
		s.cfg.Logger.Warn("issue sync: giving up on posting a new task", "task", taskID)
	}
}

// retryParked tenta de novo as tarefas novas que não saíram.
func (s *Syncer) retryParked(ctx context.Context) {
	for _, id := range s.queue.parkedIDs() {
		if ctx.Err() != nil {
			return
		}
		s.autoPublish(ctx, id)
	}
}
