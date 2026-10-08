package issuesync

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/domain/integration"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/team"
)

// Mode diz quanto da integração uma rodada olha.
type Mode int

const (
	// Incremental pede só as issues mexidas desde o cursor (abertas e fechadas): é a rodada de fundo.
	Incremental Mode = iota
	// Full olha todas as issues abertas e confere as que já eram ligadas e sumiram da lista: é a do
	// botão Sincronizar agora e a de hora em hora.
	Full
)

// Summary é o que uma rodada fez, para o botão responder.
type Summary struct {
	Created  int `json:"created"`  // tarefas novas, de issues abertas que não tinham tarefa
	Updated  int `json:"updated"`  // tarefas mudadas por algo que mudou no GitHub
	Closed   int `json:"closed"`   // das atualizadas, as que fecharam porque a issue fechou
	Pushed   int `json:"pushed"`   // issues mudadas por algo que mudou aqui
	Unmapped int `json:"unmapped"` // issues com responsável no GitHub que não se liga a ninguém daqui
	Errors   int `json:"errors"`   // issues que deram erro e ficam para a próxima rodada
	// Partial: a rodada parou antes de olhar tudo (limite de requisições, falha de rede). O cursor não
	// anda, e o que ficou por olhar é visto na próxima.
	Partial bool `json:"partial"`
}

// Config são os ajustes da sincronização. O valor zero serve: cada campo tem um padrão.
type Config struct {
	// Interval é de quanto em quanto tempo a rotina de fundo olha as integrações; zero a desliga.
	Interval time.Duration
	// Intervals tem, por tipo de integração ("github", "trello"), o intervalo próprio: o tipo que está aí
	// usa ele (zero desliga a rotina só para esse tipo), e o que não está usa Interval.
	Intervals map[string]time.Duration
	// Debounce é quanto o gancho espera, depois da primeira mudança, para juntar as que vierem.
	Debounce time.Duration
	// FullEvery é de quanto em quanto tempo a rotina de fundo faz uma rodada completa.
	FullEvery time.Duration
	// MaxPushes é o teto de issues mudadas numa rodada; o resto fica para a seguinte.
	MaxPushes int
	Now       func() time.Time
	Logger    *slog.Logger
}

// intervalFor é o intervalo da rotina de fundo para um tipo de integração.
func (c Config) intervalFor(integrationType string) time.Duration {
	if d, ok := c.Intervals[integrationType]; ok {
		return d
	}
	return c.Interval
}

// tickEvery é de quanto em quanto tempo a rotina acorda: o menor dos intervalos ligados. Zero é a rotina
// desligada para todos os tipos.
func (c Config) tickEvery() time.Duration {
	var least time.Duration
	for _, d := range append([]time.Duration{c.Interval}, mapValues(c.Intervals)...) {
		if d > 0 && (least == 0 || d < least) {
			least = d
		}
	}
	return least
}

func mapValues(m map[string]time.Duration) []time.Duration {
	out := make([]time.Duration, 0, len(m))
	for _, d := range m {
		out = append(out, d)
	}
	return out
}

func (c Config) withDefaults() Config {
	if c.Debounce <= 0 {
		c.Debounce = 1500 * time.Millisecond
	}
	if c.FullEvery <= 0 {
		c.FullEvery = time.Hour
	}
	if c.MaxPushes <= 0 {
		c.MaxPushes = 100
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	return c
}

// Deps são o que a sincronização usa dos outros domínios.
type Deps struct {
	Integrations *integration.Service
	Tasks        *task.Store
	People       *person.Store
	Members      *team.MembershipStore
	Rows         *Store
}

// Syncer sincroniza as issues das integrações com as tarefas.
type Syncer struct {
	d     Deps
	cfg   Config
	cache *cache

	mu    sync.Mutex
	locks map[uuid.UUID]*sync.Mutex

	queue    *queue
	backoff  map[uuid.UUID]*backoff
	lastRun  map[uuid.UUID]time.Time // a última rodada completa, por integração
	lastTick map[uuid.UUID]time.Time // a última vez que a rotina de fundo olhou a integração
}

// New monta o sincronizador. Ele não faz nada sozinho: as rodadas vêm do botão (SyncNow), do gancho das
// tarefas (Notify, Flush) e da rotina de fundo (Run).
func New(d Deps, cfg Config) *Syncer {
	cfg = cfg.withDefaults()
	return &Syncer{
		d: d, cfg: cfg,
		cache:    newCache(cfg.Now),
		locks:    map[uuid.UUID]*sync.Mutex{},
		queue:    newQueue(),
		backoff:  map[uuid.UUID]*backoff{},
		lastRun:  map[uuid.UUID]time.Time{},
		lastTick: map[uuid.UUID]time.Time{},
	}
}

func (s *Syncer) lock(id uuid.UUID) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.locks[id]
	if m == nil {
		m = &sync.Mutex{}
		s.locks[id] = m
	}
	return m
}

