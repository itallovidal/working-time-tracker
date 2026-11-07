package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/allocation"
	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/domain/customer"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/receivable"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/team"
	"working-time-tracker/internal/validate"
)

// A organização do modo solo: uma freelancer (PJ) com um ano de trabalho registrado, para a demonstração mostrar o
// modo solo cheio, como o de quem já usa o sistema há tempo. Vem na mesma execução da software house: o login dela
// está ao lado dos outros. Tem cinco clientes (três por hora e dois por contrato mensal, um deles já encerrado),
// projetos que começaram e terminaram ao longo do ano, sessões de doze meses (com duas semanas de férias) e o ponto
// aberto agora. Os recebíveis nascem do período em que o seed roda (nada tem data fixa), quase todos recebidos, e os
// mais recentes em estados variados: um recebido com multa e juros, um confirmado adiantado, um atrasado, um em aberto
// e as propostas ainda por confirmar.
const (
	soloOrgName  = "Marina Duarte Design"
	soloName     = "Marina Duarte"
	soloEmail    = "marina@example.com"
	soloTimezone = "America/Sao_Paulo" // o fuso do país padrão (BR), onde os períodos fecham

	// soloHistoryMonths são os meses de histórico antes do corrente: com o corrente, um ano. Assim as três janelas
	// da tela inicial (este mês, o mês passado e o ano) têm horas e dinheiro em qualquer dia em que o seed rode.
	soloHistoryMonths = 11
	// soloOpenMinutes é há quantos minutos a Marina está com o ponto aberto.
	soloOpenMinutes = 50
)

// soloCustomers são os clientes, com os termos de cada um em seedSolo. Os meses são contados a partir do corrente (0
// é este mês, -11 o primeiro do histórico).
var soloCustomers = []struct {
	key, name, document, contact, email, phone string
}{
	{"pixel", "Agência Pixel", "45.997.418/0001-53", "Camila Ribeiro", "camila@pixel.example", "+55 (11) 3003-5100"},
	{"aurora", "Clínica Aurora", "", "Dr. Paulo Menezes", "paulo@aurora.example", "+55 (48) 3003-7700"},
	{"horizonte", "Escola Horizonte", "08.765.432/0001-03", "Luciana Prado", "luciana@horizonte.example", "+55 (31) 3003-2200"},
	{"bambu", "Café Bambu", "31.456.782/0001-00", "Tiago Nogueira", "tiago@cafebambu.example", "+55 (41) 3003-8800"},
	{"ventania", "Editora Ventania", "27.318.904/0001-16", "Beatriz Lacerda", "beatriz@ventania.example", "+55 (21) 3003-4400"},
}

// soloHourlyRate é o valor por hora de cada cliente por hora, em centavos.
var soloHourlyRate = map[string]int{"pixel": 13500, "bambu": 12000, "ventania": 11000}

// soloHorizonteLastMonth é o último mês do contrato da Escola Horizonte: o contrato acabou há quatro meses.
const soloHorizonteLastMonth = -4

// soloProjects: extra é o acréscimo do projeto em centavos (Decisão 21), por hora no cliente por hora e por mês no
// mensal; from e until são os meses em que o projeto tem horas (os que terminam antes de 0 são trabalhos entregues);
// weight é o peso do projeto na divisão do dia de trabalho, entre os que estão em andamento no dia.
var soloProjects = []struct {
	key, customer, name, description string
	extra                            int
	from, until                      int
	weight                           float64
}{
	{"social", "pixel", "Campanha de redes sociais", "Posts, carrosséis e stories para a campanha de verão da agência.", 0, -11, -6, 0.30},
	{"landing", "pixel", "Landing pages de campanha", "Páginas de campanha com formulário de leads e medição de conversão.", 0, -8, 0, 0.25},
	{"identidade", "pixel", "Identidade visual da Pixel", "Marca, manual e papelaria para o rebranding da agência.", 1500, -5, 0, 0.30},
	{"suporte", "aurora", "Site e suporte mensal", "Manutenção do site, correções, backup e relatório mensal de acessos.", 40000, -11, 0, 0.15},
	{"agenda", "aurora", "Agendamento online", "Agenda de consultas no site, por especialidade, com lembrete por e-mail.", 0, -4, -2, 0.25},
	{"portal", "horizonte", "Portal da Escola Horizonte", "Site da escola com matrícula online, calendário e avisos para os pais.", 0, -11, soloHorizonteLastMonth, 0.25},
	{"loja", "bambu", "Loja virtual do Café Bambu", "Loja com assinatura de cafés, frete por CEP e cupom de primeira compra.", 0, -5, 0, 0.30},
	{"ebooks", "ventania", "Capas e diagramação de e-books", "Capas, miolo e EPUB da coleção de bolso da editora.", 0, -2, 0, 0.25},
	{"estudos", "", "Estudos e administração", "Propostas, contabilidade e estudos. Horas sem cliente.", 0, -11, 0, 0.10},
	{"portfolio", "", "Portfólio e site próprio", "O site novo da Marina, com os cases do ano.", 0, -3, 0, 0.08},
}

