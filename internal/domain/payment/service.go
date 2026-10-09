package payment

import (
	"cmp"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/work_session"
)

const (
	// DefaultHistory é quantos períodos fechados a resposta traz quando o pedido não diz.
	DefaultHistory = 12
	// MaxHistory limita ?history=N.
	MaxHistory = 60
	// UpcomingCount é quantas datas de pagamento futuras a resposta traz.
	UpcomingCount = 3
)

// Deps são os domínios de onde os pagamentos leem.
type Deps struct {
	Orgs     *organization.Service
	People   *person.Service
	Sessions *work_session.Service
	Projects *project.Service
	// Tasks conta as tarefas do período por estado; sem ele o detalhe vem sem essa contagem.
	Tasks *task.Service
	// Now é o relógio da conta; sem ele vale time.Now. Os testes o fixam.
	Now func() time.Time
}

type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

func (s *Service) now() time.Time {
	if s.deps.Now != nil {
		return s.deps.Now()
	}
	return time.Now()
}

// Ref é um projeto citado no detalhe.
type Ref struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// ProjectShare é o que a pessoa fez num projeto dentro do período.
type ProjectShare struct {
	Project     Ref     `json:"project"`
	Seconds     float64 `json:"seconds"`
	AmountCents *int    `json:"amount_cents"`
	// RevenueCents é o que o projeto cobra dos clientes por esse tempo. Só vai para admins; para os outros é nil.
	RevenueCents *int `json:"revenue_cents,omitempty"`
}

// TaskCounts são as tarefas em que a pessoa trabalhou no período, contadas pelo estado que têm agora. O sistema não
// guarda quando uma tarefa fechou, então "fechadas" são as que ela trabalhou e hoje estão fechadas.
type TaskCounts struct {
	Worked          int `json:"worked"`
	Backlog         int `json:"backlog"`
	InProgress      int `json:"in_progress"`
	AwaitingClosure int `json:"awaiting_closure"`
	Closed          int `json:"closed"`
}

// PeriodView é um período para a tela. Start e PayDate são dias de calendário YYYY-MM-DD no fuso da organização,
// os dois inclusivos: nunca instantes, que o navegador deslocaria de dia.
type PeriodView struct {
	Start   string `json:"start"`
	PayDate string `json:"pay_date"`
	// Days são os dias do período.
	Days int `json:"days"`
	// Seconds e AmountCents são o que foi feito no período (até agora, no corrente). AmountCents é nil para o dono,
	// que não tem valor pago, e nos períodos que ainda não começaram não é preenchido.
	Seconds     float64 `json:"seconds"`
	AmountCents *int    `json:"amount_cents"`
	// DaysLeft (no corrente) são os dias até o pagamento; 0 é hoje.
	DaysLeft *int `json:"days_left,omitempty"`
	// GoalSeconds (no corrente, com jornada) é a meta de horas: jornada × dias do período / 7.
	GoalSeconds *float64 `json:"goal_seconds,omitempty"`
	// Projects (no corrente) é o detalhe por projeto.
	Projects []ProjectShare `json:"projects,omitempty"`
	// RevenueCents e MarginCents (no corrente, só para admins) são o que o tempo da pessoa rendeu aos projetos e o que
	// sobra depois do que ela recebe: a soma de todos os projetos dela.
	RevenueCents *int `json:"revenue_cents,omitempty"`
	MarginCents  *int `json:"margin_cents,omitempty"`
	// Tasks (no corrente) são as tarefas em que ela trabalhou no período.
	Tasks *TaskCounts `json:"tasks,omitempty"`
}

// Totals é o acumulado dos períodos fechados do histórico.
type Totals struct {
	Seconds     float64 `json:"seconds"`
	AmountCents *int    `json:"amount_cents"`
}

// Lifetime é tudo o que a pessoa já registrou, com ou sem regra de pagamento: cada sessão inteira (a aberta, até
// agora), como o "Tudo" da visão geral. AmountCents é nil para o dono. FirstDay é o dia da primeira sessão, no fuso da
// organização (vazio para quem nunca bateu ponto).
type Lifetime struct {
	Seconds     float64        `json:"seconds"`
	AmountCents *int           `json:"amount_cents"`
	FirstDay    string         `json:"first_day,omitempty"`
	Projects    []ProjectShare `json:"projects"`
}

// Summary é a resposta de GET /persons/:id/payments.
type Summary struct {
	GeneratedAt time.Time           `json:"generated_at"`
	Timezone    string              `json:"timezone"`
	Owner       bool                `json:"owner"`
	Rule        *person.PaymentRule `json:"rule"`
	Current     *PeriodView         `json:"current"`
	Upcoming    []PeriodView        `json:"upcoming"`
	History     []PeriodView        `json:"history"`
	Totals      Totals              `json:"totals"`
	Lifetime    Lifetime            `json:"lifetime"`
}

// location carrega o fuso da organização; um nome inválido cai para UTC.
func location(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
}

