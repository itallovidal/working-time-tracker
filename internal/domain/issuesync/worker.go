package issuesync

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/adapter"
)

// O que faz a sincronização andar sozinha: o gancho das tarefas (Notify), que junta as mudanças e as
// empurra pouco depois, e a rotina de fundo (Run), que olha as integrações de tempos em tempos. O
// GitHub não alcança este servidor, então não há webhook: quem traz as mudanças de lá é a rotina.

// flushBatch é o teto de tarefas que o gancho sincroniza de uma vez. Renomear uma etiqueta usada por mil
// tarefas avisa as mil; elas saem em lotes, com o intervalo de agregação entre um lote e outro.
const flushBatch = 50

type queue struct {
	mu  sync.Mutex
	ids map[uuid.UUID]struct{}
	// created são as tarefas criadas aqui que esperam ser postadas na plataforma que as recebe sozinha;
	// parked, as que não deu para postar ainda (a plataforma fora do ar, o limite de requisições), com o
	// número de tentativas. A rotina de fundo e o botão Sincronizar tentam de novo as de parked.
	created map[uuid.UUID]struct{}
	parked  map[uuid.UUID]int
	wake    chan struct{}
}

func newQueue() *queue {
	return &queue{
		ids: map[uuid.UUID]struct{}{}, created: map[uuid.UUID]struct{}{}, parked: map[uuid.UUID]int{},
		wake: make(chan struct{}, 1),
	}
}

func (q *queue) addCreated(id uuid.UUID) {
	q.mu.Lock()
	q.created[id] = struct{}{}
	q.mu.Unlock()
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// takeCreated tira até max tarefas novas da fila.
func (q *queue) takeCreated(max int) []uuid.UUID {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]uuid.UUID, 0, min(max, len(q.created)))
	for id := range q.created {
		if len(out) == max {
			break
		}
		out = append(out, id)
		delete(q.created, id)
	}
	return out
}

// maxPublishAttempts é quantas vezes uma tarefa nova que não saiu é tentada de novo antes de a
// sincronização desistir dela.
const maxPublishAttempts = 10

// park guarda a tarefa para a próxima rodada de tentativas e diz se ainda vale tentar.
func (q *queue) park(id uuid.UUID) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.parked[id]++
	if q.parked[id] > maxPublishAttempts {
		delete(q.parked, id)
		return false
	}
	return true
}

// release tira a tarefa das que esperam: foi postada, ou não há mais o que postar.
func (q *queue) release(id uuid.UUID) {
	q.mu.Lock()
	delete(q.parked, id)
	q.mu.Unlock()
}

func (q *queue) parkedIDs() []uuid.UUID {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]uuid.UUID, 0, len(q.parked))
	for id := range q.parked {
		out = append(out, id)
	}
	return out
}

// work diz quantas tarefas esperam o worker: as mudadas e as novas.
func (q *queue) work() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.ids) + len(q.created)
}

func (q *queue) add(id uuid.UUID) {
	q.mu.Lock()
	q.ids[id] = struct{}{}
	q.mu.Unlock()
	select {
	case q.wake <- struct{}{}:
	default: // já há um aviso esperando o worker
	}
}

// take tira até max tarefas da fila.
func (q *queue) take(max int) []uuid.UUID {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]uuid.UUID, 0, min(max, len(q.ids)))
	for id := range q.ids {
		if len(out) == max {
			break
		}
		out = append(out, id)
		delete(q.ids, id)
	}
	return out
}

func (q *queue) len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.ids)
}

// Notify avisa que a tarefa mudou aqui. Não bloqueia nem fala com o GitHub: só anota, e o worker (ou
// Flush) cuida do resto. É o que o gancho do task.Store chama.
func (s *Syncer) Notify(taskID uuid.UUID) { s.queue.add(taskID) }

// NotifyCreated avisa que a tarefa foi criada aqui. Como Notify, só anota: se há uma integração que posta as
// tarefas novas sozinha (o Trello), o worker a posta pouco depois, sem a pessoa esperar a plataforma.
func (s *Syncer) NotifyCreated(taskID uuid.UUID) { s.queue.addCreated(taskID) }

// Pending diz quantas tarefas esperam para ser sincronizadas pelo gancho.
func (s *Syncer) Pending() int { return s.queue.len() }

