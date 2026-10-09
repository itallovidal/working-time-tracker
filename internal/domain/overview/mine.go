package overview

import (
	"cmp"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/task"
)

// Mine é o painel de quem abriu a primeira tela: o que a pessoa trabalhou hoje e na semana e como
// estão as tarefas que são dela, em todos os projetos da organização. Cada pessoa lê o seu, seja
// qual for o papel, e nunca o de outra; por isso não leva dinheiro nem nada de ninguém mais.
type Mine struct {
	// GeneratedAt é o instante da conta: a sessão aberta foi medida até aqui.
	GeneratedAt time.Time `json:"generated_at"`
	// Timezone é o fuso em que "hoje" e "semana" foram medidos, e WeekStart, a segunda-feira à
	// meia-noite dele em que a semana começou.
	Timezone  string    `json:"timezone"`
	WeekStart time.Time `json:"week_start"`
	// WeeklyHours é a jornada semanal combinada com a pessoa; nil quando ninguém informou.
	WeeklyHours *int      `json:"weekly_hours"`
	Hours       MineHours `json:"hours"`
	Tasks       MineTasks `json:"tasks"`
	// WorkingNow diz que a pessoa está com o ponto aberto, em qualquer projeto, e WorkingOn em que
	// tarefas (vazia, nunca nula, quando a sessão ficou sem tarefa).
	WorkingNow bool        `json:"working_now"`
	WorkingOn  []WorkingOn `json:"working_on"`
}

// MineHours é o tempo da pessoa, só o dela, somando os projetos todos.
type MineHours struct {
	TodaySeconds float64 `json:"today_seconds"`
	WeekSeconds  float64 `json:"week_seconds"`
	// Days são os sete dias da semana, de segunda a domingo; o que ainda não chegou vem com zero.
	Days []MineDay `json:"days"`
	// Projects é onde as horas da semana foram, projeto a projeto, do que mais teve tempo para o que menos
	// (é o tempo da própria pessoa em cada projeto, não uma comparação entre pessoas); só os que têm tempo.
	Projects []MineProject `json:"projects"`
}

// MineProject é o tempo da semana da pessoa num projeto.
type MineProject struct {
	Project Ref     `json:"project"`
	Seconds float64 `json:"seconds"`
}

// MineDay é o tempo de um dia (AAAA-MM-DD, no fuso do painel).
type MineDay struct {
	Date    string  `json:"date"`
	Seconds float64 `json:"seconds"`
}

// MineTasks conta as tarefas da pessoa. Aberta é toda a que não está fechada. Atrasada é a aberta
// cujo prazo já passou, e DueThisWeek, a aberta que ainda vence até o domingo.
type MineTasks struct {
	Total           int `json:"total"`
	Open            int `json:"open"`
	Backlog         int `json:"backlog"`
	InProgress      int `json:"in_progress"`
	AwaitingClosure int `json:"awaiting_closure"`
	Closed          int `json:"closed"`
	Overdue         int `json:"overdue"`
	DueThisWeek     int `json:"due_this_week"`
}

// MyTask é uma tarefa da pessoa na lista da primeira tela, com o projeto dela. Deadline é nil
// quando a tarefa não tem prazo.
type MyTask struct {
	ID       uuid.UUID  `json:"id"`
	Name     string     `json:"name"`
	Status   string     `json:"status"`
	Priority string     `json:"priority"`
	Deadline *time.Time `json:"deadline"`
	Project  Ref        `json:"project"`
}

// MyTasksPage é uma página das tarefas da pessoa. Total conta todas as do estado pedido.
type MyTasksPage struct {
	Items   []MyTask `json:"items"`
	Total   int      `json:"total"`
	Page    int      `json:"page"`
	PerPage int      `json:"per_page"`
}

// Os dois estados da lista: as tarefas por fazer e as fechadas.
const (
	StateOpen   = "open"
	StateClosed = "closed"
)

// Os tamanhos de página da lista de tarefas, como os da lista de projetos.
const (
	defaultTasksPerPage = 10
	maxTasksPerPage     = 100
)

// openStatuses são os status de uma tarefa que ainda não terminou.
var openStatuses = []string{"backlog", "in_progress", "awaiting_closure"}

