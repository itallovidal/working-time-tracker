package overview

import (
	"cmp"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/database"
)

// OrgOverview é a visão geral da organização inteira, para admins: o tempo registrado e o
// que ele custou e rendeu, somados em todos os projetos, em três janelas, e quanto cada
// pessoa trabalhou. Só lê o que os outros domínios guardam; não tem tabela própria.
type OrgOverview struct {
	People   OrgPeople   `json:"people"`
	Projects OrgProjects `json:"projects"`
	Periods  Periods     `json:"periods"`
	// ByPerson traz todas as pessoas da organização, também as que ainda não bateram ponto,
	// da que mais trabalhou (no total) para a que menos. A soma de cada janela é igual à
	// soma das linhas dela.
	ByPerson []OrgPersonTotal `json:"by_person"`
	// GeneratedAt é o instante da conta: as sessões abertas e as janelas de 7 e 30 dias
	// foram medidas até aqui.
	GeneratedAt time.Time `json:"generated_at"`
}

type OrgPeople struct {
	Total int `json:"total"`
	// WorkingNow são as pessoas com uma sessão aberta, em qualquer projeto.
	WorkingNow int `json:"working_now"`
}

type OrgProjects struct {
	Total int `json:"total"`
}

// Periods são as três janelas da visão geral. As duas primeiras contam só o trecho de cada
// sessão que caiu dentro delas; a terceira, tudo.
type Periods struct {
	Last7Days  Period `json:"last_7_days"`
	Last30Days Period `json:"last_30_days"`
	AllTime    Period `json:"all_time"`
}

// Period é o que se trabalhou numa janela. Os valores seguem a regra do projeto: cada sessão
// arredondada ao centavo (a que atravessa a borda da janela, só no trecho de dentro), com
// nil quando nenhuma sessão tem aquele valor por hora.
type Period struct {
	// Seconds é o tempo de todos, e MySeconds a parte de quem abriu a página.
	Seconds   float64 `json:"seconds"`
	MySeconds float64 `json:"my_seconds"`
	// SessionCount são as sessões que têm algum tempo dentro da janela.
	SessionCount int   `json:"session_count"`
	Money        Money `json:"money"`
}

// OrgPersonTotal é o tempo de uma pessoa em cada janela e, se está com o ponto aberto, no
// que ela trabalha agora.
type OrgPersonTotal struct {
	Person     Person `json:"person"`
	WorkingNow bool   `json:"working_now"`
	// WorkingOn são as tarefas que a pessoa tem na sessão aberta neste instante, na ordem em
	// que entraram, cada uma com o projeto. Vem vazia (nunca nula) para quem não trabalha agora
	// e para quem abriu uma sessão que já ficou sem tarefa.
	WorkingOn         []WorkingOn `json:"working_on"`
	Last7DaysSeconds  float64     `json:"last_7_days_seconds"`
	Last30DaysSeconds float64     `json:"last_30_days_seconds"`
	TotalSeconds      float64     `json:"total_seconds"`
}

// Ref é um recurso com o nome, para a tela mostrar e levar até ele sem outra chamada.
type Ref struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// WorkingOn é uma tarefa em que alguém trabalha agora, com o projeto dela.
type WorkingOn struct {
	Task    Ref `json:"task"`
	Project Ref `json:"project"`
}