// soloIntegrations são de demonstração e sem credencial, como as da software house. Os itens das tarefas (link) usam a
// primeira integração do tipo no projeto.
var soloIntegrations = []struct {
	project, kind, name string
	metadata            map[string]interface{}
}{
	{"loja", "github", "Repositório da loja", map[string]interface{}{"repo": "marinaduarte/loja-cafe-bambu"}},
	{"landing", "trello", "Quadro das campanhas", map[string]interface{}{"api_key": "demo-key-pixel-0003", "board_id": "Px9Lm2Qr"}},
}

// Os status que o seed dá às tarefas (o pacote task só exporta o do backlog, o inicial).
const (
	statusInProgress      = "in_progress"
	statusAwaitingClosure = "awaiting_closure"
	statusClosed          = "closed"
)

// soloTasks: status, prazo em dias a partir de hoje e os itens das integrações (o formato de linkTask). Os projetos
// entregues têm todas as tarefas fechadas, com o prazo no mês em que terminaram.
var soloTasks = []struct {
	project, name, description, status string
	deadlineDays                       float64
	link                               string
}{
	{"social", "Calendário editorial do verão", "", statusClosed, -320, ""},
	{"social", "Carrosséis de lançamento", "Dez carrosséis com a nova linha de serviços da agência.", statusClosed, -290, ""},
	{"social", "Stories com enquete", "", statusClosed, -250, ""},
	{"social", "Relatório de alcance da campanha", "", statusClosed, -185, ""},

	{"landing", "Landing da campanha de inverno", "", statusClosed, -150, ""},
	{"landing", "Landing da campanha de fim de ano", "Layout responsivo e textos aprovados pela agência.", statusInProgress, 5, "tr:Lf8Qa2Wn"},
	{"landing", "Formulário de leads com validação", "", statusInProgress, 8, "tr:Fm3Zt7Kd"},
	{"landing", "Medição de conversão", "", statusClosed, -3, ""},
	{"landing", "Teste A/B do título", "", task.StatusBacklog, 15, "tr:Ab6Tx1Pq"},

	{"identidade", "Moodboard e referências", "", statusClosed, -130, ""},
	{"identidade", "Logotipo e variações", "Versões em cores, preto e branco e ícone.", statusInProgress, 3, ""},
	{"identidade", "Revisão final com a Pixel", "", statusAwaitingClosure, 2, ""},
	{"identidade", "Manual de marca em PDF", "", task.StatusBacklog, 12, ""},
	{"identidade", "Papelaria: cartão e papel timbrado", "", task.StatusBacklog, 20, ""},

	{"suporte", "Migração para a nova hospedagem", "", statusClosed, -280, ""},
	{"suporte", "Atualização mensal do site", "Plugins, conteúdo e checagem de links.", statusInProgress, 4, ""},
	{"suporte", "Correção de bugs reportados", "", statusInProgress, 1, ""},
	{"suporte", "Backup e monitoramento", "", statusClosed, -7, ""},
	{"suporte", "Relatório mensal de acessos", "", task.StatusBacklog, 10, ""},

	{"agenda", "Fluxo de agendamento por especialidade", "", statusClosed, -95, ""},
	{"agenda", "Confirmação e lembrete por e-mail", "", statusClosed, -75, ""},
	{"agenda", "Treinamento da recepção", "", statusClosed, -65, ""},

	{"portal", "Layout do portal e área dos pais", "", statusClosed, -300, ""},
	{"portal", "Formulário de matrícula online", "", statusClosed, -255, ""},
	{"portal", "Calendário escolar e avisos", "", statusClosed, -215, ""},
	{"portal", "Entrega dos acessos no fim do contrato", "", statusClosed, -125, ""},

	{"loja", "Catálogo de cafés e moagem", "", statusClosed, -120, "gh:3"},
	{"loja", "Frete por CEP", "", statusClosed, -40, "gh:9"},
	{"loja", "Assinatura de cafés com renovação mensal", "Planos de 250 g e 500 g, com pausa e troca de moagem.", statusInProgress, 6, "gh:12"},
	{"loja", "Cupom de primeira compra", "", statusAwaitingClosure, 1, "gh:15"},
	{"loja", "Página de origem dos grãos", "", task.StatusBacklog, 18, "gh:18"},
	{"loja", "Relatório de vendas mensal", "", task.StatusBacklog, 25, ""},

	{"ebooks", "Capa da coleção de bolso", "", statusClosed, -20, ""},
	{"ebooks", "Diagramação do volume 1", "", statusInProgress, 4, ""},
	{"ebooks", "EPUB com sumário navegável", "", statusInProgress, 9, ""},
	{"ebooks", "Diagramação do volume 2", "", task.StatusBacklog, 22, ""},

	{"estudos", "Curso de motion design", "", statusClosed, -90, ""},
	{"estudos", "Propostas e orçamentos", "", statusInProgress, 2, ""},
	{"estudos", "Enviar as notas do mês à contabilidade", "", statusAwaitingClosure, 6, ""},
	{"estudos", "Estudo de acessibilidade WCAG", "", task.StatusBacklog, 25, ""},

	{"portfolio", "Novo layout do portfólio", "", statusInProgress, 10, ""},
	{"portfolio", "Case da loja do Café Bambu", "", task.StatusBacklog, 30, ""},
}

