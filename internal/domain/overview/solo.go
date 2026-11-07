package overview

import (
	"cmp"
	"math"
	"slices"
	"time"

	"github.com/google/uuid"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"working-time-tracker/internal/database"
	"working-time-tracker/internal/validate"
)

// As janelas da tela inicial do modo solo. São de calendário, no fuso da organização: este mês, o mês passado e o ano
// corrente (de 1º de janeiro até o fim do mês corrente).
const (
	WindowMonth     = "month"
	WindowLastMonth = "last_month"
	WindowYear      = "year"
)

// SoloOverview é a tela inicial do modo solo: o que o dono trabalhou, recebeu e espera receber numa janela de
// calendário, no total e por cliente. Só lê o que os outros domínios guardam; não tem tabela própria.
//
// A Receita e a Receita projetada são duas contas de datas diferentes e não se misturam: a Receita é o que ENTROU na
// janela (a data do recebimento, received_on), e a projetada é o que se ESPERA dos períodos de cobrança que TERMINAM
// nela (period_end), confirmados ou não, pagos ou não.
type SoloOverview struct {
	// Window é a janela pedida (month, last_month ou year), e From e To os dias que ela cobre, YYYY-MM-DD no fuso da
	// organização, os dois inclusivos. A janela do ano vai até o último dia do mês corrente: assim ela sempre contém a
	// do mês, e os períodos que terminam em meses que ainda não chegaram não entram na conta.
	Window   string `json:"window"`
	From     string `json:"from"`
	To       string `json:"to"`
	Timezone string `json:"timezone"`
	Currency string `json:"currency"`
	// GeneratedAt é o instante da conta: a sessão aberta e o período em andamento foram medidos até aqui.
	GeneratedAt time.Time  `json:"generated_at"`
	Totals      SoloTotals `json:"totals"`
	// Customers são os clientes que têm horas, recebimento ou valor projetado na janela, em ordem de nome. Nunca por
	// horas nem por valor: o sistema não ranqueia.
	Customers []SoloCustomer `json:"customers"`
	// InternalSeconds são as horas dos projetos sem cliente (internos) na janela. Não têm receita: só o tempo.
	InternalSeconds float64 `json:"internal_seconds"`
}

// SoloTotals são os cartões da tela: tudo da organização na janela.
type SoloTotals struct {
	// Seconds é o tempo trabalhado na janela, de todas as sessões (as dos projetos internos também). A sessão que
	// atravessa a borda conta só o trecho de dentro.
	Seconds float64 `json:"seconds"`
	// ReceivedCents é a soma de received_cents dos recebíveis com received_on dentro da janela.
	ReceivedCents int `json:"received_cents"`
	// ProjectedCents é o que se espera dos períodos que terminam na janela: o valor do recebível quando ele existe, e
	// senão a proposta (no por hora, o período em andamento também; no mensal, a mensalidade).
	ProjectedCents int `json:"projected_cents"`
	// EffectiveRateCents é a Receita projetada dividida pelas horas da janela, nulo sem horas. BelowMinRate o compara
	// com o piso de valor por hora (validate.MinRateCents): é um aviso, não um erro.
	EffectiveRateCents *int `json:"effective_rate_cents"`
	BelowMinRate       bool `json:"below_min_rate"`
}

// SoloCustomer é a linha de um cliente na tabela da tela.
type SoloCustomer struct {
	Customer Ref `json:"customer"`
	// Kind é o modo de cobrança do cliente hoje: hourly ou monthly.
	Kind string `json:"kind"`
	// Seconds são as horas das sessões dos projetos do cliente na janela, também no mensal (o dono continua batendo
	// ponto nele).
	Seconds            float64 `json:"seconds"`
	ReceivedCents      int     `json:"received_cents"`
	ProjectedCents     int     `json:"projected_cents"`
	EffectiveRateCents *int    `json:"effective_rate_cents"`
	BelowMinRate       bool    `json:"below_min_rate"`
}

// soloWindow é a janela já resolvida: os dias (para comparar com as datas de calendário dos recebíveis) e os instantes
// [from, to) (para cortar as sessões).
type soloWindow struct {
	first, last string
	from, to    time.Time
}

// windowAt resolve a janela key para o instante now, no fuso loc. key já foi conferida por validWindow.
func windowAt(key string, now time.Time, loc *time.Location) soloWindow {
	year, month, _ := now.In(loc).Date()
	from := time.Date(year, month, 1, 0, 0, 0, 0, loc)
	to := time.Date(year, month+1, 1, 0, 0, 0, 0, loc)
	switch key {
	case WindowLastMonth:
		from, to = time.Date(year, month-1, 1, 0, 0, 0, 0, loc), from
	case WindowYear:
		from = time.Date(year, time.January, 1, 0, 0, 0, 0, loc)
	}
	return soloWindow{
		first: from.Format(time.DateOnly), last: to.AddDate(0, 0, -1).Format(time.DateOnly),
		from: from, to: to,
	}
}

func validWindow(key string) bool {
	return key == WindowMonth || key == WindowLastMonth || key == WindowYear
}

// has diz se o dia YYYY-MM-DD cai dentro da janela.
func (w soloWindow) has(day string) bool { return day >= w.first && day <= w.last }

// effectiveRate é o valor por hora que cents rendem em seconds: o valor sobre as horas, ao centavo. Nulo sem horas.
// below diz que ficou abaixo do piso de valor por hora, o que é um aviso e não um erro.
func effectiveRate(cents int, seconds float64) (rate *int, below bool) {
	if seconds <= 0 {
		return nil, false
	}
	r := int(math.Round(float64(cents) * 3600 / seconds))
	return &r, r < validate.MinRateCents
}