// SyncNow é a rodada que a pessoa pede pelo botão: completa, e perguntando tudo de novo ao GitHub (o
// que ela acabou de mudar no perfil dele conta). Recusa se já há uma rodada nesta integração.
func (s *Syncer) SyncNow(ctx context.Context, id uuid.UUID) (*Summary, error) {
	sum, err := s.syncNow(ctx, id)
	if !errors.Is(err, ErrSyncRunning) {
		// Quem aperta o botão também quer as tarefas novas que ainda não saíram: depois da rodada, sem o cadeado dela.
		s.retryParked(ctx)
	}
	return sum, err
}

func (s *Syncer) syncNow(ctx context.Context, id uuid.UUID) (*Summary, error) {
	lock := s.lock(id)
	if !lock.TryLock() {
		return nil, ErrSyncRunning
	}
	defer lock.Unlock()
	s.cache.forget(id.String() + "/")
	return s.runLocked(ctx, id, Full)
}

// Sync é a rodada de fundo: se já há outra rodada nesta integração, não faz nada.
func (s *Syncer) Sync(ctx context.Context, id uuid.UUID, mode Mode) (*Summary, error) {
	lock := s.lock(id)
	if !lock.TryLock() {
		return nil, ErrSyncRunning
	}
	defer lock.Unlock()
	return s.runLocked(ctx, id, mode)
}

// run é uma rodada: a integração, a conexão com a plataforma e o que ela leu do banco.
type run struct {
	s     *Syncer
	ctx   context.Context
	integ *integration.Integration
	conn  adapter.Connection
	src   adapter.IssueSyncer
	orgID uuid.UUID
	// caps é o que o tipo sabe sincronizar além do básico; k liga o id guardado na tarefa à chave do vínculo;
	// label e numeric nomeiam o item quando ele chega sem título.
	caps    adapter.SyncCaps
	k       keyer
	label   string
	numeric bool

	readOnly bool
	sum      Summary
	pushes   int
	// problem é o pior aviso da rodada (o código que a integração mostra), sem ser um erro que parou tudo.
	problem string

	rows   map[string]*Row
	tasks  map[uuid.UUID]*task.Task
	linked map[string][]*task.Task // tarefas ligadas à mão a cada item, as mais antigas primeiro
	bound  map[uuid.UUID]bool      // tarefas que já têm vínculo
	labels map[string]bool         // as etiquetas que o repositório tem, carregadas na primeira vez que se precisa
}

// keyer liga o id de item guardado numa tarefa à chave do vínculo (adapter.Issue.ID). Nos tipos com id
// numérico, o que não é número não é de item nenhum.
type keyer struct {
	numeric bool
	norm    adapter.ItemNormalizer
}

func newKeyer(impl adapter.Integration) keyer {
	k := keyer{numeric: impl.Descriptor().ItemNumeric}
	k.norm, _ = impl.(adapter.ItemNormalizer)
	return k
}

// key devolve a chave do item e se o id guardado serve de chave.
func (k keyer) key(raw string) (string, bool) {
	id := raw
	if k.norm != nil {
		id = k.norm.NormalizeItemID(raw)
	}
	if k.numeric {
		if _, err := strconv.Atoi(id); err != nil {
			return "", false
		}
	}
	return id, id != ""
}

// stop é o erro que acaba a rodada inteira, e não só o item: a plataforma recusou o token, mandou parar
// de pedir, ou a rodada foi cancelada.
func stop(err error) bool { return adapter.StopsSync(err) }

