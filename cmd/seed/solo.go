package main

import (
	"context"
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

// A organização do modo solo: uma freelancer (PJ) que atende dois clientes, um cobrado por hora e outro por contrato
// mensal. Vem na mesma execução da software house, para a demonstração mostrar os dois jeitos de usar o sistema: o
// login dela está ao lado dos outros. As sessões vão do começo do mês retrasado até hoje, e os recebíveis nascem do
// período em que o seed roda (nada tem data fixa), em estados variados: um recebido com multa e juros, um recebido no
// mês, um confirmado adiantado, um atrasado e as propostas ainda por confirmar.
const (
	soloOrgName  = "Marina Duarte Design"
	soloName     = "Marina Duarte"
	soloEmail    = "marina@example.com"
	soloTimezone = "America/Sao_Paulo" // o fuso do país padrão (BR), onde os períodos fecham
)

// soloCustomers são os dois clientes: o primeiro por hora, o segundo por contrato mensal. Os termos do por hora são o
// valor padrão (copiado para os projetos), o dia de fechamento e o prazo de pagamento; os do mensal, a mensalidade e a
// vigência, o prazo, a multa de 2%, os juros de 1% ao mês e a tolerância de 3 dias.
var soloCustomers = []struct {
	key, name, document, contact, email, phone string
}{
	{"pixel", "Agência Pixel", "45.997.418/0001-53", "Camila Ribeiro", "camila@pixel.example", "+55 (11) 3003-5100"},
	{"aurora", "Clínica Aurora", "", "Dr. Paulo Menezes", "paulo@aurora.example", "+55 (48) 3003-7700"},
}

// soloProjects: a Pixel (por hora) tem dois projetos, cada um com o seu valor cobrado (o primeiro é o valor padrão do
// cliente, copiado); a Aurora (mensal) tem um, sem valor por hora; e o último é interno. weight é o peso do projeto
// na divisão do dia de trabalho.
var soloProjects = []struct {
	key, customer, name, description string
	billRate                         int // centavos por hora; zero: sem valor (cliente mensal ou interno)
	weight                           float64
}{
	{"identidade", "pixel", "Identidade visual da Pixel", "Marca, manual e papelaria para o rebranding da agência.", 15000, 0.30},
	{"landing", "pixel", "Landing pages de campanha", "Páginas de campanha com formulário de leads e medição de conversão.", 13500, 0.25},
	{"suporte", "aurora", "Site e suporte mensal", "Manutenção do site, correções, backup e relatório mensal de acessos.", 0, 0.30},
	{"estudos", "", "Estudos e administração", "Propostas, contabilidade e estudos. Horas sem cliente.", 0, 0.15},
}

// Os status que o seed dá às tarefas (o pacote task só exporta o do backlog, o inicial).
const (
	statusInProgress      = "in_progress"
	statusAwaitingClosure = "awaiting_closure"
	statusClosed          = "closed"
)

// soloTasks: status e prazo em dias a partir de hoje.
var soloTasks = []struct {
	project, name, description, status string
	deadlineDays                       float64
}{
	{"identidade", "Moodboard e referências", "", statusClosed, -20},
	{"identidade", "Logotipo e variações", "Versões em cores, preto e branco e ícone.", statusInProgress, 3},
	{"identidade", "Revisão final com a Pixel", "", statusAwaitingClosure, 2},
	{"identidade", "Manual de marca em PDF", "", task.StatusBacklog, 12},
	{"identidade", "Papelaria: cartão e papel timbrado", "", task.StatusBacklog, 20},
	{"landing", "Landing da campanha de fim de ano", "Layout responsivo e textos aprovados pela agência.", statusInProgress, 5},
	{"landing", "Formulário de leads com validação", "", statusInProgress, 8},
	{"landing", "Teste A/B do título", "", task.StatusBacklog, 15},
	{"landing", "Medição de conversão", "", statusClosed, -3},
	{"suporte", "Atualização mensal do site", "Plugins, conteúdo e checagem de links.", statusInProgress, 4},
	{"suporte", "Correção de bugs reportados", "", statusInProgress, 1},
	{"suporte", "Backup e monitoramento", "", statusClosed, -7},
	{"suporte", "Relatório mensal de acessos", "", task.StatusBacklog, 10},
	{"estudos", "Propostas e orçamentos", "", statusInProgress, 2},
	{"estudos", "Estudo de acessibilidade WCAG", "", task.StatusBacklog, 25},
	{"estudos", "Enviar as notas do mês à contabilidade", "", statusAwaitingClosure, 6},
	{"estudos", "Portfólio: novo case", "", task.StatusBacklog, 30},
}

// soloSummary é o que main imprime da organização solo.
type soloSummary struct {
	sessions                  int
	projects, tasks           int
	confirmed, received, late int
	toConfirm                 int
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
	// O primeiro dia do mês retrasado: de lá as sessões chegam a hoje, e o contrato mensal começa nele.
	first := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, loc).AddDate(0, -2, 0)

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
		Summary:      text("Design e desenvolvimento web para agências e pequenas empresas."),
		Description:  text("Atendo sozinha, como PJ: identidade visual, landing pages e manutenção de sites, por hora ou por contrato mensal."),
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

	// Os clientes. O por hora leva o valor padrão, o fechamento no dia 25 e 10 dias para pagar; o mensal, a mensalidade
	// de 6.300,00 desde o primeiro dia do mês retrasado, fechando no último dia do mês (o padrão), 5 dias para pagar,
	// multa de 2%, juros de 1% ao mês e 3 dias de tolerância.
	customerID := map[string]string{}
	for _, c := range soloCustomers {
		in := customer.Input{
			Name: text(c.name), Document: text(c.document), ContactName: text(c.contact), ContactEmail: text(c.email), ContactPhone: text(c.phone),
		}
		switch c.key {
		case "pixel":
			in.DefaultRateCents = money(15000)
			in.ClosingDay = number(25)
			in.PaymentDays = number(10)
		case "aurora":
			in.BillingMode = text(customer.BillingMonthly)
			in.MonthlyFeeCents = money(630000)
			in.ContractStart = text(first.Format(time.DateOnly))
			in.PaymentDays = number(5)
			in.LateFeeBP = number(200)
			in.LateInterestBPMonth = number(100)
			in.GraceDays = number(3)
		}
		created, err := customerSvc.Create(orgID, in)
		must(err)
		customerID[c.key] = created.ID.String()
	}

	// Os projetos. A dona entra em todos com custo zero (o dono não é pago): o que ela tira deles é o que o cliente paga.
	type soloProject struct {
		id       string
		billRate *int
		weight   float64
		tasks    []*task.Task
	}
	prjs := map[string]*soloProject{}
	var order []string
	for _, p := range soloProjects {
		var billRate *int
		if p.billRate > 0 {
			billRate = number(p.billRate)
		}
		created, err := projectSvc.CreateWithCustomer(orgID, project.CreateInput{
			Name: p.name, Description: p.description, CustomerID: customerID[p.customer], BillRateCents: billRate,
		})
		must(err)
		id := created.ID.String()
		_, err = allocationSvc.Set(id, ownerID, 0)
		must(err)
		// O projeto nasce hoje; volta no tempo para começar antes da primeira sessão.
		_, err = db.Raw.ExecContext(ctx, `UPDATE projects SET created_at = $1 WHERE id = $2`, first.AddDate(0, 0, -5), id)
		must(err)
		prjs[p.key] = &soloProject{id: id, billRate: billRate, weight: p.weight}
		order = append(order, p.key)
	}

	// As tarefas, todas da dona, com prazo, status e as etiquetas que o nome pede.
	labelID := map[string]string{}
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
		prj.tasks = append(prj.tasks, created)
	}

	// As sessões, do primeiro dia do mês retrasado até agora, de uma sequência fixa à parte. Guardam o valor cobrado do
	// projeto de quando o ponto abriu (nulo nos de cliente mensal e no interno) e as horas da dona (sem custo).
	rng := rand.New(rand.NewSource(20261107))
	nowMinute := today.Hour()*60 + today.Minute()
	at := func(day time.Time, minutes int) time.Time {
		return time.Date(day.Year(), day.Month(), day.Day(), 0, minutes, 0, 0, loc)
	}
	pick := func() *soloProject { // o projeto de um bloco de trabalho, pelo peso
		r := rng.Float64()
		for _, key := range order {
			if r -= prjs[key].weight; r <= 0 {
				return prjs[key]
			}
		}
		return prjs[order[0]]
	}
	var builders []*ent.WorkSessionCreate
	var links []uuid.UUID // a tarefa de cada sessão, na mesma ordem
	var spans [][2]time.Time
	for day := first; !day.After(today); day = day.AddDate(0, 0, 1) {
		weekend := day.Weekday() == time.Saturday || day.Weekday() == time.Sunday
		if weekend && rng.Float64() > 0.06 {
			continue
		}
		if rng.Float64() < 0.07 { // folga, feriado ou dia de reunião fora
			continue
		}
		target := 5.5 + rng.Float64()*2.5 // horas do dia
		blocks := 2
		if rng.Float64() < 0.4 {
			blocks = 3
		}
		clockMin := 8*60 + 30 + rng.Intn(90)
		for b := 0; b < blocks; b++ {
			prj := pick()
			// Terça e quinta são os dias de suporte da Aurora: o contrato mensal tem horas em todo mês.
			if b == 0 && (day.Weekday() == time.Tuesday || day.Weekday() == time.Thursday) {
				prj = prjs["suporte"]
			}
			length := int((target/float64(blocks))*(0.7+rng.Float64()*0.6)*60/5) * 5
			if length < 40 {
				length = 40
			}
			start, end := clockMin, clockMin+length
			if end > 21*60 || (day.Format(time.DateOnly) == today.Format(time.DateOnly) && end > nowMinute-15) {
				break // hoje só entra o que já terminou
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
			links = append(links, prj.tasks[rng.Intn(len(prj.tasks))].ID)
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
	// Bater o ponto põe a tarefa em progresso, mas o seed grava as sessões direto no banco: a tarefa com tempo
	// registrado não fica no backlog.
	_, err = db.Raw.ExecContext(ctx, `UPDATE tasks SET status = 'in_progress'
		WHERE status = 'backlog' AND project_id IN (SELECT id FROM projects WHERE organization_id = $1)
		  AND id IN (SELECT task_id FROM work_session_tasks)`, orgID)
	must(err)

	sum := soloSummary{sessions: len(builders), projects: len(soloProjects), tasks: len(soloTasks)}
	seedReceivables(receivableSvc, orgID, customerID, today, &sum)
	return sum
}

// seedReceivables confirma e recebe alguns recebíveis, deixando propostas por confirmar. Parte das propostas que o
// sistema calcula do período em que o seed roda, então os estados valem em qualquer dia:
//
//   - Clínica Aurora (mensal, vence 5 dias depois de fechar o mês): o mês retrasado, recebido 12 dias depois do
//     vencimento, com a multa e os juros que o sistema sugere; o mês passado, recebido no vencimento (ou hoje, se ele
//     ainda não chegou); o mês corrente, confirmado adiantado e em aberto; o seguinte, só proposta.
//   - Agência Pixel (por hora, fecha no dia 25): o período fechado mais antigo está confirmado e sem pagar, atrasado em
//     qualquer data (vence 10 dias depois de fechar, e os fechados são de mais de um mês atrás); os outros, propostas.
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
	day := today.Format(time.DateOnly)
	earlier := func(a, b string) string { // o menor de dois dias YYYY-MM-DD
		if a < b {
			return a
		}
		return b
	}
	confirm := func(p receivable.Proposal, amount int, invoice, note string) *receivable.Receivable {
		r, err := svc.Confirm(orgID, receivable.ConfirmInput{
			CustomerID: p.Customer.ID.String(), PeriodStart: p.PeriodStart, PeriodEnd: p.PeriodEnd,
			AmountCents: &amount, InvoiceNumber: invoice, Note: note,
		})
		must(err)
		sum.confirmed++
		return r
	}
	// receive registra o recebimento no dia on, com a multa e os juros que o sistema sugere para ele (zero quando o dia
	// não passa do vencimento e da tolerância).
	receive := func(r *receivable.Receivable, on string) {
		c, err := svc.LateCharges(orgID, r.ID.String(), on)
		must(err)
		total := r.AmountCents + c.LateFeeCents + c.InterestCents
		_, err = svc.Receive(orgID, r.ID.String(), receivable.ReceiveInput{
			ReceivedOn: on, ReceivedCents: &total, LateFeeCents: &c.LateFeeCents, InterestCents: &c.InterestCents,
		})
		must(err)
		sum.received++
	}
	plusDays := func(d string, n int) string {
		t, err := time.Parse(time.DateOnly, d)
		must(err)
		return t.AddDate(0, 0, n).Format(time.DateOnly)
	}

	aurora := periodsOf("aurora")
	if len(aurora) < 4 {
		log.Fatalf("o contrato da Aurora devia propor 4 períodos, propôs %d", len(aurora))
	}
	fee := aurora[0].CalculatedCents
	r := confirm(aurora[0], fee, "NF 101", "")
	receive(r, earlier(plusDays(r.DueDate, 12), day))
	r = confirm(aurora[1], fee, "NF 102", "")
	receive(r, earlier(r.DueDate, day))
	confirm(aurora[2], fee, "NF 103", "Cobrança adiantada, a pedido da clínica.")
	sum.toConfirm++ // o período seguinte fica como proposta

	pixel := periodsOf("pixel")
	if len(pixel) < 2 {
		log.Fatalf("a Pixel devia ter ao menos 2 períodos fechados, tem %d", len(pixel))
	}
	// O valor fechado em reais inteiros, combinado por e-mail; o que o sistema calculou fica guardado no recebível.
	late := confirm(pixel[0], pixel[0].CalculatedCents/100*100, "NF 87", "Cliente pediu mais prazo por e-mail.")
	if !late.Late {
		log.Fatalf("o recebível da Pixel devia estar atrasado (vence em %s, hoje é %s)", late.DueDate, day)
	}
	sum.late++
	sum.toConfirm += len(pixel) - 1
}