// Solo monta a tela inicial do modo solo da organização na janela dada (month, last_month ou year; vazio vale month).
// Quem chama garante que a organização é solo e que quem pede é o dono: a rota confere as duas coisas. Um ID malformado
// vale como "não encontrado".
//
// O dinheiro vem do domínio dos recebíveis, sem refazer a conta de período:
//   - Receita: o received_cents dos recebíveis com received_on na janela, pelo dia do recebimento. Uma cobrança de
//     janeiro recebida em 5 de fevereiro é Receita de fevereiro.
//   - Receita projetada: dos períodos que terminam na janela (period_end), o amount_cents do recebível quando ele
//     existe, e a proposta quando não. As propostas já excluem os períodos confirmados, então nada conta duas vezes.
//     Os tetos das propostas (por cliente por hora, 24 períodos fechados no ciclo mensal e 52 no de 14 dias; 60 por
//     cliente mensal) não chegam a valer: uma janela tem no máximo 12 períodos mensais de cada cliente, ou 26 fechados
//     do ciclo de 14 dias, os mais recentes.
func (s *Service) Solo(orgID, window string) (*SoloOverview, error) {
	if window == "" {
		window = WindowMonth
	}
	if !validWindow(window) {
		return nil, ErrInvalidWindow.With("field", "window")
	}
	if _, err := uuid.Parse(orgID); err != nil {
		return nil, database.ErrNotFound
	}
	org, err := s.deps.Orgs.Get(orgID)
	if err != nil {
		return nil, err
	}
	customers, err := s.deps.Customers.ListByOrg(orgID)
	if err != nil {
		return nil, err
	}
	projects, err := s.deps.Projects.ListByOrg(orgID, nil)
	if err != nil {
		return nil, err
	}
	receivables, err := s.deps.Receivables.Receivables(orgID)
	if err != nil {
		return nil, err
	}
	proposals, err := s.deps.Receivables.Proposals(orgID, true)
	if err != nil {
		return nil, err
	}
	// Um instante só para a resposta inteira: a janela e a sessão aberta são medidas até aqui.
	now := s.now()
	w := windowAt(window, now, location(org.Timezone))
	sessions, err := s.deps.Sessions.ListByOrganizationSince(orgID, w.from)
	if err != nil {
		return nil, err
	}

	rows := make(map[uuid.UUID]*SoloCustomer, len(customers))
	for _, c := range customers {
		rows[c.ID] = &SoloCustomer{Customer: Ref{ID: c.ID, Name: c.Name}, Kind: c.BillingMode}
	}
	o := &SoloOverview{
		Window: window, From: w.first, To: w.last, Timezone: org.Timezone, Currency: org.Currency,
		GeneratedAt: now, Customers: []SoloCustomer{},
	}

	// As horas: cada sessão cortada na janela, no cliente do projeto dela, ou nos internos se o projeto não tem cliente.
	customerOf := make(map[uuid.UUID]uuid.UUID, len(projects))
	for _, p := range projects {
		if p.Customer != nil {
			customerOf[p.ID] = p.Customer.ID
		}
	}
	for i := range sessions {
		seconds, _, _ := sessions[i].WithinRange(w.from, w.to, now)
		if seconds <= 0 {
			continue
		}
		o.Totals.Seconds += seconds
		if row := rows[customerOf[sessions[i].ProjectID]]; row != nil {
			row.Seconds += seconds
		} else {
			o.InternalSeconds += seconds
		}
	}

	// A Receita é pelo dia do recebimento, e a projetada pelo fim do período: duas datas, duas contas.
	for _, r := range receivables {
		row := rows[r.Customer.ID]
		if row == nil {
			continue
		}
		if r.ReceivedOn != nil && r.ReceivedCents != nil && w.has(*r.ReceivedOn) {
			row.ReceivedCents += *r.ReceivedCents
		}
		if w.has(r.PeriodEnd) {
			row.ProjectedCents += r.AmountCents
		}
	}
	for _, p := range proposals {
		if row := rows[p.Customer.ID]; row != nil && w.has(p.PeriodEnd) {
			row.ProjectedCents += p.CalculatedCents
		}
	}

	// A tabela traz quem tem alguma coisa na janela, em ordem de nome (o do cadastro, sem acento nem caixa).
	byName := collate.New(language.Und)
	for _, row := range rows {
		o.Totals.ReceivedCents += row.ReceivedCents
		o.Totals.ProjectedCents += row.ProjectedCents
		if row.Seconds <= 0 && row.ReceivedCents == 0 && row.ProjectedCents == 0 {
			continue
		}
		row.EffectiveRateCents, row.BelowMinRate = effectiveRate(row.ProjectedCents, row.Seconds)
		o.Customers = append(o.Customers, *row)
	}
	slices.SortFunc(o.Customers, func(a, b SoloCustomer) int {
		return cmp.Or(
			byName.CompareString(a.Customer.Name, b.Customer.Name),
			cmp.Compare(a.Customer.Name, b.Customer.Name),
			cmp.Compare(a.Customer.ID.String(), b.Customer.ID.String()),
		)
	})
	o.Totals.EffectiveRateCents, o.Totals.BelowMinRate = effectiveRate(o.Totals.ProjectedCents, o.Totals.Seconds)
	return o, nil
}