// Organization monta a visão geral da organização para quem a pede (viewerID, a pessoa
// logada, de quem vem o "meu tempo"). Um ID malformado vale como "não encontrado".
func (s *Service) Organization(orgID, viewerID string) (*OrgOverview, error) {
	if _, err := uuid.Parse(orgID); err != nil {
		return nil, database.ErrNotFound
	}
	viewer, err := uuid.Parse(viewerID)
	if err != nil {
		return nil, database.ErrNotFound
	}
	people, err := s.deps.People.ListByOrg(orgID)
	if err != nil {
		return nil, err
	}
	projects, err := s.deps.Projects.CountByOrg(orgID)
	if err != nil {
		return nil, err
	}
	sessions, err := s.deps.Sessions.ListByOrganization(orgID)
	if err != nil {
		return nil, err
	}
	// Um instante só para a resposta inteira, tirado depois de ler as sessões.
	now := s.now()

	totals := make([]OrgPersonTotal, len(people))
	byPerson := make(map[uuid.UUID]*OrgPersonTotal, len(people))
	for i, p := range people {
		totals[i].Person = Person{ID: p.ID, Name: p.Name}
		totals[i].WorkingOn = []WorkingOn{}
		byPerson[p.ID] = &totals[i]
	}
	o := &OrgOverview{
		People:      OrgPeople{Total: len(people)},
		Projects:    OrgProjects{Total: projects},
		ByPerson:    totals,
		GeneratedAt: now,
	}

	// Cada janela soma no período dela e no campo de tempo da pessoa que lhe corresponde.
	windows := []struct {
		period *Period
		since  time.Time
		person func(*OrgPersonTotal) *float64
	}{
		{&o.Periods.Last7Days, now.Add(-7 * day), func(p *OrgPersonTotal) *float64 { return &p.Last7DaysSeconds }},
		{&o.Periods.Last30Days, now.Add(-30 * day), func(p *OrgPersonTotal) *float64 { return &p.Last30DaysSeconds }},
		{&o.Periods.AllTime, time.Time{}, func(p *OrgPersonTotal) *float64 { return &p.TotalSeconds }},
	}
	for i := range sessions {
		sess := &sessions[i]
		pt := byPerson[sess.PersonID]
		for _, w := range windows {
			seconds, pay, bill := sess.Within(w.since, now)
			if seconds <= 0 {
				continue
			}
			w.period.Seconds += seconds
			w.period.SessionCount++
			w.period.Money.PayAmountCents = sum(w.period.Money.PayAmountCents, pay)
			w.period.Money.BillAmountCents = sum(w.period.Money.BillAmountCents, bill)
			if sess.PersonID == viewer {
				w.period.MySeconds += seconds
			}
			if pt != nil {
				*w.person(pt) += seconds
			}
		}
		// Uma pessoa só tem uma sessão aberta por vez, então cada uma conta uma vez.
		if sess.EndAt == nil && pt != nil {
			pt.WorkingNow = true
			o.People.WorkingNow++
		}
	}
	for _, w := range windows {
		w.period.Money.addMargin()
	}
	if o.People.WorkingNow > 0 {
		if err := s.addWorkingOn(orgID, byPerson); err != nil {
			return nil, err
		}
	}

	slices.SortFunc(o.ByPerson, func(a, b OrgPersonTotal) int {
		return cmp.Or(
			cmp.Compare(b.TotalSeconds, a.TotalSeconds),
			cmp.Compare(a.Person.Name, b.Person.Name),
			cmp.Compare(a.Person.ID.String(), b.Person.ID.String()),
		)
	})
	return o, nil
}

// addWorkingOn põe em cada pessoa com o ponto aberto as tarefas que estão na sessão dela agora
// (os intervalos sem fim), com o nome do projeto. Só olha as pessoas que a conta principal já
// marcou como trabalhando, para working_on nunca vir cheio sem working_now. Um projeto
// excluído no meio do caminho deixa de aparecer, sem derrubar a resposta.
func (s *Service) addWorkingOn(orgID string, byPerson map[uuid.UUID]*OrgPersonTotal) error {
	open, err := s.deps.Sessions.ListOpenByOrganization(orgID)
	if err != nil {
		return err
	}
	projects := map[uuid.UUID]*Ref{} // nil: o projeto não existe mais
	for i := range open {
		sess := &open[i]
		pt := byPerson[sess.PersonID]
		if pt == nil || !pt.WorkingNow {
			continue
		}
		project, seen := projects[sess.ProjectID]
		if !seen {
			p, err := s.deps.Projects.Get(sess.ProjectID.String())
			switch {
			case err == nil:
				project = &Ref{ID: p.ID, Name: p.Name}
			case !errors.Is(err, database.ErrNotFound):
				return err
			}
			projects[sess.ProjectID] = project
		}
		if project == nil {
			continue
		}
		for _, link := range sess.Tasks {
			if link.UntilAt != nil || link.Task == nil {
				continue
			}
			pt.WorkingOn = append(pt.WorkingOn, WorkingOn{
				Task:    Ref{ID: link.Task.ID, Name: link.Task.Name},
				Project: *project,
			})
		}
	}
	return nil
}

// addMargin calcula a margem: a receita menos o custo. Sem receita não há margem para mostrar.
func (m *Money) addMargin() {
	if m.BillAmountCents == nil {
		return
	}
	margin := *m.BillAmountCents
	if m.PayAmountCents != nil {
		margin -= *m.PayAmountCents
	}
	m.MarginCents = &margin
}