// newRun lê a integração e fala com o GitHub o bastante para saber se pode seguir: o repositório existe
// e o token escreve nele ou não.
func (s *Syncer) newRun(ctx context.Context, id uuid.UUID, cachedRepo bool) (*run, error) {
	conn, integ, err := s.d.Integrations.Connection(id.String())
	if err != nil {
		return nil, err
	}
	if !integ.SyncIssues || !integ.Enabled {
		return nil, ErrSyncOff
	}
	impl, err := adapter.GetIntegration(integ.Type)
	if err != nil {
		return nil, err
	}
	src, ok := impl.(adapter.IssueSyncer)
	if !ok {
		return nil, ErrSyncOff
	}
	orgID, err := s.d.Rows.OrganizationOf(integ.ProjectID)
	if err != nil {
		return nil, err
	}
	desc := impl.Descriptor()
	r := &run{
		s: s, ctx: ctx, integ: integ, conn: conn, src: src, orgID: orgID,
		caps: desc.Caps, k: newKeyer(impl), label: desc.Label, numeric: desc.ItemNumeric,
		rows: map[string]*Row{}, tasks: map[uuid.UUID]*task.Task{}, linked: map[string][]*task.Task{}, bound: map[uuid.UUID]bool{},
	}

	// O repositório é lido a cada rodada; a do gancho, que é uma issue só, confia no que a última viu.
	readRepo := func() (string, error) {
		repo, err := src.Repo(ctx, conn)
		if err != nil {
			return "", err
		}
		if repo.CanPush && !repo.Archived {
			return "write", nil
		}
		return "read", nil
	}
	key := id.String() + "/repo"
	var mode string
	if cachedRepo {
		mode, err = s.cache.get(key, cacheRepo, cacheRepo, readRepo)
	} else if mode, err = readRepo(); err == nil {
		s.cache.put(key, mode, cacheRepo)
	}
	if err != nil {
		return nil, err
	}
	r.readOnly = mode != "write"
	if r.readOnly {
		r.note(ErrReadOnly.Code)
	}
	return r, nil
}

// note guarda o aviso mais importante da rodada: o primeiro que chega manda, e os de baixo não o trocam.
func (r *run) note(code string) {
	rank := func(c string) int {
		switch c {
		case "":
			return 0
		case ErrNoLogin.Code:
			return 1
		case ErrLabelRefused.Code:
			return 2
		case ErrPushRejected.Code:
			return 3
		case ErrPushDiscarded.Code:
			return 4
		case ErrReadOnly.Code:
			return 5
		}
		return 6
	}
	if rank(code) > rank(r.problem) {
		r.problem = code
	}
}

// load lê do banco os vínculos e as tarefas ligadas à integração.
func (r *run) load() error {
	rows, err := r.s.d.Rows.ByIntegration(r.integ.ID)
	if err != nil {
		return err
	}
	linked, err := r.s.d.Tasks.ListLinked(r.integ.ID)
	if err != nil {
		return err
	}
	r.rows = rows
	r.tasks = make(map[uuid.UUID]*task.Task, len(linked))
	r.linked = map[string][]*task.Task{}
	r.bound = map[uuid.UUID]bool{}
	for _, row := range rows {
		if row.TaskID != nil {
			r.bound[*row.TaskID] = true
		}
	}
	for i := range linked {
		t := &linked[i]
		r.tasks[t.ID] = t
		if id, ok := r.k.key(*t.ExternalItemID); ok && !r.bound[t.ID] {
			r.linked[id] = append(r.linked[id], t)
		}
	}
	return nil
}

