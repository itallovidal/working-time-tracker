// Command seed popula um banco vazio com dados de demonstração para as telas
// ficarem cheias: uma software house com doze pessoas, quatro clientes e oito
// projetos (um interno), cada um com o valor cobrado do cliente, times, o valor
// por hora de cada pessoa e tarefas com prazos espalhados, atrasadas e sem
// responsável. Há ainda cerca de dois meses de sessões de trabalho já fechadas,
// geradas por uma sequência fixa (o mesmo seed dá sempre o mesmo banco), duas
// pessoas com o ponto aberto e integrações com itens vinculados a tarefas.
//
// Uso: go run ./cmd/seed (lê DATABASE_URL do ambiente ou do .env)
package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"

	"working-time-tracker/ent"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/allocation"
	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/domain/customer"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/team"
)

const password = "demo12345"

// historyDays é até onde as sessões voltam no tempo.
const historyDays = 75

// As pessoas da software house. A primeira cria a organização, é admin e dona dela; as
// outras entram por convite, com o papel indicado. weeklyHours é a jornada
// semanal combinada com cada uma, que vale para todos os projetos: a Ana, sócia,
// não tem jornada definida, e a Elisa e o João trabalham meio período.
var people = []struct {
	key, name, email, role string
	weeklyHours            int
}{
	{"ana", "Ana Souza", "ana@example.com", "admin", 0},
	{"helena", "Helena Costa", "helena@example.com", "admin", 40},
	{"bruno", "Bruno Lima", "bruno@example.com", "member", 40},
	{"carla", "Carla Mendes", "carla@example.com", "member", 40},
	{"diego", "Diego Rocha", "diego@example.com", "member", 30},
	{"elisa", "Elisa Prado", "elisa@example.com", "member", 20},
	{"fabio", "Fábio Teixeira", "fabio@example.com", "member", 40},
	{"gabriela", "Gabriela Nunes", "gabriela@example.com", "member", 30},
	{"henrique", "Henrique Barros", "henrique@example.com", "member", 40},
	{"isabela", "Isabela Cardoso", "isabela@example.com", "member", 40},
	{"joao", "João Pedro Alves", "joao@example.com", "member", 20},
	{"larissa", "Larissa Freitas", "larissa@example.com", "member", 40},
}

var customers = []struct{ key, name, document, contact, email, phone string }{
	{"bompreco", "Rede Bom Preço", "12.ABC.345/01DE-35", "Marcos Dias", "marcos@bompreco.example", "+55 (11) 3003-1000"},
	{"vidaplena", "Clínica Vida Plena", "11.444.777/0001-61", "Renata Alves", "renata@vidaplena.example", "+55 (48) 3003-2200"},
	{"pagai", "Pagaí Pagamentos", "", "Tiago Nunes", "tiago@pagai.example", ""},
	{"atacado", "Atacado Norte", "45.723.174/0001-10", "Sérgio Matos", "sergio@atacadonorte.example", "+55 (92) 3003-4400"},
}