// Person monta os pagamentos da pessoa: os períodos da regra dela (se há uma) e o que ela registrou desde o início.
// Um ID malformado ou de outra organização vale como "não encontrado". Com revenue, o período corrente traz também o que
// o tempo da pessoa rendeu aos projetos (valor cobrado), que só os admins veem.
func (s *Service) Person(orgID, personID string, history int, revenue bool) (*Summary, error) {
	org, err := s.deps.Orgs.Get(orgID)
	if err != nil {
		return nil, err
	}
	p, err := s.deps.People.Get(personID)
	if err != nil {
		return nil, err
	}
	if p.OrganizationID.String() != orgID {
		return nil, database.ErrNotFound
	}
	if history <= 0 {
		history = DefaultHistory
	}
	history = min(history, MaxHistory)
	loc := location(org.Timezone)
	now := s.now()

	out := newSummary(p, org.Timezone, now)
	// O "desde o início" pede a história inteira da pessoa, e os períodos saem das mesmas sessões.
	sessions, err := s.deps.Sessions.ListByPersonSince(orgID, personID, time.Time{})
	if err != nil {
		return nil, err
	}
	var current map[uuid.UUID]*sum
	if p.Payment != nil {
		current = fill(out, p, sessions, now, history, loc)
	}
	all := lifetime(out, sessions, now, p.IsOwner, loc)
	if err := s.nameProjects(out, current, all, revenue); err != nil {
		return nil, err
	}
	if p.Payment != nil && out.Current != nil {
		if err := s.countTasks(out.Current, sessions, p, now, loc); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func newSummary(p *person.Person, tz string, now time.Time) *Summary {
	return &Summary{
		GeneratedAt: now, Timezone: tz, Owner: p.IsOwner, Rule: p.Payment,
		Upcoming: []PeriodView{}, History: []PeriodView{},
		Lifetime: Lifetime{Projects: []ProjectShare{}},
	}
}

// sinceFor é o começo do período mais antigo do histórico: de onde as sessões interessam.
func sinceFor(rule person.PaymentRule, now time.Time, history int, loc *time.Location) time.Time {
	cur, ok := PeriodAt(rule, now, loc)
	if !ok {
		return now
	}
	start := cur.Start
	for i := 0; i < history; i++ {
		prev, ok := Previous(cur)
		if !ok {
			break
		}
		cur, start = prev, prev.Start
	}
	return start
}

type sum struct {
	seconds float64
	cents   int
	money   bool
	// bill é o que os clientes pagam por esse tempo; billed diz se alguma sessão tinha valor cobrado.
	bill   int
	billed bool
}

// add soma um pedaço de sessão. Sem valor (o dono, ou sessão sem taxa), os centavos ficam de fora.
func (t *sum) add(seconds float64, pay, bill *int, owner bool) {
	t.seconds += seconds
	if !owner && pay != nil {
		t.cents += *pay
		t.money = true
	}
	if bill != nil {
		t.bill += *bill
		t.billed = true
	}
}

// addTo soma o pedaço na linha do projeto, criando-a na primeira vez.
func addTo(perProject map[uuid.UUID]*sum, projectID uuid.UUID, seconds float64, pay, bill *int, owner bool) {
	ps := perProject[projectID]
	if ps == nil {
		ps = &sum{}
		perProject[projectID] = ps
	}
	ps.add(seconds, pay, bill, owner)
}

// within soma o que as sessões têm em [from, to), no total e, com perProject, por projeto.
func within(sessions []work_session.WorkSession, from, to, now time.Time, owner bool, perProject map[uuid.UUID]*sum) sum {
	var total sum
	for i := range sessions {
		sec, pay, bill := sessions[i].WithinRange(from, to, now)
		if sec <= 0 {
			continue
		}
		total.add(sec, pay, bill, owner)
		if perProject != nil {
			addTo(perProject, sessions[i].ProjectID, sec, pay, bill, owner)
		}
	}
	return total
}

// lifetime preenche o "desde o início" e devolve o detalhe por projeto dele. Cada sessão entra inteira, com o valor
// arredondado uma vez, então o total bate com a soma das sessões que as telas dos projetos mostram.
func lifetime(out *Summary, sessions []work_session.WorkSession, now time.Time, owner bool, loc *time.Location) map[uuid.UUID]*sum {
	byProject := map[uuid.UUID]*sum{}
	var total sum
	var first time.Time
	for i := range sessions {
		if first.IsZero() || sessions[i].StartAt.Before(first) {
			first = sessions[i].StartAt
		}
		sec, pay, bill := sessions[i].Within(time.Time{}, now)
		if sec <= 0 {
			continue
		}
		total.add(sec, pay, bill, owner)
		addTo(byProject, sessions[i].ProjectID, sec, pay, bill, owner)
	}
	out.Lifetime.Seconds, out.Lifetime.AmountCents = total.seconds, total.amount(owner)
	if !first.IsZero() {
		out.Lifetime.FirstDay = first.In(loc).Format(time.DateOnly)
	}
	return byProject
}

func (t sum) amount(owner bool) *int {
	if owner {
		return nil
	}
	c := t.cents
	return &c
}

// fill preenche o corrente, os próximos, o histórico e o acumulado de s.Rule, e devolve o detalhe por projeto do corrente.
func fill(out *Summary, p *person.Person, sessions []work_session.WorkSession, now time.Time, history int, loc *time.Location) map[uuid.UUID]*sum {
	rule := *p.Payment
	owner := p.IsOwner
	var byProject map[uuid.UUID]*sum
	if cur, ok := PeriodAt(rule, now, loc); ok {
		byProject = map[uuid.UUID]*sum{}
		total := within(sessions, cur.Start, cur.End, now, owner, byProject)
		v := view(cur)
		v.Seconds, v.AmountCents = total.seconds, total.amount(owner)
		left := daysBetween(midnight(now, loc), cur.PayDate)
		v.DaysLeft = &left
		if p.WeeklyHours != nil && *p.WeeklyHours > 0 {
			goal := float64(*p.WeeklyHours) * 3600 * float64(cur.Days()) / 7
			v.GoalSeconds = &goal
		}
		out.Current = &v

		for h, prev := 0, cur; h < history; h++ {
			var ok bool
			if prev, ok = Previous(prev); !ok {
				break
			}
			t := within(sessions, prev.Start, prev.End, now, owner, nil)
			pv := view(prev)
			pv.Seconds, pv.AmountCents = t.seconds, t.amount(owner)
			out.History = append(out.History, pv)
			out.Totals.Seconds += t.seconds
		}
		if !owner {
			var cents int
			for _, h := range out.History {
				cents += *h.AmountCents
			}
			out.Totals.AmountCents = &cents
		}
	}
	for _, up := range Upcoming(rule, now, UpcomingCount, loc) {
		out.Upcoming = append(out.Upcoming, view(up))
	}
	return byProject
}

func view(p Period) PeriodView {
	return PeriodView{Start: p.StartDay(), PayDate: p.PayDay(), Days: p.Days()}
}

// nameProjects põe no período corrente e no "desde o início" o detalhe por projeto, com o nome de cada um, buscando
// cada projeto uma vez. Um projeto que sumiu no meio do caminho fica de fora, sem derrubar a resposta.
func (s *Service) nameProjects(out *Summary, current, all map[uuid.UUID]*sum, revenue bool) error {
	names := map[uuid.UUID]string{}
	for _, byProject := range []map[uuid.UUID]*sum{current, all} {
		for id := range byProject {
			if _, ok := names[id]; ok {
				continue
			}
			prj, err := s.deps.Projects.Get(id.String())
			if errors.Is(err, database.ErrNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			names[id] = prj.Name
		}
	}
	if out.Current != nil {
		out.Current.Projects = shares(current, names, revenue)
		if revenue {
			var bill int
			for _, t := range current {
				bill += t.bill
			}
			out.Current.RevenueCents = &bill
			if out.Current.AmountCents != nil {
				margin := bill - *out.Current.AmountCents
				out.Current.MarginCents = &margin
			}
		}
	}
	out.Lifetime.Projects = shares(all, names, false)
	return nil
}

// countTasks conta as tarefas em que a pessoa trabalhou dentro do período corrente (alguma sessão dela o tocou, com a
// tarefa dentro da sessão), pelo estado de hoje. Sem o serviço de tarefas, ou com uma tarefa que sumiu, a contagem
// segue sem ela.
func (s *Service) countTasks(cur *PeriodView, sessions []work_session.WorkSession, p *person.Person, now time.Time, loc *time.Location) error {
	period, ok := PeriodAt(*p.Payment, now, loc)
	if !ok || s.deps.Tasks == nil {
		return nil
	}
	seen := map[uuid.UUID]bool{}
	counts := TaskCounts{}
	for i := range sessions {
		if sec, _, _ := sessions[i].WithinRange(period.Start, period.End, now); sec <= 0 {
			continue
		}
		for _, st := range sessions[i].Tasks {
			if seen[st.TaskID] {
				continue
			}
			seen[st.TaskID] = true
			t, err := s.deps.Tasks.Get(st.TaskID.String())
			if errors.Is(err, database.ErrNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			counts.Worked++
			switch t.Status {
			case "in_progress":
				counts.InProgress++
			case "awaiting_closure":
				counts.AwaitingClosure++
			case "closed":
				counts.Closed++
			default:
				counts.Backlog++
			}
		}
	}
	cur.Tasks = &counts
	return nil
}

// shares monta o detalhe por projeto, do que teve mais tempo para o que teve menos.
func shares(byProject map[uuid.UUID]*sum, names map[uuid.UUID]string, revenue bool) []ProjectShare {
	list := []ProjectShare{}
	for id, t := range byProject {
		name, ok := names[id]
		if !ok {
			continue
		}
		share := ProjectShare{Project: Ref{ID: id, Name: name}, Seconds: t.seconds}
		if t.money {
			c := t.cents
			share.AmountCents = &c
		}
		if revenue {
			b := t.bill
			share.RevenueCents = &b
		}
		list = append(list, share)
	}
	slices.SortFunc(list, func(a, b ProjectShare) int {
		return cmp.Or(
			cmp.Compare(b.Seconds, a.Seconds),
			strings.Compare(a.Project.Name, b.Project.Name),
			strings.Compare(a.Project.ID.String(), b.Project.ID.String()),
		)
	})
	return list
}
