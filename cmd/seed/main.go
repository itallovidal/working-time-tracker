// Command seed popula um banco vazio com dados de demonstração: uma software
// house com o perfil preenchido, cinco pessoas, três clientes e seis projetos
// (um deles interno). Cada projeto tem o valor cobrado do cliente, e cada pessoa
// tem um valor por hora em cada projeto em que trabalha. Há também times,
// tarefas e sessões de trabalho dos últimos dias, já com os valores.
//
// Uso: go run ./cmd/seed (lê DATABASE_URL do ambiente ou do .env)
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"

	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/allocation"
	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/domain/customer"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/team"
)

const password = "demo12345"

// As pessoas da software house. A primeira cria a organização e é a admin; as
// outras entram por convite, como membros.
var people = []struct{ key, name, email string }{
	{"ana", "Ana Souza", "ana@example.com"},
	{"bruno", "Bruno Lima", "bruno@example.com"},
	{"carla", "Carla Mendes", "carla@example.com"},
	{"diego", "Diego Rocha", "diego@example.com"},
	{"elisa", "Elisa Prado", "elisa@example.com"},
}

var customers = []struct{ key, name, document, contact, email, phone string }{
	{"bompreco", "Rede Bom Preço", "12.ABC.345/01DE-35", "Marcos Dias", "marcos@bompreco.example", "+55 (11) 3003-1000"},
	{"vidaplena", "Clínica Vida Plena", "11.444.777/0001-61", "Renata Alves", "renata@vidaplena.example", "+55 (48) 3003-2200"},
	{"pagai", "Pagaí Pagamentos", "", "Tiago Nunes", "tiago@pagai.example", ""},
}

// Os projetos. customer vazio é um projeto interno, sem valor cobrado. Em rates
// está quanto cada pessoa recebe por hora no projeto, em centavos: o Bruno
// recebe mais na API de Cobranças, que é mais complexa, do que nos outros.
var projects = []struct {
	key, customer, name, description string
	sprintDays, weeklyHours          int
	daily, weekly                    string
	billRate                         int
	teams                            map[string][]string
	rates                            map[string]int
}{
	{
		key: "app", customer: "bompreco", name: "App de Pedidos",
		description: "Aplicativo para os clientes da rede pedirem e acompanharem as entregas.",
		sprintDays:  14, weeklyHours: 40, daily: "09:30", weekly: "friday", billRate: 14000,
		teams: map[string][]string{"Mobile": {"diego", "ana"}, "Backend": {"bruno"}},
		rates: map[string]int{"ana": 9000, "bruno": 5500, "diego": 6000},
	},
	{
		key: "painel", customer: "bompreco", name: "Painel do Lojista",
		description: "Painel web para cada loja acompanhar pedidos, repasses e avaliações.",
		sprintDays:  14, weeklyHours: 20, billRate: 12000,
		teams: map[string][]string{"Web": {"carla", "bruno"}},
		rates: map[string]int{"bruno": 5500, "carla": 5000},
	},
	{
		key: "agenda", customer: "vidaplena", name: "Agendamento Online",
		description: "Marcação de consultas pelo site e pelo WhatsApp, com confirmação automática.",
		sprintDays:  7, weeklyHours: 30, daily: "10:00", weekly: "monday", billRate: 11000,
		teams: map[string][]string{"Produto": {"carla", "diego", "elisa"}},
		rates: map[string]int{"carla": 5000, "diego": 5800, "elisa": 4000},
	},
	{
		// O Diego está no time, mas ainda sem valor: a aba Valores avisa, e ele
		// não consegue bater ponto aqui até um admin definir.
		key: "portal", customer: "vidaplena", name: "Portal do Paciente",
		description: "Resultados de exames e histórico de consultas para o paciente.",
		sprintDays:  14, weeklyHours: 20, billRate: 11500,
		teams: map[string][]string{"Web": {"carla", "bruno", "diego"}},
		rates: map[string]int{"bruno": 5500, "carla": 5200},
	},
	{
		key: "api", customer: "pagai", name: "API de Cobranças",
		description: "API de boletos e Pix, com conciliação diária.",
		sprintDays:  14, weeklyHours: 40, daily: "09:00", weekly: "wednesday", billRate: 18000,
		teams: map[string][]string{"Backend": {"bruno", "ana"}},
		rates: map[string]int{"ana": 11000, "bruno": 7000},
	},
	{
		key: "site", name: "Site da Jatobá",
		description: "Site institucional e blog da própria Jatobá.",
		sprintDays:  14,
		teams:       map[string][]string{"Marketing": {"carla"}},
		rates:       map[string]int{"carla": 4500},
	},
}