// Mine monta o painel da pessoa. tz é o nome do fuso (America/Sao_Paulo) de quem olha, que decide
// onde começam o dia e a semana; um nome vazio ou desconhecido vale UTC, e o fuso usado volta na
// resposta. Um ID malformado vale como "não encontrado".
func (s *Service) Mine(orgID, personID, tz string) (*Mine, error) {
	if _, err := uuid.Parse(orgID); err != nil {
		return nil, database.ErrNotFound
	}
	pid, err := uuid.Parse(personID)
	if err != nil {
		return nil, database.ErrNotFound
	}
	loc := location(tz)
	weekStart := startOfWeek(s.now(), loc)

	me, err := s.deps.People.Get(personID)
	if err != nil {
		return nil, err
	}
	sessions, err := s.deps.Sessions.ListByPersonSince(orgID, personID, weekStart)
	if err != nil {
		return nil, err
	}
	// Um instante só para a resposta inteira, tirado depois de ler as sessões.
	now := s.now()

	m := &Mine{
		GeneratedAt: now,
		Timezone:    loc.String(),
		WeekStart:   weekStart,
		WeeklyHours: me.WeeklyHours,
		Hours:       MineHours{Days: make([]MineDay, 7), Projects: []MineProject{}},
		WorkingOn:   []WorkingOn{},
	}

	todayStart := startOfDay(now, loc)
	for i := range m.Hours.Days {
		from := weekStart.AddDate(0, 0, i)
		m.Hours.Days[i].Date = from.Format(time.DateOnly)
		to := weekStart.AddDate(0, 0, i+1)
		for j := range sessions {
			// Within mede da borda até o fim da sessão: o dia é o que sobra tirando o que vem depois dele.
			in, _, _ := sessions[j].Within(from, now)
			after, _, _ := sessions[j].Within(to, now)
			m.Hours.Days[i].Seconds += in - after
		}
	}
	byProject := map[uuid.UUID]float64{}
	for j := range sessions {
		week, _, _ := sessions[j].Within(weekStart, now)
		today, _, _ := sessions[j].Within(todayStart, now)
		m.Hours.WeekSeconds += week
		m.Hours.TodaySeconds += today
		if week > 0 {
			byProject[sessions[j].ProjectID] += week
		}
	}
	if err := s.fillMineProjects(m, byProject); err != nil {
		return nil, err
	}

	if err := s.fillMineTasks(m, orgID, personID, now, weekStart.AddDate(0, 0, 7).Add(-time.Nanosecond)); err != nil {
		return nil, err
	}

	open, err := s.openWork(orgID)
	if err != nil {
		return nil, err
	}
	for _, p := range open {
		if p.PersonID == pid {
			m.WorkingNow = true
			m.WorkingOn = p.WorkingOn
		}
	}
	return m, nil
}