// soloOpenTask é a tarefa em que a Marina está com o ponto aberto agora.
var soloOpenTask = struct{ project, task string }{"identidade", "Logotipo e variações"}

// soloSummary é o que main imprime da organização solo.
type soloSummary struct {
	customers, hourly, monthly int
	projects, internal, tasks  int
	integrations, sessions     int
	confirmed, received        int
	open, late, toConfirm      int
}

// soloTask é uma tarefa criada, com o prazo e o status, para escolher a de cada sessão.
type soloTask struct {
	id       uuid.UUID
	deadline time.Time
	status   string
}

// seedSolo cria a organização solo. Roda depois da software house, com os mesmos services, e não toca no sorteio
// dela: os sorteios daqui são outros, para a demonstração de empresa continuar dando sempre o mesmo banco.
func seedSolo(ctx context.Context, db *database.DB, now time.Time) soloSummary {
	authSvc := auth.NewService(auth.NewStore(db.Client))
	orgSvc := organization.NewService(organization.NewStore(db.Client))
	customerSvc := customer.NewService(customer.NewStore(db.Client))
	personSvc := person.NewService(person.NewStore(db.Client))
	allocationSvc := allocation.NewService(allocation.NewStore(db.Client))
	projectSvc := project.NewService(project.NewStore(db.Client))
	taskSvc := task.NewService(task.NewStore(db.Client), team.NewMembershipStore(db.Client), nil)
	receivableSvc := receivable.NewService(receivable.Deps{Store: receivable.NewStore(db.Client)})

	loc, err := time.LoadLocation(soloTimezone)
	if err != nil {
		loc = time.UTC
	}
	today := now.In(loc)
	// monthStart é o primeiro dia do mês offset (0 é o corrente); monthOf é o offset do mês de um dia.
	monthStart := func(offset int) time.Time {
		return time.Date(today.Year(), today.Month()+time.Month(offset), 1, 0, 0, 0, 0, loc)
	}
	monthOf := func(day time.Time) int {
		return (day.Year()-today.Year())*12 + int(day.Month()) - int(today.Month())
	}
	// O primeiro dia do histórico: de lá as sessões chegam a hoje, e os contratos mensais começam nele.
	first := monthStart(-soloHistoryMonths)

	text := func(v string) *string { return &v }
	number := func(v int) *int { return &v }
	money := func(v int) validate.Optional[int] { return validate.Optional[int]{Set: true, Value: &v} }

	// A organização e a pessoa: o dono, e a única. Marcada como solo já no cadastro (sem convites, sem equipe).
	owner, _, err := authSvc.Signup(auth.SignupInput{
		OrganizationName: soloOrgName, Name: soloName, Email: soloEmail, Password: password, Solo: true,
	})
	must(err)
	must(authSvc.CompleteOnboarding(owner))
	orgID := owner.OrganizationID.String()
	ownerID := owner.PersonID.String()
	_, err = orgSvc.Update(orgID, organization.UpdateInput{
		Summary:      text("Design e desenvolvimento web para agências, pequenas empresas e editoras."),
		Description:  text("Atendo sozinha, como PJ: identidade visual, sites, lojas virtuais e diagramação de livros, por hora ou por contrato mensal."),
		Website:      text("https://marinaduarte.example"),
		ContactEmail: text("contato@marinaduarte.example"),
		LegalName:    text("Marina Duarte Design ME"),
		CNPJ:         text("12.345.678/0001-95"),
		Country:      text("BR"),
		WorkMode:     text("remote"),
	})
	must(err)
	_, err = personSvc.SetWeeklyHours(ownerID, number(40))
	must(err)

	sum := soloSummary{customers: len(soloCustomers), projects: len(soloProjects), tasks: len(soloTasks), integrations: len(soloIntegrations)}

	// Os clientes, com os termos de cada um:
	//   - Agência Pixel, por hora: 135,00, fecha no dia 25 e paga em 10 dias.
	//   - Clínica Aurora, contrato mensal de 6.300,00 desde o começo do histórico, fechando no último dia do mês (o
	//     padrão), 5 dias para pagar, multa de 2%, juros de 1% ao mês e 3 dias de tolerância.
	//   - Escola Horizonte, contrato mensal de 3.800,00 que acabou há quatro meses, 10 dias para pagar e sem multa.
	//   - Café Bambu, por hora: 120,00, fecha no último dia do mês e paga em 15 dias.
	//   - Editora Ventania, por hora: 110,00, fecha no último dia do mês e paga em 30 dias.
	customerID := map[string]string{}
	for _, c := range soloCustomers {
		in := customer.Input{
			Name: text(c.name), Document: text(c.document), ContactName: text(c.contact), ContactEmail: text(c.email), ContactPhone: text(c.phone),
		}
		if rate, ok := soloHourlyRate[c.key]; ok {
			in.HourlyRateCents = money(rate)
			sum.hourly++
		} else {
			in.BillingMode = text(customer.BillingMonthly)
			in.ContractStart = text(first.Format(time.DateOnly))
			sum.monthly++
		}
		switch c.key {
		case "pixel":
			in.ClosingDay = number(25)
			in.PaymentDays = number(10)
		case "aurora":
			in.MonthlyFeeCents = money(630000)
			in.PaymentDays = number(5)
			in.LateFeeBP = number(200)
			in.LateInterestBPMonth = number(100)
			in.GraceDays = number(3)
		case "horizonte":
			in.MonthlyFeeCents = money(380000)
			in.ContractEnd = text(monthStart(soloHorizonteLastMonth+1).AddDate(0, 0, -1).Format(time.DateOnly))
			in.PaymentDays = number(10)
		case "bambu":
			in.ClosingDay = number(31)
			in.PaymentDays = number(15)
		case "ventania":
			in.ClosingDay = number(31)
			in.PaymentDays = number(30)
		}
		created, err := customerSvc.Create(orgID, in)
		must(err)
		customerID[c.key] = created.ID.String()
	}

	// Os projetos. A dona entra em todos com custo zero (o dono não é pago): o que ela tira deles é o que o cliente paga.
	type soloProject struct {
		id          string
		billRate    *int
		from, until int
		weight      float64
		tasks       []soloTask
	}
	prjs := map[string]*soloProject{}
	var order []string
	customerSince := map[string]time.Time{}
	for _, p := range soloProjects {
		// O valor cobrado que as sessões guardam: o do cliente por hora mais o acréscimo; nenhum no mensal e no interno.
		var billRate *int
		if rate, ok := soloHourlyRate[p.customer]; ok {
			billRate = number(rate + p.extra)
		}
		created, err := projectSvc.CreateWithCustomer(orgID, project.CreateInput{
			Name: p.name, Description: p.description, CustomerID: customerID[p.customer], BillExtraCents: p.extra,
		})
		must(err)
		id := created.ID.String()
		_, err = allocationSvc.Set(id, ownerID, 0)
		must(err)
		// O projeto nasce hoje; volta no tempo para começar antes da primeira sessão. O cliente, antes do primeiro projeto.
		since := monthStart(p.from).AddDate(0, 0, -5)
		_, err = db.Raw.ExecContext(ctx, `UPDATE projects SET created_at = $1 WHERE id = $2`, since, id)
		must(err)
		if p.customer != "" {
			if old, ok := customerSince[p.customer]; !ok || since.Before(old) {
				customerSince[p.customer] = since
			}
		} else {
			sum.internal++
		}
		prjs[p.key] = &soloProject{id: id, billRate: billRate, from: p.from, until: p.until, weight: p.weight}
		order = append(order, p.key)
	}
	for key, since := range customerSince {
		_, err = db.Raw.ExecContext(ctx, `UPDATE customers SET created_at = $1 WHERE id = $2`, since.AddDate(0, 0, -2), customerID[key])
		must(err)
	}

	// As integrações vão direto ao banco, como as da software house: o service validaria o token, e aqui não há.
	integrationOf := map[string]*ent.Integration{}
	for _, it := range soloIntegrations {
		created := db.Client.Integration.Create().
			SetProjectID(uuid.MustParse(prjs[it.project].id)).
			SetType(it.kind).
			SetDisplayName(it.name).
			SetMetadata(it.metadata).
			SetEnabled(true).
			SaveX(ctx)
		if _, ok := integrationOf[it.project+"/"+it.kind]; !ok {
			integrationOf[it.project+"/"+it.kind] = created
		}
	}

	// As tarefas, todas da dona, com prazo, status, as etiquetas que o nome pede e os itens das integrações.
	labelID := map[string]string{}
	var openTask uuid.UUID
	for _, t := range soloTasks {
		prj := prjs[t.project]
		deadline := today.Add(time.Duration(t.deadlineDays * 24 * float64(time.Hour)))
		var ids []string
		for _, l := range labelsFor(t.name) {
			if _, ok := labelID[t.project+"/"+l]; !ok {
				created, err := taskSvc.CreateLabel(prj.id, l)
				must(err)
				labelID[t.project+"/"+l] = created.ID.String()
			}
			ids = append(ids, labelID[t.project+"/"+l])
		}
		priority := priorityFor(t.deadlineDays)
		created, err := taskSvc.CreateAs(ownerID, prj.id, t.name, t.description, ownerID, &deadline,
			task.Attrs{Priority: &priority, LabelIDs: &ids})
		must(err)
		if t.status != task.StatusBacklog {
			status := t.status
			created, err = taskSvc.UpdateAs(ownerID, created.ID.String(), created.Name, created.Description, nil, nil, task.Attrs{Status: &status})
			must(err)
		}
		if t.link != "" {
			linkTask(ctx, db.Client, integrationOf, t.project, created.ID, t.link)
		}
		if t.project == soloOpenTask.project && t.name == soloOpenTask.task {
			openTask = created.ID
		}
		prj.tasks = append(prj.tasks, soloTask{id: created.ID, deadline: deadline, status: t.status})
	}
	if openTask == uuid.Nil {
		log.Fatalf("a tarefa do ponto aberto (%s) não está em soloTasks", soloOpenTask.task)
	}

	// As sessões, do começo do histórico até agora, de uma sequência fixa à parte. Guardam o valor cobrado do projeto
	// de quando o ponto abriu (nulo nos de cliente mensal e nos internos) e as horas da dona (sem custo).
	rng := rand.New(rand.NewSource(20261107))
	// Hoje só entram as sessões que terminaram antes do ponto aberto agora, com uma folga.
	todayLimit := today.Hour()*60 + today.Minute() - soloOpenMinutes - 10
	at := func(day time.Time, minutes int) time.Time {
		return time.Date(day.Year(), day.Month(), day.Day(), 0, minutes, 0, 0, loc)
	}
	// Duas semanas de férias, no meio do histórico.
	vacationFrom := monthStart(-7).AddDate(0, 0, 9)
	vacationUntil := vacationFrom.AddDate(0, 0, 14)
	// pick escolhe o projeto de um bloco de trabalho, pelo peso, entre os que estão em andamento no mês.
	pick := func(month int) *soloProject {
		var active []*soloProject
		total := 0.0
		for _, key := range order {
			if p := prjs[key]; p.from <= month && month <= p.until {
				active = append(active, p)
				total += p.weight
			}
		}
		r := rng.Float64() * total
		for _, p := range active {
			if r -= p.weight; r <= 0 {
				return p
			}
		}
		return active[len(active)-1]
	}
	// taskFor escolhe a tarefa de uma sessão: uma que já saiu do backlog e cujo prazo ainda não tinha passado (com uma
	// semana de folga) no dia; sem nenhuma assim, a de prazo mais tarde.
	taskFor := func(prj *soloProject, day time.Time) uuid.UUID {
		var open []uuid.UUID
		var latest *soloTask
		for i := range prj.tasks {
			t := &prj.tasks[i]
			if t.status == task.StatusBacklog {
				continue
			}
			if !t.deadline.Before(day.AddDate(0, 0, -7)) {
				open = append(open, t.id)
			}
			if latest == nil || t.deadline.After(latest.deadline) {
				latest = t
			}
		}
		if len(open) > 0 {
			return open[rng.Intn(len(open))]
		}
		if latest == nil {
			log.Fatalf("o projeto %s não tem tarefa fora do backlog para as sessões", prj.id)
		}
		return latest.id
	}
	var builders []*ent.WorkSessionCreate
	var links []uuid.UUID // a tarefa de cada sessão, na mesma ordem
	var spans [][2]time.Time
	for day := first; !day.After(today); day = day.AddDate(0, 0, 1) {
		if !day.Before(vacationFrom) && day.Before(vacationUntil) {
			continue
		}
		weekend := day.Weekday() == time.Saturday || day.Weekday() == time.Sunday
		if weekend && rng.Float64() > 0.06 {
			continue
		}
		if rng.Float64() < 0.07 { // folga, feriado ou dia de reunião fora
			continue
		}
		month := monthOf(day)
		isToday := day.Format(time.DateOnly) == today.Format(time.DateOnly)
		target := 5.5 + rng.Float64()*2.5 // horas do dia
		blocks := 2
		if rng.Float64() < 0.4 {
			blocks = 3
		}
		clockMin := 8*60 + 30 + rng.Intn(90)
		for b := 0; b < blocks; b++ {
			prj := pick(month)
			// Terça e quinta são os dias de suporte da Aurora, e quarta o da Escola Horizonte enquanto o contrato durou:
			// os contratos mensais têm horas em todo mês.
			if b == 0 {
				switch {
				case day.Weekday() == time.Tuesday || day.Weekday() == time.Thursday:
					prj = prjs["suporte"]
				case day.Weekday() == time.Wednesday && month <= soloHorizonteLastMonth:
					prj = prjs["portal"]
				}
			}
			length := int((target/float64(blocks))*(0.7+rng.Float64()*0.6)*60/5) * 5
			if length < 40 {
				length = 40
			}
			start, end := clockMin, clockMin+length
			if end > 21*60 || (isToday && end > todayLimit) {
				break
			}
			session := db.Client.WorkSession.Create().
				SetProjectID(uuid.MustParse(prj.id)).
				SetPersonID(owner.PersonID).
				SetStartAt(at(day, start)).
				SetEndAt(at(day, end)).
				SetPayRateCents(0).
				SetNillableBillRateCents(prj.billRate).
				SetOwnerHours(true)
			builders = append(builders, session)
			links = append(links, taskFor(prj, day))
			spans = append(spans, [2]time.Time{at(day, start), at(day, end)})
			clockMin = end + 40 + rng.Intn(60)
		}
	}
	for start := 0; start < len(builders); start += 200 {
		end := min(start+200, len(builders))
		created := db.Client.WorkSession.CreateBulk(builders[start:end]...).SaveX(ctx)
		var linkBuilders []*ent.WorkSessionTaskCreate
		for i, s := range created {
			linkBuilders = append(linkBuilders, db.Client.WorkSessionTask.Create().
				SetSessionID(s.ID).SetTaskID(links[start+i]).SetFromAt(spans[start+i][0]).SetUntilAt(spans[start+i][1]))
		}
		db.Client.WorkSessionTask.CreateBulk(linkBuilders...).ExecX(ctx)
	}
	sum.sessions = len(builders)

	// O ponto aberto agora, na identidade visual da Pixel.
	openPrj := prjs[soloOpenTask.project]
	startedAt := now.Add(-soloOpenMinutes * time.Minute)
	session := db.Client.WorkSession.Create().
		SetProjectID(uuid.MustParse(openPrj.id)).
		SetPersonID(owner.PersonID).
		SetStartAt(startedAt).
		SetPayRateCents(0).
		SetNillableBillRateCents(openPrj.billRate).
		SetOwnerHours(true).
		SaveX(ctx)
	db.Client.WorkSessionTask.Create().SetSessionID(session.ID).SetTaskID(openTask).SetFromAt(startedAt).ExecX(ctx)

	// Bater o ponto põe a tarefa em progresso, mas o seed grava as sessões direto no banco: a tarefa com tempo
	// registrado não fica no backlog.
	_, err = db.Raw.ExecContext(ctx, `UPDATE tasks SET status = 'in_progress'
		WHERE status = 'backlog' AND project_id IN (SELECT id FROM projects WHERE organization_id = $1)
		  AND id IN (SELECT task_id FROM work_session_tasks)`, orgID)
	must(err)

	seedReceivables(receivableSvc, orgID, customerID, today, &sum)
	return sum
}