// Os projetos. customer vazio é um projeto interno, sem valor cobrado. Em rates
// está quem trabalha no projeto e quanto recebe por hora nele, em centavos. Os
// times só têm gente que está em rates, e quem está em rates sem time aparece
// como "sem time". Na Migração do ERP o valor cobrado é menor que o que a maioria
// recebe, de propósito: a margem fica negativa e a tela mostra isso em vermelho.
var projects = []struct {
	key, customer, name, description string
	sprintDays                       int
	daily, weekly, weeklyTime        string
	meetingDay, meetingTime          string
	presets                          map[string]string // quem tem um grupo de permissões além de colaborador
	billRate, ageDays                int
	teams                            map[string][]string
	rates                            map[string]int
}{
	{
		key: "app", customer: "bompreco", name: "App de Pedidos", ageDays: 330,
		description: "Aplicativo para os clientes da rede pedirem e acompanharem as entregas.",
		sprintDays:  14, daily: "09:30", weekly: "friday", weeklyTime: "14:00", billRate: 14000, meetingDay: "wednesday", meetingTime: "10:00",
		teams:   map[string][]string{"Mobile": {"diego", "gabriela", "ana"}, "Backend": {"bruno", "henrique", "fabio"}, "Qualidade": {"elisa"}, "Produto": {"isabela", "carla"}},
		rates:   map[string]int{"ana": 9000, "bruno": 5500, "diego": 6000, "gabriela": 5200, "henrique": 4200, "isabela": 6500, "elisa": 4000, "carla": 5000, "fabio": 6200},
		presets: map[string]string{"isabela": "manager"},
	},
	{
		key: "painel", customer: "bompreco", name: "Painel do Lojista", ageDays: 214,
		description: "Painel web para cada loja acompanhar pedidos, repasses e avaliações.",
		sprintDays:  14, daily: "09:15", weekly: "thursday", weeklyTime: "13:00", billRate: 12000, meetingDay: "monday", meetingTime: "15:00",
		teams:   map[string][]string{"Web": {"carla", "bruno", "henrique"}, "Design": {"gabriela"}},
		rates:   map[string]int{"bruno": 5500, "carla": 5000, "elisa": 4000, "henrique": 4200, "gabriela": 5200},
		presets: map[string]string{"carla": "manager"},
	},
	{
		key: "agenda", customer: "vidaplena", name: "Agendamento Online", ageDays: 152,
		description: "Marcação de consultas pelo site e pelo WhatsApp, com confirmação automática.",
		sprintDays:  7, daily: "10:00", weekly: "monday", weeklyTime: "11:30", billRate: 11000, meetingDay: "friday", meetingTime: "11:00",
		teams:   map[string][]string{"Produto": {"carla", "diego", "elisa", "isabela"}},
		rates:   map[string]int{"carla": 5000, "diego": 5800, "elisa": 4000, "isabela": 6500, "joao": 2500},
		presets: map[string]string{"isabela": "manager"},
	},
	{
		key: "portal", customer: "vidaplena", name: "Portal do Paciente", ageDays: 96,
		description: "Resultados de exames e histórico de consultas para o paciente.",
		sprintDays:  14, weekly: "tuesday", weeklyTime: "11:00", billRate: 11500,
		teams: map[string][]string{"Web": {"bruno", "carla"}, "Dados": {"larissa"}, "Design": {"gabriela"}},
		rates: map[string]int{"bruno": 5500, "carla": 5200, "larissa": 6000, "gabriela": 5200},
	},
	{
		key: "api", customer: "pagai", name: "API de Cobranças", ageDays: 68,
		description: "API de boletos e Pix, com conciliação diária.",
		sprintDays:  14, daily: "09:00", weekly: "wednesday", weeklyTime: "15:00", billRate: 18000, meetingDay: "tuesday", meetingTime: "14:30",
		teams: map[string][]string{"Backend": {"bruno", "ana", "fabio"}, "Infra": {"fabio", "henrique"}, "Gestão": {"helena"}},
		rates: map[string]int{"ana": 11000, "bruno": 7000, "fabio": 7200, "henrique": 4800, "helena": 12000},
	},
	{
		key: "dados", customer: "pagai", name: "Dashboard Financeiro", ageDays: 120,
		description: "Painéis de recebimentos, inadimplência e conciliação para o financeiro da Pagaí.",
		sprintDays:  14, daily: "11:00", weekly: "thursday", weeklyTime: "16:00", billRate: 15000,
		teams:   map[string][]string{"Dados": {"larissa", "henrique", "fabio"}, "Produto": {"isabela"}},
		rates:   map[string]int{"larissa": 6500, "henrique": 4500, "fabio": 6500, "isabela": 6500},
		presets: map[string]string{"larissa": "finance"},
	},
	{
		key: "erp", customer: "atacado", name: "Migração do ERP", ageDays: 190,
		description: "Migração do ERP legado do Atacado Norte para um sistema novo, com carga de dados e homologação.",
		sprintDays:  30, daily: "08:45", weekly: "friday", weeklyTime: "10:00", billRate: 9000,
		teams: map[string][]string{"Migração": {"bruno", "fabio", "larissa"}, "Gestão": {"ana"}},
		rates: map[string]int{"bruno": 9500, "fabio": 8000, "larissa": 7500, "ana": 11500, "joao": 3000},
	},
	{
		key: "site", name: "Site da Jatobá", ageDays: 37,
		description: "Site institucional e blog da própria Jatobá.",
		sprintDays:  14, daily: "10:30",
		teams: map[string][]string{"Marketing": {"carla", "gabriela"}},
		rates: map[string]int{"carla": 4500, "gabriela": 4800, "joao": 2500},
	},
}

