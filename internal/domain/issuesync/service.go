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
	// Debounce é quanto o gancho espera, depois da primeira mudança, para juntar as que vierem.
	Debounce time.Duration
	// FullEvery é de quanto em quanto tempo a rotina de fundo faz uma rodada completa.
	FullEvery time.Duration
	// MaxPushes é o teto de issues mudadas numa rodada; o resto fica para a seguinte.
	MaxPushes int
	Now       func() time.Time
	Logger    *slog.Logger
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

	queue   *queue
	backoff map[uuid.UUID]*backoff
	lastRun map[uuid.UUID]time.Time // a última rodada completa, por integração
}

// New monta o sincronizador. Ele não faz nada sozinho: as rodadas vêm do botão (SyncNow), do gancho das
// tarefas (Notify, Flush) e da rotina de fundo (Run).
func New(d Deps, cfg Config) *Syncer {
	cfg = cfg.withDefaults()
	return &Syncer{
		d: d, cfg: cfg,
		cache:   newCache(cfg.Now),
		locks:   map[uuid.UUID]*sync.Mutex{},
		queue:   newQueue(),
		backoff: map[uuid.UUID]*backoff{},
		lastRun: map[uuid.UUID]time.Time{},
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

// run é uma rodada: a integração, a conexão com o GitHub e o que ela leu do banco.
type run struct {
	s     *Syncer
	ctx   context.Context
	integ *integration.Integration
	conn  adapter.Connection
	gh    adapter.IssueSyncer
	orgID uuid.UUID

	readOnly bool
	sum      Summary
	pushes   int
	// problem é o pior aviso da rodada (o código que a integração mostra), sem ser um erro que parou tudo.
	problem string

	rows   map[int]*Row
	tasks  map[uuid.UUID]*task.Task
	linked map[int][]*task.Task // tarefas ligadas à mão a cada número, as mais antigas primeiro
	bound  map[uuid.UUID]bool   // tarefas que já têm vínculo
	labels map[string]bool      // as etiquetas que o repositório tem, carregadas na primeira vez que se precisa
}

// stop é o erro que acaba a rodada inteira, e não só a issue: o GitHub recusou o token, mandou parar
// de pedir, ou a rodada foi cancelada.
func stop(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if _, limited := adapter.RateLimitedUntil(err); limited {
		return true
	}
	return errors.Is(err, adapter.ErrInvalidToken) || errors.Is(err, adapter.ErrGitHubRepoNotFound) || errors.Is(err, adapter.ErrProviderUnreachable)
}

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
	gh, ok := impl.(adapter.IssueSyncer)
	if !ok {
		return nil, ErrSyncOff
	}
	orgID, err := s.d.Rows.OrganizationOf(integ.ProjectID)
	if err != nil {
		return nil, err
	}
	r := &run{
		s: s, ctx: ctx, integ: integ, conn: conn, gh: gh, orgID: orgID,
		rows: map[int]*Row{}, tasks: map[uuid.UUID]*task.Task{}, linked: map[int][]*task.Task{}, bound: map[uuid.UUID]bool{},
	}

	// O repositório é lido a cada rodada; a do gancho, que é uma issue só, confia no que a última viu.
	readRepo := func() (string, error) {
		repo, err := gh.Repo(ctx, conn)
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
	r.linked = map[int][]*task.Task{}
	r.bound = map[uuid.UUID]bool{}
	for _, row := range rows {
		if row.TaskID != nil {
			r.bound[*row.TaskID] = true
		}
	}
	for i := range linked {
		t := &linked[i]
		r.tasks[t.ID] = t
		if n, err := strconv.Atoi(*t.ExternalItemID); err == nil && !r.bound[t.ID] {
			r.linked[n] = append(r.linked[n], t)
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
	if cursor == nil {
		mode = Full
	}
	opts := adapter.ListIssuesOptions{State: "open"}
	if mode == Incremental {
		opts = adapter.ListIssuesOptions{State: "all", Since: *cursor, ByUpdated: true}
	}
	list, listErr := r.gh.ListIssues(ctx, r.conn, opts)

	seen := map[int]bool{}
	var fatal error
	for _, issue := range list.Issues {
		seen[issue.Number] = true
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
	row := r.rows[issue.Number]
	switch {
	case row == nil:
		if issue.State != stateOpen {
			return nil // só as abertas viram tarefa
		}
		if t := r.manualLink(issue.Number); t != nil {
			return r.adopt(nil, t, issue)
		}
		return r.importIssue(issue)
	case row.TaskID == nil:
		// A tarefa foi excluída aqui, e a issue não volta. Só uma tarefa ligada de novo à mão a traz de volta.
		if issue.State == stateOpen {
			if t := r.manualLink(issue.Number); t != nil {
				return r.adopt(row, t, issue)
			}
		}
		return nil
	default:
		return r.reconcile(row, issue)
	}
}

// manualLink é a tarefa mais antiga ligada à mão a esta issue que ainda não é de nenhum vínculo.
func (r *run) manualLink(number int) *task.Task {
	for _, t := range r.linked[number] {
		if !r.bound[t.ID] {
			return t
		}
	}
	return nil
}

// checkUnseen confere as issues ligadas e abertas na última rodada que a lista de abertas não trouxe.
func (r *run) checkUnseen(seen map[int]bool) error {
	for number, row := range r.rows {
		if row.TaskID == nil || seen[number] || row.State == stateGone {
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
		issue, err := r.gh.GetIssue(r.ctx, r.conn, number)
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
	if t == nil || !linkedTo(t, r.integ.ID, row.Number) {
		return t != nil // desvinculada: o vínculo precisa ser solto
	}
	l, b := toLocal(t), row.snapshot()
	return norm(l.Title) != norm(b.Title) || norm(l.Body) != norm(b.Body) || l.Closed != b.Closed ||
		!sameKeys(keyed(l.Labels), keyed(b.Labels)) || !samePerson(l.Person, b.MappedPerson)
}

// markGone anota que a issue sumiu do repositório (apagada ou transferida). A tarefa fica como está.
func (r *run) markGone(row *Row) {
	if row.State == stateGone {
		return
	}
	row.State = stateGone
	row.SyncedAt = r.s.cfg.Now()
	if err := r.s.d.Rows.Save(row); err != nil {
		r.s.cfg.Logger.Error("issue sync: marking an issue as gone", "issue", row.Number, "error", err)
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
	r.rows = map[int]*Row{rows.Number: rows}
	r.tasks = map[uuid.UUID]*task.Task{t.ID: t}
	issue, err := r.gh.GetIssue(ctx, r.conn, rows.Number)
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

func itoa(n int) string { return strconv.Itoa(n) }