// soloPlan é o que o seed faz com um período proposto: confirma com amount e note e, quando paidAfter não é negativo,
// recebe paidAfter dias depois do vencimento (ou hoje, se esse dia ainda não chegou). Um período sem plano fica como
// proposta por confirmar.
type soloPlan struct {
	p         receivable.Proposal
	amount    int
	note      string
	paidAfter int
}

// seedReceivables confirma e recebe os períodos que o sistema propõe para o período em que o seed roda, então os
// estados valem em qualquer dia. As notas fiscais seguem a ordem dos períodos, de todos os clientes juntos.
//
//   - Clínica Aurora (mensal, vence 5 dias depois de fechar o mês): os meses até o retrasado recebidos no prazo ou na
//     tolerância, menos o de quatro meses atrás, recebido 12 dias depois do vencimento, com a multa e os juros que o
//     sistema sugere; os seis mais antigos com a mensalidade de antes do reajuste anual; o mês passado recebido no
//     vencimento (ou hoje, se ele ainda não chegou); o corrente confirmado adiantado e em aberto.
//   - Escola Horizonte (mensal, encerrado): todos os meses recebidos, alguns com atraso e sem multa, como combinado.
//   - Agência Pixel (por hora, fecha no dia 25): os períodos recebidos, menos os dois últimos fechados: o penúltimo
//     confirmado e sem pagar (atrasado em qualquer data: vence 10 dias depois de fechar, mais de um mês atrás) e o
//     último por confirmar.
//   - Café Bambu (por hora, fecha no fim do mês): recebidos, menos o último fechado, por confirmar.
//   - Editora Ventania (por hora, fecha no fim do mês, 30 dias para pagar): o primeiro mês recebido e o último fechado
//     confirmado, ainda no prazo.
func seedReceivables(svc *receivable.Service, orgID string, customerID map[string]string, today time.Time, sum *soloSummary) {
	proposals, err := svc.Proposals(orgID, false)
	must(err)
	periodsOf := func(key string) []receivable.Proposal {
		var out []receivable.Proposal
		for _, p := range proposals {
			if p.Customer.ID.String() == customerID[key] {
				out = append(out, p)
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].PeriodEnd < out[j].PeriodEnd })
		return out
	}
	// delay devolve os dias de atraso do i-ésimo pagamento, repetindo a lista.
	delay := func(list []int, i int) int { return list[i%len(list)] }

	var plans []soloPlan
	aurora := periodsOf("aurora")
	if len(aurora) != soloHistoryMonths+1 {
		log.Fatalf("o contrato da Aurora devia propor %d períodos, propôs %d", soloHistoryMonths+1, len(aurora))
	}
	raise := len(aurora) - 6 // o primeiro mês com o reajuste
	for i, p := range aurora {
		plan := soloPlan{p: p, amount: p.CalculatedCents, paidAfter: delay([]int{0, 1, 0, 2, 0, 0, 1}, i)}
		switch {
		case i < raise:
			plan.amount -= 40000
		case i == raise:
			plan.note = "Primeira mensalidade com o reajuste anual."
		}
		switch i {
		case len(aurora) - 5:
			plan.paidAfter = 12
			plan.note = "Pagou com atraso, com a multa e os juros do contrato."
		case len(aurora) - 2:
			plan.paidAfter = 0
		case len(aurora) - 1:
			plan.paidAfter = -1
			plan.note = "Cobrança adiantada, a pedido da clínica."
		}
		plans = append(plans, plan)
	}

	horizonte := periodsOf("horizonte")
	for i, p := range horizonte {
		plan := soloPlan{p: p, amount: p.CalculatedCents, paidAfter: delay([]int{0, 3, 1, 0, 6, 2, 0, 4}, i)}
		if i == len(horizonte)-1 {
			plan.note = "Último mês do contrato."
		}
		plans = append(plans, plan)
	}

	pixel := periodsOf("pixel")
	if len(pixel) < 3 {
		log.Fatalf("a Pixel devia ter ao menos 3 períodos fechados, tem %d", len(pixel))
	}
	var latePixel receivable.Proposal
	for i, p := range pixel[:len(pixel)-1] {
		plan := soloPlan{p: p, amount: p.CalculatedCents, paidAfter: delay([]int{2, 0, 5, 1, 0, 3, 0, 1, 4, 0}, i)}
		if i == len(pixel)-2 {
			// O valor fechado em reais inteiros, combinado por e-mail; o que o sistema calculou fica guardado no recebível.
			plan.amount = p.CalculatedCents / 100 * 100
			plan.note = "Cliente pediu mais prazo por e-mail."
			plan.paidAfter = -1
			latePixel = p
		}
		plans = append(plans, plan)
	}

	bambu := periodsOf("bambu")
	for i, p := range bambu[:max(len(bambu)-1, 0)] {
		plans = append(plans, soloPlan{p: p, amount: p.CalculatedCents, paidAfter: delay([]int{0, 1, 0, 2, 0}, i)})
	}

	ventania := periodsOf("ventania")
	for i, p := range ventania {
		plan := soloPlan{p: p, amount: p.CalculatedCents, paidAfter: 0}
		if i == len(ventania)-1 {
			plan.paidAfter = -1
		}
		plans = append(plans, plan)
	}

	sort.SliceStable(plans, func(i, j int) bool {
		if plans[i].p.PeriodEnd != plans[j].p.PeriodEnd {
			return plans[i].p.PeriodEnd < plans[j].p.PeriodEnd
		}
		return plans[i].p.Customer.Name < plans[j].p.Customer.Name
	})
	day := today.Format(time.DateOnly)
	for i, plan := range plans {
		p := plan.p
		r, err := svc.Confirm(orgID, receivable.ConfirmInput{
			CustomerID: p.Customer.ID.String(), PeriodStart: p.PeriodStart, PeriodEnd: p.PeriodEnd,
			AmountCents: &plan.amount, InvoiceNumber: fmt.Sprintf("NF %d", 101+i), Note: plan.note,
		})
		must(err)
		if plan.paidAfter < 0 {
			continue
		}
		due, err := time.Parse(time.DateOnly, r.DueDate)
		must(err)
		on := due.AddDate(0, 0, plan.paidAfter).Format(time.DateOnly)
		if on > day {
			on = day
		}
		in := receivable.ReceiveInput{ReceivedOn: on}
		// O mensal recebe com a multa e os juros que o sistema sugere para o dia (zero dentro do prazo e da
		// tolerância); o por hora recebe exatamente o confirmado.
		if r.Kind == receivable.KindMonthly {
			c, err := svc.LateCharges(orgID, r.ID.String(), on)
			must(err)
			total := r.AmountCents + c.LateFeeCents + c.InterestCents
			in.ReceivedCents, in.LateFeeCents, in.InterestCents = &total, &c.LateFeeCents, &c.InterestCents
		}
		_, err = svc.Receive(orgID, r.ID.String(), in)
		must(err)
	}

	list, err := svc.Receivables(orgID)
	must(err)
	for _, r := range list {
		sum.confirmed++
		switch {
		case r.ReceivedOn != nil:
			sum.received++
		case r.Late:
			sum.late++
			if r.PeriodEnd == latePixel.PeriodEnd && r.Customer.ID.String() == customerID["pixel"] {
				latePixel = receivable.Proposal{}
			}
		default:
			sum.open++
		}
	}
	if latePixel.PeriodEnd != "" {
		log.Fatalf("o recebível da Pixel de %s devia estar atrasado (hoje é %s)", latePixel.PeriodEnd, day)
	}
	left, err := svc.Proposals(orgID, false)
	must(err)
	sum.toConfirm = len(left)
}
