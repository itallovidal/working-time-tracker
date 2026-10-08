package overview

import (
	"cmp"
	"math"
	"slices"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/collaborator"
	"working-time-tracker/internal/domain/integration"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/team"
	"working-time-tracker/internal/domain/work_session"
)

const day = 24 * time.Hour

// Deps são os domínios de onde a visão geral lê.
type Deps struct {
	Projects      *project.Service
	Collaborators *collaborator.Service
	Teams         *team.Service
	Sessions      *work_session.Service
	Integrations  *integration.Service
	Tasks         *task.Store
	// People serve à visão geral da organização, que lista todas as pessoas dela.
	People *person.Service
	// Now é o relógio da conta; sem ele vale time.Now. Os testes fixam o instante para as
	// janelas de 7 e 30 dias e as sessões abertas darem sempre o mesmo número.
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

// Get monta a visão geral do projeto. Um ID malformado vale como "não encontrado".
func (s *Service) Get(projectID string) (*Overview, error) {
	if _, err := uuid.Parse(projectID); err != nil {
		return nil, database.ErrNotFound
	}
	p, err := s.deps.Projects.Get(projectID)
	if err != nil {
		return nil, err
	}
	billing, err := s.deps.Projects.Billing(projectID)
	if err != nil {
		return nil, err
	}
	collaborators, err := s.deps.Collaborators.ListByProject(projectID)
	if err != nil {
		return nil, err
	}
	teams, err := s.deps.Teams.ListByProject(projectID)
	if err != nil {
		return nil, err
	}
	integrations, err := s.deps.Integrations.ListByProject(projectID)
	if err != nil {
		return nil, err
	}
	sessions, err := s.deps.Sessions.ListByProject(projectID, nil, nil)
	if err != nil {
		return nil, err
	}
	// Um instante só para a resposta inteira, tirado depois de ler as sessões: os
	// valores das sessões abertas já vieram calculados até a leitura.
	now := s.now()
	overdue, err := s.deps.Tasks.CountByProject(projectID, task.ListFilter{DeadlineTo: &now})
	if err != nil {
		return nil, err
	}

	o := &Overview{
		Project: Project{
			ID:            p.ID,
			Name:          p.Name,
			CreatedAt:     p.CreatedAt,
			Age:           ageBetween(p.CreatedAt, now),
			Customer:      billing.Customer,
			BillRateCents: billing.BillRateCents,
		},
		People:       People{Total: len(collaborators)},
		Teams:        Teams{Total: len(teams)},
		Tasks:        Tasks{Total: p.TaskCount, Overdue: overdue},
		Integrations: Integrations{Total: len(integrations), Items: make([]IntegrationItem, 0, len(integrations))},
		GeneratedAt:  now,
	}

	inProject := make(map[uuid.UUID]bool, len(collaborators))
	for _, c := range collaborators {
		inProject[c.Person.ID] = true
		if len(c.Teams) == 0 {
			o.People.WithoutTeam++
		}
		if c.PayRateCents == nil {
			o.People.WithoutRate++
		}
	}
	// A integração vai campo a campo: o que não está em IntegrationItem não sai.
	for _, it := range integrations {
		if it.Enabled {
			o.Integrations.Enabled++
		}
		o.Integrations.Items = append(o.Integrations.Items, IntegrationItem{
			ID:          it.ID,
			Type:        it.Type,
			DisplayName: it.DisplayName,
			Enabled:     it.Enabled,
			HasToken:    it.HasToken,
			CreatedAt:   it.CreatedAt,

			SyncIssues:    it.SyncIssues,
			LastSyncedAt:  it.LastSyncedAt,
			SyncUnmatched: it.SyncUnmatched,
		})
	}
	o.addSessions(sessions, inProject, now)
	return o, nil
}

// addSessions soma as sessões do projeto: o tempo, o que ele custou e rendeu, e
// a parte de cada pessoa. Os valores somam o de cada sessão, já arredondado,
// para o total bater com as linhas da aba Ponto. A sessão aberta conta até now.
func (o *Overview) addSessions(sessions []work_session.WorkSession, inProject map[uuid.UUID]bool, now time.Time) {
	byPerson := map[uuid.UUID]*PersonTotal{}
	for i := range sessions {
		s := &sessions[i]
		seconds := s.Seconds(now)

		o.Time.TotalSeconds += seconds
		o.Time.SessionCount++
		o.Time.Last7DaysSeconds += within(s, now.Add(-7*day), now)
		o.Time.Last30DaysSeconds += within(s, now.Add(-30*day), now)
		if o.Time.FirstSessionAt == nil || s.StartAt.Before(*o.Time.FirstSessionAt) {
			o.Time.FirstSessionAt = &s.StartAt
		}
		if o.Time.LastSessionAt == nil || s.StartAt.After(*o.Time.LastSessionAt) {
			o.Time.LastSessionAt = &s.StartAt
		}
		o.Money.PayAmountCents = sum(o.Money.PayAmountCents, s.PayAmountCents)
		o.Money.BillAmountCents = sum(o.Money.BillAmountCents, s.BillAmountCents)

		pt := byPerson[s.PersonID]
		if pt == nil {
			pt = &PersonTotal{Person: Person{ID: s.PersonID}, InProject: inProject[s.PersonID]}
			if s.Person != nil {
				pt.Person.Name = s.Person.Name
			}
			byPerson[s.PersonID] = pt
		}
		pt.TotalSeconds += seconds
		pt.SessionCount++
		pt.PayAmountCents = sum(pt.PayAmountCents, s.PayAmountCents)
		pt.BillAmountCents = sum(pt.BillAmountCents, s.BillAmountCents)
		// Uma pessoa só tem uma sessão aberta por vez, então cada uma conta uma vez.
		if s.EndAt == nil {
			pt.WorkingNow = true
			o.People.WorkingNow++
		}
	}

	o.Money.addMargin()

	o.ByPerson = make([]PersonTotal, 0, len(byPerson))
	for _, pt := range byPerson {
		o.ByPerson = append(o.ByPerson, *pt)
	}
	slices.SortFunc(o.ByPerson, func(a, b PersonTotal) int {
		return cmp.Or(
			cmp.Compare(b.TotalSeconds, a.TotalSeconds),
			cmp.Compare(a.Person.Name, b.Person.Name),
			cmp.Compare(a.Person.ID.String(), b.Person.ID.String()),
		)
	})
}

// within é quanto da sessão caiu de since até now.
func within(s *work_session.WorkSession, since, now time.Time) float64 {
	start, end := s.StartAt, now
	if s.EndAt != nil {
		end = *s.EndAt
	}
	if start.Before(since) {
		start = since
	}
	return math.Max(0, end.Sub(start).Seconds())
}

// sum soma dois valores opcionais: nil só quando os dois são nil.
func sum(total, v *int) *int {
	if v == nil {
		return total
	}
	n := *v
	if total != nil {
		n += *total
	}
	return &n
}

// ageBetween é há quanto tempo o projeto existe. Dias e semanas contam o tempo
// corrido; meses, os de calendário já completos: de 15 de janeiro a 14 de
// fevereiro ainda são zero, e no dia 15 é um. Quando o mês de agora é mais curto
// que o dia do início (31 de janeiro visto em fevereiro), o mês se completa no
// último dia dele. Um início no futuro vale zero.
func ageBetween(start, now time.Time) Age {
	if !now.After(start) {
		return Age{}
	}
	days := int(now.Sub(start) / day)

	start, now = start.UTC(), now.UTC()
	months := (now.Year()-start.Year())*12 + int(now.Month()) - int(start.Month())
	lastDay := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	anniversary := time.Date(now.Year(), now.Month(), min(start.Day(), lastDay),
		start.Hour(), start.Minute(), start.Second(), start.Nanosecond(), time.UTC)
	if now.Before(anniversary) {
		months--
	}
	return Age{Days: days, Weeks: days / 7, Months: months}
}