// As tarefas, com o prazo em dias a partir de hoje (negativo é atrasada).
// assignee vazio é uma tarefa sem responsável: fica disponível no quadro, e quem
// bater o ponto nela a pega. link, quando há, vincula a tarefa a um item da
// integração do projeto: "gh:42" (issue do GitHub), "gl:17" (issue do GitLab) ou
// "tr:AbCd1234" (cartão do Trello).
// priorityFor e labelsFor dão a prioridade e as etiquetas de cada tarefa a partir do que ela
// já diz: o prazo (a atrasada é urgente) e o assunto do nome. Assim a lista de tarefas
// não precisa carregar dois campos a mais em cada linha.
func priorityFor(deadlineDays float64) string {
	switch {
	case deadlineDays < 0:
		return "urgent"
	case deadlineDays <= 3:
		return "high"
	case deadlineDays <= 10:
		return "medium"
	case deadlineDays <= 20:
		return "low"
	}
	return "none"
}

// statusFor dá o status de cada tarefa a partir do que ela já diz: a sem responsável está no
// backlog, a atrasada ou a que vence logo está em progresso, e as demais se espalham pelos
// quatro status, na ordem em que aparecem, para o quadro mostrar todos.
func statusFor(i int, assigned bool, deadlineDays float64) string {
	switch {
	case !assigned:
		return "backlog"
	case deadlineDays <= 3:
		return "in_progress"
	}
	return task.Statuses[i%len(task.Statuses)]
}

var labelKeywords = []struct {
	label string
	words []string
}{
	{"backend", []string{"endpoint", "api ", "api de", "repasses", "perfis de acesso"}},
	{"frontend", []string{"tela", "layout", "modo escuro", "fluxo", "filtros", "home"}},
	{"design", []string{"layout", "modo escuro", "acessibilidade", "boas-vindas"}},
	{"qualidade", []string{"teste", "revisão", "acessibilidade"}},
	{"infra", []string{"pipeline", "build", "carga", "publicação"}},
}

func labelsFor(name string) []string {
	lower := strings.ToLower(name) + " "
	var out []string
	for _, k := range labelKeywords {
		for _, w := range k.words {
			if strings.Contains(lower, w) {
				out = append(out, k.label)
				break
			}
		}
	}
	return out
}