// fillMineProjects põe em m as horas da semana por projeto, com o nome de cada um. Um projeto que sumiu no
// meio do caminho fica de fora, sem derrubar a resposta.
func (s *Service) fillMineProjects(m *Mine, seconds map[uuid.UUID]float64) error {
	for id, secs := range seconds {
		p, err := s.deps.Projects.Get(id.String())
		if errors.Is(err, database.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		m.Hours.Projects = append(m.Hours.Projects, MineProject{Project: Ref{ID: p.ID, Name: p.Name}, Seconds: secs})
	}
	slices.SortFunc(m.Hours.Projects, func(a, b MineProject) int {
		return cmp.Or(
			cmp.Compare(b.Seconds, a.Seconds),
			strings.Compare(a.Project.Name, b.Project.Name),
			strings.Compare(a.Project.ID.String(), b.Project.ID.String()),
		)
	})
	return nil
}

// fillMineTasks conta as tarefas da pessoa: por status numa consulta, e as atrasadas e as que vencem
// até weekEnd em mais duas.
func (s *Service) fillMineTasks(m *Mine, orgID, personID string, now, weekEnd time.Time) error {
	byStatus, err := s.deps.Tasks.AssignedByStatus(orgID, personID)
	if err != nil {
		return err
	}
	t := &m.Tasks
	t.Backlog, t.InProgress, t.AwaitingClosure, t.Closed = byStatus["backlog"], byStatus["in_progress"], byStatus["awaiting_closure"], byStatus["closed"]
	t.Open = t.Backlog + t.InProgress + t.AwaitingClosure
	t.Total = t.Open + t.Closed

	if t.Open == 0 {
		return nil
	}
	if t.Overdue, err = s.deps.Tasks.CountAssigned(orgID, personID, task.ListFilter{Statuses: openStatuses, DeadlineTo: &now}); err != nil {
		return err
	}
	untilWeekEnd, err := s.deps.Tasks.CountAssigned(orgID, personID, task.ListFilter{Statuses: openStatuses, DeadlineTo: &weekEnd})
	if err != nil {
		return err
	}
	// As atrasadas também vencem antes do fim da semana: o que sobra é o que ainda vai vencer.
	t.DueThisWeek = max(0, untilWeekEnd-t.Overdue)
	return nil
}

// MyTasks lista as tarefas da pessoa em todos os projetos da organização, do estado pedido (StateOpen
// ou StateClosed). As abertas vêm com as que vencem primeiro (as atrasadas à frente), depois as de
// maior prioridade e as mais novas; as fechadas, da mais nova para a mais antiga, porque a tarefa
// não guarda quando foi fechada. page e perPage zerados valem a página 1 e dez por página; uma página
// além da última volta a última. Um ID malformado vale como "não encontrado".
func (s *Service) MyTasks(orgID, personID, state string, page, perPage int) (*MyTasksPage, error) {
	if _, err := uuid.Parse(orgID); err != nil {
		return nil, database.ErrNotFound
	}
	if _, err := uuid.Parse(personID); err != nil {
		return nil, database.ErrNotFound
	}
	if perPage <= 0 {
		perPage = defaultTasksPerPage
	}
	perPage = min(perPage, maxTasksPerPage)
	if page <= 0 {
		page = 1
	}

	out := &MyTasksPage{Items: []MyTask{}, PerPage: perPage}
	var rows []task.Assigned
	switch state {
	case StateClosed:
		total, err := s.deps.Tasks.CountAssigned(orgID, personID, task.ListFilter{Statuses: []string{"closed"}})
		if err != nil {
			return nil, err
		}
		out.Total, out.Page = total, clampPage(page, total, perPage)
		if total == 0 {
			return out, nil
		}
		if rows, err = s.deps.Tasks.ListAssigned(orgID, personID, task.ListFilter{Statuses: []string{"closed"}, Page: out.Page, PerPage: perPage}); err != nil {
			return nil, err
		}
	default:
		all, err := s.deps.Tasks.ListAssigned(orgID, personID, task.ListFilter{Statuses: openStatuses})
		if err != nil {
			return nil, err
		}
		slices.SortFunc(all, compareOpen)
		out.Total, out.Page = len(all), clampPage(page, len(all), perPage)
		from := min((out.Page-1)*perPage, len(all))
		rows = all[from:min(from+perPage, len(all))]
	}

	for i := range rows {
		t := &rows[i]
		mt := MyTask{
			ID:       t.ID,
			Name:     t.Name,
			Status:   t.Status,
			Priority: t.Priority,
			Project:  Ref{ID: t.ProjectID, Name: t.ProjectName},
		}
		if t.HasDeadline() {
			d := t.Deadline
			mt.Deadline = &d
		}
		out.Items = append(out.Items, mt)
	}
	return out, nil
}

// compareOpen põe as tarefas abertas na ordem da lista: as com prazo antes das sem prazo e, entre
// elas, o prazo mais perto primeiro; depois a prioridade mais alta, a mais nova e, por fim, o id,
// para a ordem não mudar de uma página para a outra.
func compareOpen(a, b task.Assigned) int {
	byDeadline := 0
	switch {
	case a.HasDeadline() && b.HasDeadline():
		byDeadline = a.Deadline.Compare(b.Deadline)
	case a.HasDeadline():
		byDeadline = -1
	case b.HasDeadline():
		byDeadline = 1
	}
	return cmp.Or(
		byDeadline,
		cmp.Compare(priorityRank(a.Priority), priorityRank(b.Priority)),
		b.CreatedAt.Compare(a.CreatedAt),
		strings.Compare(a.ID.String(), b.ID.String()),
	)
}

// priorityRank é a posição da prioridade, da mais urgente (0) à sem prioridade.
func priorityRank(p string) int {
	if i := slices.Index(task.Priorities, p); i >= 0 {
		return i
	}
	return len(task.Priorities)
}

// clampPage leva a página pedida para dentro das que existem (sempre ao menos a primeira).
func clampPage(page, total, perPage int) int {
	last := max(1, (total+perPage-1)/perPage)
	return min(max(page, 1), last)
}

// location acha o fuso pelo nome; vazio, "Local" ou desconhecido valem UTC.
func location(name string) *time.Location {
	if name == "" || name == "Local" {
		return time.UTC
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
}

// startOfDay é a meia-noite do dia de t no fuso loc.
func startOfDay(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

// startOfWeek é a segunda-feira à meia-noite da semana de t no fuso loc.
func startOfWeek(t time.Time, loc *time.Location) time.Time {
	midnight := startOfDay(t, loc)
	return midnight.AddDate(0, 0, -((int(midnight.Weekday()) + 6) % 7))
}