// runLocked faz a rodada de uma integração; quem chama já tem o cadeado dela.
func (s *Syncer) runLocked(ctx context.Context, id uuid.UUID, mode Mode) (*Summary, error) {
	r, err := s.newRun(ctx, id, false)
	if err != nil {
		if !errors.Is(err, ErrSyncOff) && !errors.Is(err, context.Canceled) {
			s.record(id, nil, err, "")
		}
		return nil, err
	}
	if err := r.load(); err != nil {
		return nil, err
	}

	cursor := r.integ.SyncCursor
	if cursor == nil || !r.caps.ServerSince {
		// Sem filtro por data na plataforma não há rodada incremental: toda rodada olha tudo.
		mode = Full
	}
	opts := adapter.ListIssuesOptions{State: "open"}
	if mode == Incremental {
		opts = adapter.ListIssuesOptions{State: "all", Since: *cursor, ByUpdated: true}
	}
	list, listErr := r.src.ListIssues(ctx, r.conn, opts)

	seen := map[string]bool{}
	var fatal error
	for _, issue := range list.Issues {
		seen[issue.ID] = true
		if fatal = r.handleListed(issue); fatal != nil {
			break
		}
	}
	if fatal == nil && listErr != nil && stop(listErr) {
		fatal = listErr
	}
	// A rodada completa também olha as issues ligadas que a lista de abertas não trouxe: foram
	// fechadas, apagadas ou transferidas desde a última vez.
	if fatal == nil && listErr == nil && mode == Full {
		fatal = r.checkUnseen(seen)
	}
	if listErr != nil {
		r.sum.Partial = true
		if fatal == nil {
			r.sum.Errors++
			r.problem = codeOf(listErr)
		}
	}

	if ctx.Err() != nil {
		return &r.sum, ctx.Err()
	}
	var next *time.Time
	if fatal == nil && listErr == nil && !r.sum.Partial {
		t := list.ServerTime.Add(-2 * time.Minute)
		if cursor == nil || t.After(*cursor) {
			next = &t
		}
		s.mu.Lock()
		if mode == Full {
			s.lastRun[id] = s.cfg.Now()
		}
		s.mu.Unlock()
	}
	if fatal != nil {
		r.sum.Partial = true
	}
	if n, err := s.unmatched(id); err == nil {
		r.sum.Unmapped = n
	}
	s.record(id, next, fatal, r.problem)
	return &r.sum, fatal
}

// record deixa na integração o resultado da rodada: quando terminou, o erro que a parou (ou o aviso
// que ela acusou) e, se ela viu tudo, o ponto de onde a próxima continua.
func (s *Syncer) record(id uuid.UUID, cursor *time.Time, fatal error, problem string) {
	code := problem
	if fatal != nil {
		code = codeOf(fatal)
	}
	if err := s.d.Integrations.RecordSync(id, integration.SyncResult{At: s.cfg.Now(), Cursor: cursor, Err: code}); err != nil {
		s.cfg.Logger.Error("issue sync: recording the result", "integration", id, "error", err)
	}
}

func (s *Syncer) unmatched(id uuid.UUID) (int, error) {
	it, err := s.d.Integrations.Get(id.String())
	if err != nil {
		return 0, err
	}
	return it.SyncUnmatched, nil
}

// codeOf é o código de erro que a integração mostra; um erro sem código é um defeito nosso, e vira o
// erro interno, sem o texto técnico.
func codeOf(err error) string {
	if code := apperr.Code(err); code != "" {
		return code
	}
	return apperr.ErrInternal.Code
}

// handleListed trata uma issue que veio da lista. Devolve o erro que acaba a rodada, se houver; o das
// outras issues só é contado.
func (r *run) handleListed(issue adapter.Issue) error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	err := r.handle(issue)
	return r.settle(err)
}

// settle separa o erro que acaba a rodada do que só deixa a issue para a próxima.
func (r *run) settle(err error) error {
	if err == nil {
		return nil
	}
	if stop(err) {
		return err
	}
	r.sum.Errors++
	if r.problem == "" {
		r.problem = codeOf(err)
	}
	r.s.cfg.Logger.Warn("issue sync: issue skipped", "integration", r.integ.ID, "error", err)
	return nil
}

// handle decide o que é a issue para o sistema: uma tarefa nova, uma tarefa ligada à mão que passa a ser
// sincronizada, uma issue descartada, ou uma que já tem tarefa.
func (r *run) handle(issue adapter.Issue) error {
	row := r.rows[issue.ID]
	switch {
	case row == nil:
		if issue.State != stateOpen {
			return nil // só as abertas viram tarefa
		}
		if t := r.manualLink(issue.ID); t != nil {
			return r.adopt(nil, t, issue)
		}
		return r.importIssue(issue)
	case row.TaskID == nil:
		// A tarefa foi excluída aqui, e a issue não volta. Só uma tarefa ligada de novo à mão a traz de volta.
		if issue.State == stateOpen {
			if t := r.manualLink(issue.ID); t != nil {
				return r.adopt(row, t, issue)
			}
		}
		return nil
	default:
		return r.reconcile(row, issue)
	}
}