var tasks = []struct {
	project, name, description, assignee string
	deadlineDays                         float64
	link                                 string
}{
	// App de Pedidos: mais de uma página, com prazos espalhados.
	{"app", "Tela de checkout", "Resumo do pedido, endereço e pagamento em uma tela só.", "diego", 5, "gh:142"},
	{"app", "Notificações de status do pedido", "Push quando o pedido sai para entrega e quando chega.", "diego", 9, "gh:151"},
	{"app", "Endpoint de cálculo de frete", "Frete por distância, com frete grátis acima de R$ 100.", "bruno", -2, "gh:138"},
	{"app", "Revisão de arquitetura do app", "", "ana", 0.8, ""},
	{"app", "Carrinho com itens salvos", "O carrinho continua como estava quando o cliente volta ao app.", "diego", 2, "gh:149"},
	{"app", "Histórico de pedidos", "", "gabriela", 11, "tr:Hq4Lm8Zp"},
	{"app", "Login com telefone e código por SMS", "", "bruno", 1, "gh:131"},
	{"app", "Rastreamento da entrega no mapa", "Posição do entregador atualizada a cada 30 segundos.", "diego", 16, "gh:160"},
	{"app", "Cupom de desconto no checkout", "", "henrique", 6, "gh:155"},
	{"app", "Avaliação do pedido entregue", "", "gabriela", 20, ""},
	{"app", "Endpoint de repetir pedido", "Monta um carrinho novo com os itens de um pedido anterior.", "henrique", 13, "gh:158"},
	{"app", "Testes de carga da API de pedidos", "", "fabio", -4, ""},
	{"app", "Política de privacidade no app", "", "isabela", 3, ""},
	{"app", "Publicação nas lojas de aplicativos", "", "ana", 25, ""},
	{"app", "Plano de testes da release 2.4", "Casos de regressão para checkout, cupom e rastreamento.", "elisa", 4, "tr:Xr7Tn2Ka"},
	{"app", "Pipeline de build e distribuição interna", "", "fabio", 7, "gh:129"},
	{"app", "Pesquisa de satisfação pós-entrega", "Duas perguntas e um campo livre, depois da avaliação.", "isabela", 18, ""},
	{"app", "Fluxo de boas-vindas do app", "Telas de apresentação no primeiro acesso.", "", 8, ""},
	{"app", "Modo escuro", "", "", 30, ""},
	{"app", "Acessibilidade das telas de pedido", "Leitor de tela e contraste em todo o fluxo de compra.", "", -1, ""},
	// Painel do Lojista
	{"painel", "Relatório de repasses", "Totais por dia, com exportação em CSV.", "carla", 6, ""},
	{"painel", "API de avaliações das lojas", "", "bruno", 12, ""},
	{"painel", "Filtros do painel de pedidos", "Período, status e forma de pagamento.", "henrique", 3, ""},
	{"painel", "Novo layout da home do painel", "", "gabriela", 9, ""},
	{"painel", "Testes de aceitação do painel", "", "carla", -3, ""},
	{"painel", "Revisão de acessibilidade do painel", "", "", 18, ""},
	{"painel", "Perfis de acesso por loja", "Gerente, caixa e financeiro.", "carla", 21, ""},
	{"painel", "Exportação de pedidos em planilha", "", "", 5, ""},
	// Agendamento Online
	{"agenda", "Calendário de horários disponíveis", "Horários por médico e por unidade.", "carla", 4, "tr:Wd3Kp9Qe"},
	{"agenda", "Confirmação por WhatsApp", "Mensagem na véspera, com opção de remarcar.", "diego", 8, "tr:Nb6Vx1Rt"},
	{"agenda", "Plano de testes do agendamento", "", "elisa", 3, ""},
	{"agenda", "Fila de espera para cancelamentos", "Avisa quem está na fila quando um horário abre.", "isabela", 10, ""},
	{"agenda", "Página de ajuda para pacientes", "", "elisa", 2, ""},
	{"agenda", "Lembrete por e-mail", "", "", 6, ""},
	// Portal do Paciente
	{"portal", "Login com CPF e data de nascimento", "", "bruno", 10, ""},
	{"portal", "Tela de resultados de exames", "", "carla", 14, ""},
	{"portal", "Modelo de dados do histórico de consultas", "", "larissa", 5, ""},
	{"portal", "Identidade visual do portal", "", "gabriela", -2, ""},
	{"portal", "Download de laudos em PDF", "", "", 22, ""},
	// API de Cobranças
	{"api", "Webhook de pagamento confirmado", "Avisa o sistema do cliente quando o boleto ou o Pix compensa.", "bruno", 3, "gl:41"},
	{"api", "Conciliação diária de Pix", "", "fabio", 7, "gl:44"},
	{"api", "Modelo de dados de cobranças recorrentes", "", "ana", -1, "gl:37"},
	{"api", "Limite de requisições por cliente", "", "henrique", 5, "gl:46"},
	{"api", "Painel de métricas da API", "", "fabio", 12, ""},
	{"api", "Revisão da documentação pública", "", "helena", 9, "gh:12"},
	{"api", "Autenticação por chave rotativa", "", "bruno", 16, "gl:49"},
	{"api", "Ambiente de sandbox para clientes", "", "", 19, ""},
	// Dashboard Financeiro
	{"dados", "Modelagem do data mart de recebimentos", "", "larissa", 6, "gh:8"},
	{"dados", "Gráfico de inadimplência por faixa de atraso", "", "henrique", 11, ""},
	{"dados", "Pipeline noturno de carga", "", "fabio", -5, "gh:5"},
	{"dados", "Definição dos indicadores com o financeiro", "", "isabela", 4, ""},
	{"dados", "Exportação do painel em PDF", "", "", 24, ""},
	// Migração do ERP
	{"erp", "Mapeamento de tabelas do legado", "Planilha com cada tabela de origem e o destino.", "larissa", -8, "gh:3"},
	{"erp", "Script de carga de clientes e fornecedores", "", "bruno", 2, "gh:9"},
	{"erp", "Validação dos saldos migrados", "Comparar saldos do legado e do novo, conta a conta.", "fabio", 6, "gh:14"},
	{"erp", "Plano de virada e retorno", "", "ana", 14, ""},
	{"erp", "Treinamento dos usuários do financeiro", "", "ana", 20, ""},
	{"erp", "Homologação do módulo fiscal", "", "", 28, ""},
	// Site da Jatobá
	{"site", "Página de cases", "", "carla", 15, ""},
	{"site", "Artigo sobre a migração do ERP", "", "gabriela", 8, ""},
	{"site", "Atualizar fotos da equipe", "", "gabriela", 12, ""},
	{"site", "Formulário de contato com anti-spam", "", "", 10, ""},
}