// As tarefas, com o prazo em dias a partir de hoje (negativo é atrasada).
var tasks = []struct {
	key, project, name, description, assignee string
	deadlineDays                              float64
}{
	{"checkout", "app", "Tela de checkout", "Resumo do pedido, endereço e pagamento em uma tela só.", "diego", 5},
	{"push", "app", "Notificações de status do pedido", "Push quando o pedido sai para entrega e quando chega.", "diego", 9},
	{"frete", "app", "Endpoint de cálculo de frete", "Frete por distância, com frete grátis acima de R$ 100.", "bruno", -2},
	{"arquitetura", "app", "Revisão de arquitetura do app", "", "ana", 0.8},
	{"repasses", "painel", "Relatório de repasses", "Totais por dia, com exportação em CSV.", "carla", 6},
	{"avaliacoes", "painel", "API de avaliações das lojas", "", "bruno", 12},
	{"calendario", "agenda", "Calendário de horários disponíveis", "Horários por médico e por unidade.", "carla", 4},
	{"whatsapp", "agenda", "Confirmação por WhatsApp", "Mensagem na véspera, com opção de remarcar.", "diego", 8},
	{"testes", "agenda", "Plano de testes do agendamento", "", "elisa", 3},
	{"login", "portal", "Login com CPF e data de nascimento", "", "bruno", 10},
	{"exames", "portal", "Tela de resultados de exames", "", "carla", 14},
	{"webhook", "api", "Webhook de pagamento confirmado", "Avisa o sistema do cliente quando o boleto ou o Pix compensa.", "bruno", 3},
	{"conciliacao", "api", "Conciliação diária de Pix", "", "bruno", 7},
	{"recorrencia", "api", "Modelo de dados de cobranças recorrentes", "", "ana", -1},
	{"cases", "site", "Página de cases", "", "carla", 15},
}

// As sessões de trabalho já encerradas: quantos dias atrás, e o horário de
// início e de fim.
var sessions = []struct {
	task, person       string
	daysAgo            int
	startH, startM     int
	finishH, finishMin int
}{
	{"checkout", "diego", 3, 9, 0, 12, 0},
	{"checkout", "diego", 2, 9, 0, 11, 30},
	{"checkout", "diego", 1, 9, 10, 12, 5},
	{"whatsapp", "diego", 2, 14, 0, 17, 0},
	{"whatsapp", "diego", 1, 14, 0, 16, 30},
	{"webhook", "bruno", 3, 9, 0, 12, 30},
	{"frete", "bruno", 3, 14, 0, 15, 20},
	{"webhook", "bruno", 2, 9, 0, 12, 0},
	{"avaliacoes", "bruno", 2, 14, 0, 16, 0},
	{"conciliacao", "bruno", 1, 9, 0, 11, 45},
	{"login", "bruno", 1, 14, 0, 16, 10},
	{"repasses", "carla", 3, 9, 30, 12, 0},
	{"calendario", "carla", 3, 13, 30, 17, 30},
	{"calendario", "carla", 2, 9, 0, 12, 30},
	{"exames", "carla", 2, 14, 0, 16, 0},
	{"repasses", "carla", 1, 9, 0, 11, 0},
	{"cases", "carla", 1, 15, 0, 16, 30},
	{"testes", "elisa", 2, 10, 0, 12, 0},
	{"testes", "elisa", 1, 10, 0, 12, 30},
	{"arquitetura", "ana", 2, 10, 0, 12, 30},
	{"recorrencia", "ana", 2, 14, 0, 17, 15},
	{"recorrencia", "ana", 1, 14, 0, 16, 0},
}