// manualLink é a tarefa mais antiga ligada à mão a esta issue que ainda não é de nenhum vínculo.
func (r *run) manualLink(id string) *task.Task {
	for _, t := range r.linked[id] {
		if !r.bound[t.ID] {
			return t
		}
	}
	return nil
}

// checkUnseen confere as issues ligadas e abertas na última rodada que a lista de abertas não trouxe.
func (r *run) checkUnseen(seen map[string]bool) error {
	for id, row := range r.rows {
		if row.TaskID == nil || seen[id] || row.State == stateGone {
			continue
		}
		// A issue fechada não está na lista de abertas e só interessa se a tarefa mudou aqui desde o
		// acordo (reabrir, renomear): ver isso não custa uma requisição, então a conferência só vai ao
		// GitHub por ela quando há o que mandar.
		if row.State == stateClosed && !r.dirty(row) {
			continue
		}
		if err := r.ctx.Err(); err != nil {
			return err
		}
		issue, err := r.src.GetIssue(r.ctx, r.conn, id)
		if errors.Is(err, adapter.ErrIssueGone) {
			r.markGone(row)
			continue
		}
		if err != nil {
			if fatal := r.settle(err); fatal != nil {
				return fatal
			}
			continue
		}
		if issue.PullRequest {
			r.markGone(row)
			continue
		}
		if fatal := r.settle(r.reconcile(row, *issue)); fatal != nil {
			return fatal
		}
	}
	return nil
}

// dirty diz se a tarefa ligada ao vínculo difere do último acordo em algo que vai para o GitHub.
func (r *run) dirty(row *Row) bool {
	t := r.tasks[*row.TaskID]
	if t == nil || !r.linkedTo(t, row.ItemID) {
		return t != nil // desvinculada: o vínculo precisa ser solto
	}
	l, b := toLocal(t), row.snapshot()
	return norm(l.Title) != norm(b.Title) || norm(l.Body) != norm(b.Body) || l.Closed != b.Closed ||
		!sameKeys(keyed(l.Labels), keyed(b.Labels)) || (r.caps.Assignee && !samePerson(l.Person, b.MappedPerson)) ||
		(r.caps.Deadline && !normDeadline(l.Deadline).Equal(normDeadline(b.Deadline)))
}

// markGone anota que a issue sumiu do repositório (apagada ou transferida). A tarefa fica como está.
func (r *run) markGone(row *Row) {
	if row.State == stateGone {
		return
	}
	row.State = stateGone
	row.SyncedAt = r.s.cfg.Now()
	if err := r.s.d.Rows.Save(row); err != nil {
		r.s.cfg.Logger.Error("issue sync: marking an issue as gone", "issue", row.ItemID, "error", err)
	}
}

// SyncTask sincroniza a issue de uma tarefa: é o que o gancho dispara depois que ela mudou aqui. Espera
// a vez da integração (uma rodada em andamento termina antes).
func (s *Syncer) SyncTask(ctx context.Context, row *Row) error {
	lock := s.lock(row.IntegrationID)
	lock.Lock()
	defer lock.Unlock()
	return s.syncTaskLocked(ctx, row)
}

func (s *Syncer) syncTaskLocked(ctx context.Context, row *Row) error {
	if row.TaskID == nil {
		return nil
	}
	r, err := s.newRun(ctx, row.IntegrationID, true)
	if err != nil {
		return err
	}
	// Relê o vínculo: a rodada que esperamos pode tê-lo mudado.
	rows, err := s.d.Rows.ByTask(*row.TaskID)
	if err != nil || rows == nil {
		return err
	}
	t, err := s.d.Tasks.GetByID(rows.TaskID.String())
	if err != nil {
		return err
	}
	r.rows = map[string]*Row{rows.ItemID: rows}
	r.tasks = map[uuid.UUID]*task.Task{t.ID: t}
	issue, err := r.src.GetIssue(ctx, r.conn, rows.ItemID)
	if errors.Is(err, adapter.ErrIssueGone) {
		r.markGone(rows)
		return nil
	}
	if err != nil {
		return err
	}
	if issue.PullRequest {
		r.markGone(rows)
		return nil
	}
	return r.reconcile(rows, *issue)
}