// Há quantos dias cada projeto foi cadastrado vem de projects (ageDays), que é o
// início dele na Visão geral. Todos começam antes da sessão mais antiga que têm.

// As integrações de demonstração. Não têm credencial, porque o token é de cada
// um: aparecem na aba Integrações e na Visão geral, e só buscam os itens depois
// que alguém edita e informa o token. Os itens vinculados às tarefas (link) usam
// a primeira integração do tipo no projeto.
var integrations = []struct {
	project, kind, name string
	metadata            map[string]interface{}
	enabled             bool
}{
	{"app", "github", "Repositório do app", map[string]interface{}{"repo": "jatoba-software/app-pedidos"}, true},
	{"app", "gitlab", "Espelho no GitLab", map[string]interface{}{"project_url": "jatoba-software/app-pedidos"}, false},
	{"app", "trello", "Quadro de sprint do app", map[string]interface{}{"api_key": "demo-key-app-0001", "board_id": "AbCd1234"}, true},
	{"agenda", "trello", "Quadro do agendamento", map[string]interface{}{"api_key": "demo-key-agenda-0002", "board_id": "Ef5Gh678"}, true},
	{"api", "gitlab", "Repositório da API", map[string]interface{}{"project_url": "jatoba-software/api-cobrancas"}, true},
	{"api", "github", "Documentação pública", map[string]interface{}{"repo": "jatoba-software/api-docs"}, true},
	{"dados", "github", "Pipelines de dados", map[string]interface{}{"repo": "jatoba-software/financeiro-dados"}, true},
	{"erp", "github", "Scripts de migração", map[string]interface{}{"repo": "jatoba-software/erp-migracao"}, true},
}