// Flush sincroniza o próximo lote de tarefas avisadas e devolve quantas issues tocou. As que não têm
// issue ligada (a maioria) saem da fila sem custo.
func (s *Syncer) Flush(ctx context.Context) int {
	touched := 0
	for _, id := range s.queue.takeCreated(flushBatch) {
		if ctx.Err() != nil {
			return touched
		}
		touched += s.autoPublish(ctx, id)
	}
	ids := s.queue.take(flushBatch)
	if len(ids) == 0 {
		return touched
	}
	rows, err := s.d.Rows.ByTasks(ids)
	if err != nil {
		s.cfg.Logger.Error("issue sync: finding the linked issues", "error", err)
		return touched
	}
	for _, row := range rows {
		if ctx.Err() != nil {
			return touched
		}
		if s.backedOff(row.IntegrationID) {
			continue
		}
		touched++
		if err := s.SyncTask(ctx, row); err != nil {
			s.afterRun(row.IntegrationID, err)
			if !errors.Is(err, ErrSyncOff) && !errors.Is(err, context.Canceled) {
				s.cfg.Logger.Warn("issue sync: pushing a task", "task", row.TaskID, "error", err)
			}
		}
	}
	return touched
}

// worker espera um aviso do gancho, deixa passar o intervalo de agregação e sincroniza em lotes até a
// fila esvaziar.
func (s *Syncer) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.queue.wake:
		}
		for s.queue.work() > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(s.cfg.Debounce):
			}
			s.Flush(ctx)
		}
	}
}

// backoff é a espera de uma integração que falhou: um token revogado ou o limite de requisições não se
// resolvem tentando de novo a cada rodada.
type backoff struct {
	until time.Time
	fails int
}

func (s *Syncer) backedOff(id uuid.UUID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.backoff[id]
	return b != nil && s.cfg.Now().Before(b.until)
}

// afterRun ajusta a espera da integração conforme o resultado: o limite de requisições espera até o
// GitHub liberar; os outros erros que paravam a rodada dobram a espera, até uma hora.
func (s *Syncer) afterRun(id uuid.UUID, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil || errors.Is(err, ErrSyncRunning) || errors.Is(err, ErrSyncOff) || errors.Is(err, context.Canceled) {
		delete(s.backoff, id)
		return
	}
	b := s.backoff[id]
	if b == nil {
		b = &backoff{}
		s.backoff[id] = b
	}
	b.fails++
	if until, limited := adapter.RateLimitedUntil(err); limited {
		b.until = until
		return
	}
	wait := time.Minute << min(b.fails-1, 6)
	b.until = s.cfg.Now().Add(min(wait, time.Hour))
}

// tick olha as integrações com a sincronização ligada, uma de cada vez. Faz a rodada completa na
// primeira vez, quando não há cursor, e de FullEvery em FullEvery; entre elas, a incremental.
func (s *Syncer) tick(ctx context.Context) {
	s.retryParked(ctx)
	list, err := s.d.Integrations.ListSyncing()
	if err != nil {
		s.cfg.Logger.Error("issue sync: listing the integrations", "error", err)
		return
	}
	for _, it := range list {
		if ctx.Err() != nil {
			return
		}
		if s.backedOff(it.ID) {
			continue
		}
		mode := Incremental
		s.mu.Lock()
		last := s.lastRun[it.ID]
		s.mu.Unlock()
		if it.SyncCursor == nil || s.cfg.Now().Sub(last) >= s.cfg.FullEvery {
			mode = Full
		}
		_, err := s.Sync(ctx, it.ID, mode)
		s.afterRun(it.ID, err)
		if err != nil && !errors.Is(err, ErrSyncRunning) && !errors.Is(err, context.Canceled) {
			s.cfg.Logger.Warn("issue sync: round failed", "integration", it.ID, "error", err)
		}
	}
}

// jitter espalha as rodadas: sem isso, vários servidores subidos juntos bateriam no GitHub no mesmo segundo.
func jitter(d time.Duration) time.Duration {
	return d + time.Duration(rand.Int64N(int64(d)/10+1))
}

// Run mantém a sincronização andando até o contexto acabar: o worker do gancho e, se Interval não é
// zero, a rotina que olha as integrações. Só retorna depois de os dois pararem, para o servidor poder
// fechar o banco em seguida.
func (s *Syncer) Run(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.worker(ctx)
	}()
	if s.cfg.Interval > 0 {
		// A primeira rodada não espera o intervalo inteiro: quem acabou de subir o servidor quer ver as issues.
		wait := min(s.cfg.Interval, 10*time.Second)
		for {
			select {
			case <-ctx.Done():
				wg.Wait()
				return
			case <-time.After(jitter(wait)):
			}
			s.tick(ctx)
			wait = s.cfg.Interval
		}
	}
	<-ctx.Done()
	wg.Wait()
}