// add soma o que outra rodada fez.
func (s *Summary) add(o *Summary) {
	s.Created += o.Created
	s.Updated += o.Updated
	s.Closed += o.Closed
	s.Pushed += o.Pushed
	s.Unmapped += o.Unmapped
	s.Errors += o.Errors
	s.Partial = s.Partial || o.Partial
}

// SyncProject é o botão Sincronizar das listas de tarefas: uma rodada completa em cada integração do
// projeto que está com a sincronização ligada, uma depois da outra, e a soma do que fizeram. Se uma não
// pôde rodar (já havia uma rodada nela, ou o GitHub recusou), a soma sai como parcial; se nenhuma rodou,
// devolve o motivo da primeira.
func (s *Syncer) SyncProject(ctx context.Context, projectID uuid.UUID) (*Summary, error) {
	list, err := s.d.Integrations.ListByProject(projectID.String())
	if err != nil {
		return nil, err
	}
	var total Summary
	var considered, ran int
	var first error
	running := false
	for _, it := range list {
		if !it.SyncIssues || !it.Enabled {
			continue
		}
		considered++
		sum, err := s.SyncNow(ctx, it.ID)
		if sum != nil {
			total.add(sum)
		}
		switch {
		case err == nil:
			ran++
		case errors.Is(err, ErrSyncRunning):
			running = true
		case first == nil:
			first = err
		}
	}
	switch {
	case considered == 0:
		return nil, ErrSyncOff
	case ran == 0 && first != nil:
		return nil, first
	case ran == 0:
		return nil, ErrSyncRunning
	}
	if first != nil || running {
		total.Partial = true
	}
	return &total, nil
}

// TaskSummary é o que o botão Sincronizar de uma tarefa fez: o mesmo resumo da rodada e, quando o GitHub
// deixou algo de fora (uma mudança descartada, um responsável sem usuário), o código do aviso.
type TaskSummary struct {
	Summary
	Problem string `json:"problem,omitempty"`
}

// SyncTaskNow é o botão Sincronizar da tela da tarefa: relê a issue dela no GitHub e põe as duas em
// acordo, sem esperar a rodada de fundo (o GitHub não avisa quando algo muda lá). Serve também à tarefa
// ligada à mão, que passa a ser sincronizada se a issue está aberta. Recusa se há uma rodada em andamento
// na integração.
func (s *Syncer) SyncTaskNow(ctx context.Context, taskID uuid.UUID) (*TaskSummary, error) {
	t, err := s.d.Tasks.GetByID(taskID.String())
	if err != nil {
		return nil, err
	}
	if t.ExternalIntegrationID == nil || t.ExternalItemID == nil {
		return nil, task.ErrNoExternalItem
	}
	// Nos tipos com id numérico, o id que não é número não é de item nenhum, e nem se espera a vez da integração.
	if t.ExternalIntegration != nil {
		if impl, err := adapter.GetIntegration(t.ExternalIntegration.Type); err == nil {
			if _, ok := newKeyer(impl).key(*t.ExternalItemID); !ok {
				return nil, task.ErrNoExternalItem
			}
		}
	}
	id := *t.ExternalIntegrationID
	lock := s.lock(id)
	if !lock.TryLock() {
		return nil, ErrSyncRunning
	}
	defer lock.Unlock()
	s.cache.forget(id.String() + "/")

	r, err := s.newRun(ctx, id, false)
	if err != nil {
		return nil, err
	}
	if err := r.load(); err != nil {
		return nil, err
	}
	itemID, ok := r.k.key(*t.ExternalItemID)
	if !ok {
		return nil, task.ErrNoExternalItem
	}
	issue, err := r.src.GetIssue(ctx, r.conn, itemID)
	if errors.Is(err, adapter.ErrIssueGone) {
		if row := r.rows[itemID]; row != nil {
			r.markGone(row)
		}
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	if issue.PullRequest {
		if row := r.rows[itemID]; row != nil {
			r.markGone(row)
		}
		return nil, adapter.ErrIssueGone
	}
	if err := r.handle(*issue); err != nil {
		return nil, err
	}
	return &TaskSummary{Summary: r.sum, Problem: r.problem}, nil
}

func itoa(n int) string { return strconv.Itoa(n) }