// As duas pessoas que estão com o ponto aberto agora, numa tarefa em que são
// responsáveis, e há quantos minutos começaram. O ponto fica aberto até alguém
// parar; se o banco for visto dias depois, a sessão terá crescido.
var openNow = []struct {
	person, project, task string
	minutes               int
}{
	{"bruno", "app", "Login com telefone e código por SMS", 95},
	{"carla", "painel", "Relatório de repasses", 40},
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
	ctx := context.Background()
	if err := database.Migrate(ctx, db.Raw); err != nil {
		log.Fatalf("migration: %v", err)
	}
	if n := db.Client.Organization.Query().CountX(ctx); n > 0 {
		log.Fatalf("o banco já tem %d organização(ões); o seed só roda num banco vazio", n)
	}

	authSvc := auth.NewService(auth.NewStore(db.Client))
	orgSvc := organization.NewService(organization.NewStore(db.Client))
	customerSvc := customer.NewService(customer.NewStore(db.Client))
	personSvc := person.NewService(person.NewStore(db.Client))
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
		_, token, err := authSvc.CreateInvite(admin, p.email, p.role)
		must(err)
		person[p.key], _, err = authSvc.AcceptInvite(token, auth.AcceptInviteInput{Name: p.name, Email: p.email, Password: password})
		must(err)
	}
	for _, p := range people {
		_, err := personSvc.SetWeeklyHours(person[p.key].PersonID.String(), optionalNumber(p.weeklyHours))
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
		created, err := projectSvc.Create(orgID, p.name, p.description, p.sprintDays, project.Routine{
			DailyTime: optionalText(p.daily), WeeklySyncDay: optionalText(p.weekly), WeeklySyncTime: optionalText(p.weeklyTime),
			CustomerMeetingDay: optionalText(p.meetingDay), CustomerMeetingTime: optionalText(p.meetingTime),
		})
		must(err)
		id := created.ID.String()

		billRate := optionalNumber(p.billRate)
		if p.customer != "" {
			cid := customerID[p.customer]
			_, err := projectSvc.SetBilling(id, &cid, billRate)
			must(err)
		}
		// O valor vem antes do time: é ele que põe a pessoa no projeto.
		for who, cents := range p.rates {
			// A Ana é a dona da organização: não recebe valor por hora, as horas dela
			// valem o valor cobrado. O serviço grava zero; a tabela segue o mesmo valor.
			if who == people[0].key {
				cents = 0
				p.rates[who] = 0
			}
			_, err := allocationSvc.Set(id, person[who].PersonID.String(), cents)
			must(err)
		}
		for who, preset := range p.presets {
			_, err := allocationSvc.SetPreset(id, person[who].PersonID.String(), preset)
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
		seeded[p.key] = seededProject{id: id, billRate: billRate, rates: p.rates}
	}

	now := time.Now()

	// O cadastro de cada projeto volta no tempo. O service grava a data de hoje e
	// ela não se altera depois, então sem isto todo projeto teria começado depois
	// das próprias sessões.
	projectAge := map[string]int{}
	for _, p := range projects {
		projectAge[p.key] = p.ageDays
		_, err := db.Raw.ExecContext(ctx, `UPDATE projects SET created_at = $1 WHERE id = $2`, now.AddDate(0, 0, -p.ageDays), seeded[p.key].id)
		must(err)
	}

	// As integrações vão direto ao banco: o service valida o token na plataforma,
	// e aqui não há token. A primeira de cada tipo, em cada projeto, recebe os itens.
	integrationOf := map[string]*ent.Integration{}
	for _, it := range integrations {
		created := db.Client.Integration.Create().
			SetProjectID(uuid.MustParse(seeded[it.project].id)).
			SetType(it.kind).
			SetDisplayName(it.name).
			SetMetadata(it.metadata).
			SetEnabled(it.enabled).
			SaveX(ctx)
		if _, ok := integrationOf[it.project+"/"+it.kind]; !ok {
			integrationOf[it.project+"/"+it.kind] = created
		}
	}

	// As tarefas. Guarda as que têm responsável, por projeto, para as sessões.
	type seededTask struct {
		task     *task.Task
		assignee string
	}
	byProject := map[string][]seededTask{}
	labelID := map[string]string{} // projeto/etiqueta -> id
	byTaskName := map[string]seededTask{}
	for i, t := range tasks {
		prj := seeded[t.project]
		deadline := now.Add(time.Duration(t.deadlineDays * 24 * float64(time.Hour)))
		assignee := "" // sem responsável
		if t.assignee != "" {
			assignee = person[t.assignee].PersonID.String()
		}
		// As etiquetas do projeto nascem quando a primeira tarefa as usa.
		ids := []string{}
		for _, l := range labelsFor(t.name) {
			key := t.project + "/" + l
			if _, ok := labelID[key]; !ok {
				created, err := taskSvc.CreateLabel(prj.id, l)
				must(err)
				labelID[key] = created.ID.String()
			}
			ids = append(ids, labelID[key])
		}
		priority := priorityFor(t.deadlineDays)
		created, err := taskSvc.CreateAs("", prj.id, t.name, t.description, assignee, &deadline,
			task.Attrs{Priority: &priority, LabelIDs: &ids})
		must(err)
		// Toda tarefa nasce em backlog; o seed a leva ao status que a tela deve mostrar.
		if status := statusFor(i, t.assignee != "", t.deadlineDays); status != task.StatusBacklog {
			created, err = taskSvc.UpdateAs("", created.ID.String(), created.Name, created.Description, nil, nil, task.Attrs{Status: &status})
			must(err)
		}
		st := seededTask{task: created, assignee: t.assignee}
		byTaskName[t.project+"/"+t.name] = st
		if t.assignee != "" {
			byProject[t.project] = append(byProject[t.project], st)
		}
		if t.link != "" {
			linkTask(ctx, db.Client, integrationOf, t.project, created.ID, t.link)
		}
	}

	// As sessões guardam os valores por hora de quando o ponto abriu, como o
	// clock-in faz: o que a pessoa recebe e o que o cliente paga.
	at := func(daysAgo int, minutes int) time.Time {
		d := now.AddDate(0, 0, -daysAgo)
		return time.Date(d.Year(), d.Month(), d.Day(), 0, minutes, 0, 0, d.Location())
	}
	rng := rand.New(rand.NewSource(20261006))
	// Algumas sessões têm uma segunda tarefa em paralelo. Um sorteio à parte, para o histórico
	// de quem trabalhou quando e quanto continuar o mesmo.
	rngParallel := rand.New(rand.NewSource(20261008))
	// links guarda, para cada builder, as tarefas da sessão e o intervalo de cada uma.
	type seedLink struct {
		task        uuid.UUID
		from, until *time.Time
	}
	var builders []*ent.WorkSessionCreate
	var links [][]seedLink
	closed := 0
	for daysAgo := 1; daysAgo <= historyDays; daysAgo++ {
		day := now.AddDate(0, 0, -daysAgo)
		weekend := day.Weekday() == time.Saturday || day.Weekday() == time.Sunday
		for _, who := range people {
			if weekend && rng.Float64() > 0.08 {
				continue
			}
			if rng.Float64() < 0.05 { // falta, folga ou férias
				continue
			}
			var mine []struct {
				key  string
				rate int
			}
			for _, p := range projects {
				if rate, ok := p.rates[who.key]; ok && projectAge[p.key] > daysAgo+1 && len(byProject[p.key]) > 0 {
					mine = append(mine, struct {
						key  string
						rate int
					}{p.key, rate})
				}
			}
			if len(mine) == 0 {
				continue
			}
			// A meta do dia vem da jornada: a Ana, sem jornada, trabalha seis horas.
			target := 6.0
			if who.weeklyHours > 0 {
				target = float64(who.weeklyHours) / 5
			}
			target *= 0.7 + rng.Float64()*0.45
			blocks := 2
			if rng.Float64() < 0.3 {
				blocks = 3
			}
			clockMin := 8*60 + 15 + rng.Intn(100)
			for b := 0; b < blocks; b++ {
				// O primeiro projeto da pessoa pesa mais.
				idx := 0
				if len(mine) > 1 && rng.Float64() > 0.55 {
					idx = rng.Intn(len(mine))
				}
				prj := mine[idx]
				pool := byProject[prj.key]
				var pick seededTask
				var own []seededTask
				for _, st := range pool {
					if st.assignee == who.key {
						own = append(own, st)
					}
				}
				if len(own) > 0 && rng.Float64() < 0.8 {
					pick = own[rng.Intn(len(own))]
				} else {
					pick = pool[rng.Intn(len(pool))]
				}
				share := target / float64(blocks)
				length := int((share*(0.6+rng.Float64()*0.8))*60/5) * 5
				if length < 30 {
					length = 30
				}
				startMin, endMin := clockMin, clockMin+length
				if endMin > 21*60 {
					break
				}
				builders = append(builders, db.Client.WorkSession.Create().
					SetProjectID(uuid.MustParse(seeded[prj.key].id)).
					SetPersonID(person[who.key].PersonID).
					SetStartAt(at(daysAgo, startMin)).
					SetEndAt(at(daysAgo, endMin)).
					SetPayRateCents(prj.rate).
					SetNillableBillRateCents(seeded[prj.key].billRate).
					SetOwnerHours(who.key == people[0].key))
				from, until := at(daysAgo, startMin), at(daysAgo, endMin)
				sessionLinks := []seedLink{{task: pick.task.ID, from: &from, until: &until}}
				if len(pool) > 1 && rngParallel.Float64() < 0.3 {
					other := pool[rngParallel.Intn(len(pool))]
					if other.task.ID != pick.task.ID {
						// A segunda tarefa entra no meio da sessão e vai até o fim dela.
						joined := at(daysAgo, startMin+length*(30+rngParallel.Intn(30))/100)
						sessionLinks = append(sessionLinks, seedLink{task: other.task.ID, from: &joined, until: &until})
					}
				}
				links = append(links, sessionLinks)
				closed++
				clockMin = endMin + 45 + rng.Intn(75) // pausa até o próximo bloco
			}
		}
	}
	for start := 0; start < len(builders); start += 200 {
		end := min(start+200, len(builders))
		created := db.Client.WorkSession.CreateBulk(builders[start:end]...).SaveX(ctx)
		var linkBuilders []*ent.WorkSessionTaskCreate
		for i, session := range created {
			for _, l := range links[start+i] {
				linkBuilders = append(linkBuilders, db.Client.WorkSessionTask.Create().
					SetSessionID(session.ID).SetTaskID(l.task).SetFromAt(*l.from).SetNillableUntilAt(l.until))
			}
		}
		db.Client.WorkSessionTask.CreateBulk(linkBuilders...).ExecX(ctx)
	}

	// Quem está com o ponto aberto agora.
	for _, o := range openNow {
		st := byTaskName[o.project+"/"+o.task]
		startedAt := now.Add(-time.Duration(o.minutes) * time.Minute)
		session := db.Client.WorkSession.Create().
			SetProjectID(uuid.MustParse(seeded[o.project].id)).
			SetPersonID(person[o.person].PersonID).
			SetStartAt(startedAt).
			SetPayRateCents(seeded[o.project].rates[o.person]).
			SetNillableBillRateCents(seeded[o.project].billRate).
			SetOwnerHours(o.person == people[0].key).
			SaveX(ctx)
		db.Client.WorkSessionTask.Create().
			SetSessionID(session.ID).SetTaskID(st.task.ID).SetFromAt(startedAt).
			ExecX(ctx)
	}

	// Bater o ponto põe a tarefa em progresso, mas o seed grava as sessões direto no banco. Para o
	// quadro ficar coerente, a tarefa com tempo registrado não fica no backlog, e a que está com
	// o ponto aberto agora está em progresso, seja qual for o status que o seed lhe deu.
	_, err = db.Raw.ExecContext(ctx, `UPDATE tasks SET status = 'in_progress'
		WHERE (status = 'backlog' AND id IN (SELECT task_id FROM work_session_tasks))
		   OR id IN (SELECT l.task_id FROM work_session_tasks l
		             JOIN work_sessions s ON s.id = l.session_id WHERE s.end_at IS NULL)`)
	must(err)

	unassigned := 0
	for _, t := range tasks {
		if t.assignee == "" {
			unassigned++
		}
	}
	fmt.Println("Dados de demonstração criados.")
	fmt.Println()
	fmt.Printf("  Organização: Jatobá Software, uma software house com %d pessoas\n", len(people))
	fmt.Printf("  %d clientes, %d projetos (um interno), %d tarefas (%d sem responsável)\n",
		len(customers), len(projects), len(tasks), unassigned)
	fmt.Printf("  %d sessões fechadas nos últimos %d dias e %d com o ponto aberto agora\n", closed, historyDays, len(openNow))
	fmt.Printf("  %d integrações sem credencial: informe o token na aba Integrações para usá-las\n", len(integrations))
	fmt.Println()
	fmt.Printf("  Dono:    %s / %s\n", people[0].email, password)
	for _, p := range people[1:] {
		role := "Membro: "
		if p.role == "admin" {
			role = "Admin:  "
		}
		fmt.Printf("  %s %s / %s\n", role, p.email, password)
	}
}