func main() {
	godotenv.Load()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("defina DATABASE_URL (veja o .env.example)")
	}
	db, err := database.Open(dsn)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	if err := database.AutoMigrate(db.Client); err != nil {
		log.Fatalf("migration: %v", err)
	}
	ctx := context.Background()
	if n := db.Client.Organization.Query().CountX(ctx); n > 0 {
		log.Fatalf("o banco já tem %d organização(ões); o seed só roda num banco vazio", n)
	}

	authSvc := auth.NewService(auth.NewStore(db.Client))
	orgSvc := organization.NewService(organization.NewStore(db.Client))
	customerSvc := customer.NewService(customer.NewStore(db.Client))
	allocationSvc := allocation.NewService(allocation.NewStore(db.Client))
	projectSvc := project.NewService(project.NewStore(db.Client))
	teamSvc := team.NewService(team.NewStore(db.Client))
	membershipStore := team.NewMembershipStore(db.Client)
	membershipSvc := team.NewMembershipService(membershipStore)
	taskSvc := task.NewService(task.NewStore(db.Client), membershipStore, nil)

	text := func(v string) *string { return &v }
	number := func(v int) *int { return &v }
	// optional devolve nil para o valor vazio, que os services tratam como "sem valor".
	optionalText := func(v string) *string {
		if v == "" {
			return nil
		}
		return &v
	}
	optionalNumber := func(v int) *int {
		if v == 0 {
			return nil
		}
		return &v
	}

	// A organização e as pessoas.
	admin, _, err := authSvc.Signup(auth.SignupInput{
		OrganizationName: "Jatobá Software",
		Name:             people[0].name,
		Email:            people[0].email,
		Password:         password,
	})
	must(err)
	orgID := admin.OrganizationID.String()
	person := map[string]*auth.Identity{people[0].key: admin}
	for _, p := range people[1:] {
		_, token, err := authSvc.CreateInvite(admin, p.email, "member")
		must(err)
		person[p.key], _, err = authSvc.AcceptInvite(token, auth.AcceptInviteInput{Name: p.name, Email: p.email, Password: password})
		must(err)
	}

	_, err = orgSvc.Update(orgID, organization.UpdateInput{
		Summary: text("Software house que cria e mantém produtos digitais para outras empresas."),
		Description: text("A Jatobá Software desenvolve aplicativos, sistemas web e APIs sob medida. " +
			"Trabalhamos por projeto: cada cliente tem um time dedicado, com entregas a cada sprint.\n\n" +
			"Atendemos varejo, saúde e serviços financeiros."),
		Industry:     text("Desenvolvimento de software"),
		FoundedYear:  number(2016),
		Size:         text("1-10"),
		Website:      text("https://jatoba.example"),
		ContactEmail: text("contato@jatoba.example"),
		Phone:        text("+55 (48) 3003-2016"),
		LinkedinURL:  text("https://www.linkedin.com/company/jatoba-software"),
		LegalName:    text("Jatobá Software Ltda"),
		CNPJ:         text("11.222.333/0001-81"),
		AddressLine1: text("Rua das Acácias, 120"),
		AddressLine2: text("Sala 304, Centro"),
		City:         text("Florianópolis"),
		State:        text("SC"),
		PostalCode:   text("88010-000"),
		Country:      text("Brasil"),
		WorkMode:     text("hybrid"),
	})
	must(err)

	// Os clientes.
	customerID := map[string]string{}
	for _, c := range customers {
		created, err := customerSvc.Create(orgID, customer.Input{
			Name:         text(c.name),
			Document:     text(c.document),
			ContactName:  text(c.contact),
			ContactEmail: text(c.email),
			ContactPhone: text(c.phone),
		})
		must(err)
		customerID[c.key] = created.ID.String()
	}

	// Os projetos, com o cliente e o valor cobrado, os times e o valor de cada pessoa.
	type seededProject struct {
		id       string
		billRate *int
		rates    map[string]int
	}
	seeded := map[string]seededProject{}
	for _, p := range projects {
		created, err := projectSvc.Create(orgID, p.name, p.description, p.sprintDays,
			optionalText(p.daily), optionalText(p.weekly), optionalNumber(p.weeklyHours))
		must(err)
		id := created.ID.String()

		billRate := optionalNumber(p.billRate)
		if p.customer != "" {
			cid := customerID[p.customer]
			_, err := projectSvc.SetBilling(id, &cid, billRate)
			must(err)
		}
		for teamName, members := range p.teams {
			tm, err := teamSvc.Create(id, teamName)
			must(err)
			for _, m := range members {
				_, err := membershipSvc.Add(tm.ID.String(), person[m].PersonID.String())
				must(err)
			}
		}
		for who, cents := range p.rates {
			_, err := allocationSvc.Set(id, person[who].PersonID.String(), cents)
			must(err)
		}
		seeded[p.key] = seededProject{id: id, billRate: billRate, rates: p.rates}
	}

	// As tarefas.
	now := time.Now()
	type seededTask struct {
		task    *task.Task
		project seededProject
	}
	seededTasks := map[string]seededTask{}
	for _, t := range tasks {
		prj := seeded[t.project]
		deadline := now.Add(time.Duration(t.deadlineDays * 24 * float64(time.Hour)))
		created, err := taskSvc.Create(prj.id, t.name, t.description, person[t.assignee].PersonID.String(), &deadline)
		must(err)
		seededTasks[t.key] = seededTask{task: created, project: prj}
	}

	// As sessões guardam os valores por hora de quando o ponto abriu, como o
	// clock-in faz: o que a pessoa recebe e o que o cliente paga.
	at := func(daysAgo, hour, minute int) time.Time {
		d := now.AddDate(0, 0, -daysAgo)
		return time.Date(d.Year(), d.Month(), d.Day(), hour, minute, 0, 0, d.Location())
	}
	for _, s := range sessions {
		t := seededTasks[s.task]
		db.Client.WorkSession.Create().
			SetTaskID(t.task.ID).
			SetPersonID(person[s.person].PersonID).
			SetStartAt(at(s.daysAgo, s.startH, s.startM)).
			SetEndAt(at(s.daysAgo, s.finishH, s.finishMin)).
			SetPayRateCents(t.project.rates[s.person]).
			SetNillableBillRateCents(t.project.billRate).
			SaveX(ctx)
	}

	fmt.Println("Dados de demonstração criados.")
	fmt.Println()
	fmt.Println("  Organização: Jatobá Software, uma software house")
	fmt.Printf("  %d clientes, %d projetos (um interno), %d tarefas e %d sessões de trabalho\n",
		len(customers), len(projects), len(tasks), len(sessions))
	fmt.Println()
	fmt.Printf("  Admin:   %s / %s\n", people[0].email, password)
	for _, p := range people[1:] {
		fmt.Printf("  Membro:  %s / %s\n", p.email, password)
	}
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