// linkTask vincula a tarefa a um item da integração do projeto. link tem o
// formato "gh:42", "gl:17" ou "tr:AbCd1234".
func linkTask(ctx context.Context, client *ent.Client, integrationOf map[string]*ent.Integration, projectKey string, taskID uuid.UUID, link string) {
	kinds := map[string]string{"gh": "github", "gl": "gitlab", "tr": "trello"}
	kind, item := kinds[link[:2]], link[3:]
	it, ok := integrationOf[projectKey+"/"+kind]
	if !ok {
		log.Fatalf("o projeto %s não tem integração %s para o item %s", projectKey, kind, link)
	}
	var url string
	switch kind {
	case "github":
		url = "https://github.com/" + it.Metadata["repo"].(string) + "/issues/" + item
	case "gitlab":
		url = "https://gitlab.com/" + it.Metadata["project_url"].(string) + "/-/issues/" + item
	default:
		url = "https://trello.com/c/" + item
	}
	if _, err := strconv.Atoi(item); kind != "trello" && err != nil {
		log.Fatalf("item inválido: %s", link)
	}
	client.Task.UpdateOneID(taskID).
		SetExternalIntegrationID(it.ID).
		SetExternalItemID(item).
		SetExternalItemURL(url).
		ExecX(ctx)
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
