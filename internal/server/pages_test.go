package server_test

import (
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"

	"working-time-tracker/internal/template"
	"working-time-tracker/web"
)

// pagePaths são as páginas logadas de uma organização com um projeto que todo
// mundo vê. As abas de gestão da organização (orgPagePaths) são só de admins.
func pagePaths(orgID, projectID string) []string {
	return []string{
		"/orgs/" + orgID,
		"/orgs/" + orgID + "/about",
		"/profile",
		"/projects/" + projectID + "/overview",
		"/projects/" + projectID + "/tasks",
		"/projects/" + projectID + "/collaborators",
	}
}

// managementPagePaths são as abas da Gestão do projeto, só de admins.
func managementPagePaths(projectID string) []string {
	return []string{
		"/projects/" + projectID + "/management/overview",
		"/projects/" + projectID + "/management/teams",
		"/projects/" + projectID + "/management/integrations",
		"/projects/" + projectID + "/management/settings",
	}
}

// orgPagePaths são as abas de gestão da organização: Colaboradores, Clientes e Projetos.
func orgPagePaths(orgID string) []string {
	return []string{
		"/orgs/" + orgID + "/people",
		"/orgs/" + orgID + "/customers",
		"/orgs/" + orgID + "/projects",
	}
}

func TestPages_RenderForAdminAndMember(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	projectID := createProject(t, e, admin, "Projeto Alfa")

	for _, who := range []account{admin, member} {
		for _, path := range pagePaths(admin.orgID, projectID) {
			rec := do(e, "GET", path, "", who.session)
			if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
				t.Errorf("GET %s = %d %s, want 200 html", path, rec.Code, rec.Header().Get("Content-Type"))
				continue
			}
			if strings.HasPrefix(path, "/projects/") && !strings.Contains(rec.Body.String(), "Projeto Alfa") {
				t.Errorf("GET %s does not show the project name", path)
			}
		}
	}

	// A Gestão é só de admins: o membro recebe "Página não encontrada".
	for _, path := range managementPagePaths(projectID) {
		if rec := do(e, "GET", path, "", admin.session); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Projeto Alfa") {
			t.Errorf("admin GET %s = %d, want 200 with the project name", path, rec.Code)
		}
		if rec := do(e, "GET", path, "", member.session); rec.Code != http.StatusNotFound {
			t.Errorf("member GET %s = %d, want 404", path, rec.Code)
		}
	}

	// Ações de admin não aparecem para membros.
	rec := do(e, "GET", "/orgs/"+admin.orgID, "", member.session)
	if strings.Contains(rec.Body.String(), "Novo projeto") {
		t.Error("member sees the 'Novo projeto' button")
	}
	rec = do(e, "GET", "/orgs/"+admin.orgID, "", admin.session)
	if !strings.Contains(rec.Body.String(), "Novo projeto") {
		t.Error("admin does not see the 'Novo projeto' button")
	}
}

// As abas de gestão da organização são só de admins: membros não veem os links
// e recebem "Página não encontrada" se abrirem o endereço.
func TestPages_OrgTabsAreAdminOnly(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")

	about := "/orgs/" + admin.orgID + "/about"
	// A edição não é uma aba: o admin chega nela pelo botão Editar da aba Sobre.
	edit := "/orgs/" + admin.orgID + "/settings"

	for _, path := range append(orgPagePaths(admin.orgID), edit) {
		link := `href="` + path + `"`
		rec := do(e, "GET", path, "", admin.session)
		if rec.Code != http.StatusOK {
			t.Errorf("admin GET %s = %d, want 200", path, rec.Code)
		}
		body := rec.Body.String()
		// Cada aba mostra as outras, então uma leva à outra.
		for _, tab := range orgPagePaths(admin.orgID) {
			if !strings.Contains(body, `href="`+tab+`"`) {
				t.Errorf("admin GET %s does not link to the tab %s", path, tab)
			}
		}
		if !strings.Contains(body, "Colaboradores") {
			t.Errorf("admin GET %s does not show the Colaboradores tab", path)
		}
		if path != edit && strings.Contains(body, `href="`+edit+`"`) {
			t.Errorf("admin GET %s still links to the edit page as a tab", path)
		}
		if path == edit && !strings.Contains(body, `href="`+about+`"`) {
			t.Error("the edit page does not link back to the about tab")
		}
		if rec := do(e, "GET", path, "", member.session); rec.Code != http.StatusNotFound {
			t.Errorf("member GET %s = %d, want 404", path, rec.Code)
		}
		if rec := do(e, "GET", "/orgs/"+admin.orgID, "", member.session); strings.Contains(rec.Body.String(), link) {
			t.Errorf("member sees a link to %s", path)
		}
		if rec := do(e, "GET", path, "", ""); rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/login?next=") {
			t.Errorf("GET %s without session = %d to %q, want 303 to /login", path, rec.Code, rec.Header().Get("Location"))
		}
	}
	// O item do menu leva à aba Sobre, que todo mundo lê. Só o admin vê, nela,
	// os links para as abas de gestão e o botão Editar.
	for _, who := range []account{admin, member} {
		if rec := do(e, "GET", "/orgs/"+admin.orgID, "", who.session); !strings.Contains(rec.Body.String(), `href="`+about+`"`) {
			t.Error("the menu does not link to the organization page")
		}
	}
	adminAbout := do(e, "GET", about, "", admin.session).Body.String()
	memberAbout := do(e, "GET", about, "", member.session).Body.String()
	for _, path := range append(orgPagePaths(admin.orgID), edit) {
		link := `href="` + path + `"`
		if !strings.Contains(adminAbout, link) {
			t.Errorf("admin about page does not link to %s", path)
		}
		if strings.Contains(memberAbout, link) {
			t.Errorf("member about page links to %s", path)
		}
	}
}

// A aba Projetos do admin é uma tabela de gestão; a página inicial, que todo
// mundo abre, continua com os cartões.
func TestPages_OrgProjectsTabIsATable(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	home := "/orgs/" + admin.orgID

	tab := do(e, "GET", home+"/projects", "", admin.session).Body.String()
	for _, want := range []string{"<th>Projeto</th>", "<th>Cliente</th>", ">Pessoas</th>", "<th>Tarefas</th>", `class="row-link"`} {
		if !strings.Contains(tab, want) {
			t.Errorf("projects tab does not contain %q", want)
		}
	}
	if strings.Contains(tab, "project-card") {
		t.Error("projects tab still shows the project cards")
	}
	for who, session := range map[string]string{"admin": admin.session, "member": member.session} {
		body := do(e, "GET", home, "", session).Body.String()
		if !strings.Contains(body, "project-card") || strings.Contains(body, `class="row-link"`) {
			t.Errorf("%s home page should show the cards and not the table", who)
		}
	}
}

// O modal é um só, no layout base: sem ele, o formulário que uma página teleporta
// some sem aviso. Os ícones vêm do cdnjs, sempre com SRI.
func TestPages_ModalHostAndIcons(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	projectID := createProject(t, e, admin, "Projeto Alfa")

	for _, path := range append(append(pagePaths(admin.orgID, projectID), managementPagePaths(projectID)...), orgPagePaths(admin.orgID)...) {
		body := do(e, "GET", path, "", admin.session).Body.String()
		if n := strings.Count(body, `id="modal-root"`); n != 1 {
			t.Errorf("GET %s has %d modal hosts, want 1", path, n)
		}
		if !strings.Contains(body, "/font-awesome/") || !strings.Contains(body, `integrity="sha512-`) {
			t.Errorf("GET %s does not load the icon font with SRI", path)
		}
	}

	// As telas que usam o modal: o formulário é teleportado e um botão abre.
	for page, wants := range map[string][]string{
		"customers": {"Novo cliente", `x-teleport="#modal-root"`, `id="customer-name"`},
		"people":    {"Adicionar colaborador", `x-teleport="#modal-root"`, `id="invite-email"`, "Integrantes", "Convites pendentes"},
		"projects":  {"Novo projeto", `x-teleport="#modal-root"`, `id="project-name"`, `id="project-customer"`, `id="project-bill-rate"`},
	} {
		body := do(e, "GET", "/orgs/"+admin.orgID+"/"+page, "", admin.session).Body.String()
		for _, want := range wants {
			if !strings.Contains(body, want) {
				t.Errorf("%s page does not contain %q", page, want)
			}
		}
	}
}

// O resumo da organização aparece no cabeçalho das páginas dela e na página
// inicial, e a organização chega pronta ao JavaScript.
func TestPages_OrgSummaryInHeader(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	orgPath := "/orgs/" + admin.orgID

	// Sem resumo, a página inicial diz o que cada um encontra nela: o admin, a visão geral; o membro, os projetos.
	for who, want := range map[string]string{member.session: "Os projetos da organização.", admin.session: "Como a organização está indo"} {
		if body := do(e, "GET", orgPath, "", who).Body.String(); !strings.Contains(body, want) {
			t.Errorf("home page without a summary does not show the default lede %q", want)
		}
	}

	rec := do(e, "PATCH", "/api/orgs/"+admin.orgID, `{"summary":"Entregas <rápidas> no mesmo dia","industry":"Logística"}`, admin.session)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH org = %d: %s", rec.Code, rec.Body.String())
	}
	for _, path := range []string{orgPath, orgPath + "/about"} {
		body := do(e, "GET", path, "", member.session).Body.String()
		if !strings.Contains(body, "Entregas &lt;rápidas&gt; no mesmo dia") {
			t.Errorf("GET %s does not show the escaped summary", path)
		}
		if strings.Contains(body, "<rápidas>") {
			t.Errorf("GET %s shows the summary without escaping", path)
		}
		if !strings.Contains(body, `"industry":"Logística"`) {
			t.Errorf("GET %s does not pass the organization to the page script", path)
		}
	}
}

// A página da organização e o perfil são separados, e a barra superior leva
// ao perfil.
func TestPages_OrgSettingsAndProfileAreSeparate(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")

	settings := do(e, "GET", "/orgs/"+admin.orgID+"/settings", "", admin.session).Body.String()
	if !strings.Contains(settings, `id="org-name"`) || !strings.Contains(settings, "Excluir organização") {
		t.Error("org settings page does not show the organization name and deletion")
	}
	if strings.Contains(settings, `id="me-email"`) || strings.Contains(settings, `id="pw-current"`) {
		t.Error("org settings page still shows the profile or password form")
	}
	if !strings.Contains(settings, `href="/profile"`) {
		t.Error("top bar does not link to the profile page")
	}

	profile := do(e, "GET", "/profile", "", admin.session).Body.String()
	if !strings.Contains(profile, `id="me-email"`) || !strings.Contains(profile, `id="pw-current"`) {
		t.Error("profile page does not show the profile and password forms")
	}
	if strings.Contains(profile, `id="org-name"`) || strings.Contains(profile, "Excluir organização") {
		t.Error("profile page shows organization settings")
	}
}

// A aba Colaboradores do projeto ficou no lugar de Times e de Valores. Todo
// mundo a abre e escolhe entre duas visões, a lista de pessoas e os times,
// com as pessoas em páginas nas duas. Só o admin vê os valores por hora e as
// ações: os formulários de adicionar pessoa, de novo time, de editar time e do
// colaborador ficam no modal. A linha da tabela e o cartão do time não têm
// campo nem ação direta: o valor, os times e a saída do projeto mudam no modal
// do colaborador. No de adicionar pessoa o valor por hora é obrigatório e fica
// acima do time, e o de editar time só lista quem já está no projeto. A aba
// Valores não existe mais, e o cartão de cobrança das configurações segue só
// de admins.
func TestPages_ProjectCollaboratorsTab(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	other := signup(t, e, "Outra", "caio@outra.com")
	projectID := createProject(t, e, admin, "Projeto Alfa")
	tab := "/projects/" + projectID + "/management/teams"
	rates := "/projects/" + projectID + "/rates"

	adminPage := do(e, "GET", tab, "", admin.session).Body.String()
	// A aba mora na Gestão, que é só de admins: o membro recebe o 404.
	if rec := do(e, "GET", tab, "", member.session); rec.Code != http.StatusNotFound {
		t.Errorf("member GET %s = %d, want 404", tab, rec.Code)
	}
	for who, body := range map[string]string{"admin": adminPage} {
		if !strings.Contains(body, `href="`+tab+`" aria-current="page"`) || !strings.Contains(body, " Colaboradores</a>") {
			t.Errorf("%s: the tab bar does not show Colaboradores as the current tab", who)
		}
		if strings.Contains(body, `href="`+rates+`"`) || strings.Contains(body, " Valores</a>") || strings.Contains(body, " Times</a>") {
			t.Errorf("%s: the tab bar still shows the Times or the Valores tab", who)
		}
		for _, want := range []string{
			"Sem time", `aria-label="Buscar colaborador por nome ou e-mail"`,
			// As duas visões, cada uma no seu painel.
			`role="tablist"`, `id="view-people-tab"`, `id="view-teams-tab"`,
			`id="view-people" role="tabpanel"`, `id="view-teams" role="tabpanel"`,
			// As pessoas vêm em páginas na tabela e em cada cartão de time.
			`x-for="c in peoplePage().rows"`, `aria-label="Páginas da lista de pessoas"`,
			`x-for="m in teamPage(team).rows"`, `title="Próximos integrantes"`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: the page does not contain %q", who, want)
			}
		}
		// Renomear, excluir e pôr ou tirar gente saíram do cartão do time, e o
		// campo de valor e o botão de remover saíram da linha da pessoa.
		for _, gone := range []string{
			`title="Renomear"`, `title="Remover do time"`, "Adicionar pessoa…", "addMember(", "removeMember(", "rename(",
			"saveRate(", `title="Remover do projeto"`, "input-money", "<h2>Pessoas</h2>", "<h2>Times</h2>",
		} {
			if strings.Contains(body, gone) {
				t.Errorf("%s: the collaborators tab still has %q", who, gone)
			}
		}
	}
	for _, adminOnly := range []string{
		"Adicionar pessoa", "Novo time", `x-teleport="#modal-root"`, `id="collab-search"`, `id="collab-rate"`, `id="collab-team"`, `id="team-name"`,
		`$t('collab.col_rate'`, "Margem por hora",
		// Os dois blocos de dinheiro do resumo, cada um com a conta explicada numa dica.
		"Margem por hora somada atual", "Valor das horas registradas",
		`x-data="tip"`, `aria-label="O que é a margem por hora somada atual"`, `aria-label="O que é o valor das horas registradas"`,
		`id="tip-margin" role="note"`, `id="tip-recorded" role="note"`,
		// Editar o time é um botão só de ícone, com o nome da ação.
		`title="Editar time"`, `:aria-label="$t('collab.edit_team_label', { name: team.name })"`,
		// O colaborador abre pela linha da tabela, pelo lápis dela e pela pessoa no cartão do time.
		`class="row-link" @click="if (!$event.target.closest('button')) openPerson(c)"`,
		`title="Editar colaborador"`, `class="list-btn" title="Editar colaborador" @click="openPerson(m)"`,
		// O modal do colaborador: o valor, os times em caixas de marcar e a saída do projeto.
		`x-show="$store.modal.name === 'collab-edit'"`,
		`id="collab-edit-rate" type="text" inputmode="decimal" class="num" placeholder="0,00" :required="!person.is_owner"`,
		`type="checkbox" :value="t.id" x-model="person.team_ids"`, "Tirar do projeto", "savePerson()", "removePerson()",
		// O grupo de permissões: o modal do colaborador escolhe, e adicionar pessoa é em dois passos.
		`class="presets" role="radiogroup"`, `x-for="p in presetChoices()"`, `x-model="preset"`,
		`class="wizard-steps"`, `x-show="addStep === 1"`, `x-show="addStep === 2"`, "Quem entra", "Permissões", "addSubmit()",
		// O modal de editar time: o nome, os integrantes em caixas de marcar e a exclusão.
		`x-show="$store.modal.name === 'team-edit'"`, `id="team-edit-name"`, `id="team-edit-search"`,
		`type="checkbox" :value="p.id" x-model="edit.member_ids"`, "Excluir time", "saveTeam()", "removeTeam()",
	} {
		if !strings.Contains(adminPage, adminOnly) {
			t.Errorf("admin does not see %q on the collaborators tab", adminOnly)
		}
	}

	// O formulário de adicionar pessoa vai da busca até o formulário de novo time. O valor e o
	// time ficam no primeiro passo, e o grupo, no segundo.
	addForm := adminPage[strings.Index(adminPage, `id="collab-search"`):strings.Index(adminPage, `id="team-name"`)]
	steps := adminPage[strings.Index(adminPage, `x-show="$store.modal.name === 'collab-add'"`):strings.Index(adminPage, `id="team-name"`)]
	if first, second, group := strings.Index(steps, `<div class="stack" x-show="addStep === 1">`), strings.Index(steps, `<div class="stack" x-show="addStep === 2">`), strings.Index(steps, `class="presets"`); first < 0 || second < first || group < second {
		t.Errorf("the add person form does not put the group in the second step (steps at %d and %d, group at %d)", first, second, group)
	}
	if !strings.Contains(addForm, `id="collab-rate" type="text" inputmode="decimal" class="num" placeholder="0,00" :required="!addIsOwner()"`) {
		t.Error("the hourly rate is not a required field of the add person form (except for the owner)")
	}
	if rate, team := strings.Index(addForm, `id="collab-rate"`), strings.Index(addForm, `id="collab-team"`); rate < 0 || team < rate {
		t.Error("the add person form does not show the hourly rate above the team")
	}
	if strings.Contains(addForm, `class="fields"`) {
		t.Error("the add person form puts the hourly rate and the team side by side")
	}
	// Ninguém entra no projeto sem valor: o modal de editar time só lista quem
	// já está nele, e não há mais o que contar em "sem valor por hora".
	for _, gone := range []string{">fora do projeto<", "entra nele sem valor por hora", "Sem valor por hora<", "ela não bate ponto"} {
		if strings.Contains(adminPage, gone) {
			t.Errorf("the collaborators tab still shows %q", gone)
		}
	}

	if rec := do(e, "GET", tab, "", other.session); rec.Code != http.StatusNotFound {
		t.Errorf("another organization GET %s = %d, want 404", tab, rec.Code)
	}
	for who, session := range map[string]string{"admin": admin.session, "member": member.session} {
		if rec := do(e, "GET", rates, "", session); rec.Code != http.StatusNotFound {
			t.Errorf("%s GET %s = %d, want 404: the tab is gone", who, rates, rec.Code)
		}
	}

	settings := "/projects/" + projectID + "/management/settings"
	if body := do(e, "GET", settings, "", admin.session).Body.String(); !strings.Contains(body, `id="ps-bill-rate"`) {
		t.Error("admin does not see the billing card on the project settings")
	}
	if rec := do(e, "GET", settings, "", member.session); rec.Code != http.StatusNotFound {
		t.Errorf("member GET %s = %d, want 404", settings, rec.Code)
	}
}

// A aba Tarefas tem a linha de filtros (busca, prazo, prioridade, status e etiqueta), o filtro de
// responsável dentro da lista Com responsável, sem a caixa "Só as minhas tarefas" (as suas ficam em
// Minhas tarefas), e o formulário de nova tarefa no modal. Admin e membro veem a mesma tela.
func TestPages_ProjectTasksTab(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	projectID := createProject(t, e, admin, "Projeto Alfa")

	for who, session := range map[string]string{"admin": admin.session, "member": member.session} {
		body := do(e, "GET", "/projects/"+projectID+"/tasks", "", session).Body.String()
		for _, want := range []string{
			`aria-label="Buscar tarefa pelo nome"`, `aria-label="Filtrar por responsável"`, `aria-label="Filtrar por prazo"`,
			`class="pager"`,
			"Nova tarefa", `x-teleport="#modal-root"`, `x-show="$store.modal.name === 'task-new'"`, `id="task-name"`, `id="task-assignee"`,
			// A página diz o que é e tem duas listas, cada uma com a sua página: as sem responsável e as que têm.
			`class="lede muted"`, "Aqui estão as tarefas que ninguém pegou", `x-for="key in taskLists"`, `go(key, lists[key].page + 1)`,
			// Uma tarefa pode ficar sem responsável, e a lista mostra o que está disponível.
			"Lista de tarefas", "Atribuir a mim", "Outra pessoa", `value="none" x-model="draft.assign"`,
			// Prioridade e etiqueta: os filtros (várias de cada), a coluna e os campos do modal.
			`role="group" aria-label="Filtrar por prioridade"`, `role="group" aria-label="Filtrar por etiqueta"`, `toggleFilter('priority', p.value)`,
			`toggleFilter('label', l.id)`, "<th>Prioridade</th>", `id="task-priority"`, `x-model="draft.label_ids"`,
			// O status: o filtro, a coluna e o campo do modal (que só aparece ao editar).
			`role="group" aria-label="Filtrar por status"`, `toggleFilter('status', s.value)`, "<th>Status</th>", `id="task-status"`, `<template x-if="hasStatus">`,
			// O modal é um passo a passo: nome e descrição (em Markdown, com prévia), e depois o resto.
			`class="wizard-steps"`, `submitStep('create')`, "Escrever", "Pré-visualizar", `id="task-preview"`, "Próximo", "Voltar", `id="task-deadline"`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: the tasks tab does not contain %q", who, want)
			}
		}
		// Só admins criam etiqueta: o membro só escolhe.
		if has := strings.Contains(body, `id="task-new-label"`); has != (who == "admin") {
			t.Errorf("%s: the new label field is present = %v", who, has)
		}
		if strings.Contains(body, `id="task-assignee" required`) {
			t.Errorf("%s: the assignee of a new task is still required", who)
		}
		// Sem a caixa "Só as minhas tarefas": ela desligava a busca e o responsável, e as tarefas da pessoa já
		// têm a aba delas. Nada na tela fica desligado por ela, e o script não guarda mais o estado.
		for _, not := range []string{"Só as minhas tarefas", "toggleMine", "filters.mine", `:disabled="filters.`, "listShown"} {
			if strings.Contains(body, not) {
				t.Errorf("%s: the tasks tab still has %q", who, not)
			}
		}
		// O responsável filtra só a lista Com responsável: o select fica dentro dela, depois dos filtros do
		// topo e antes da tabela, e só aparece nessa lista.
		due, list, assignee, table := strings.Index(body, `aria-label="Filtrar por prazo"`), strings.Index(body, `x-for="key in taskLists"`),
			strings.Index(body, `aria-label="Filtrar por responsável"`), strings.Index(body, "<table>")
		if !(due < list && list < assignee && assignee < table) {
			t.Errorf("%s: the assignee filter is not inside the lists, between the top filters and the table", who)
		}
		if !strings.Contains(body, `x-show="key === 'taken' && (lists.taken.total > 0 || filters.assignee)"`) {
			t.Errorf("%s: the assignee filter is not limited to the list of the tasks with an assignee", who)
		}
	}
}

// Minhas tarefas é uma aba do dia a dia, igual para admin e membro: uma lista por status, na
// ordem em que o trabalho anda, com a de fechadas recolhida. Está entre o Início e a Lista de
// tarefas na barra de abas, e outra organização recebe 404.
func TestPages_ProjectMyTasksTab(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	outsider := signup(t, e, "Outra", "zeca@test.com")
	projectID := createProject(t, e, admin, "Projeto Alfa")
	mine := "/projects/" + projectID + "/my-tasks"

	for who, session := range map[string]string{"admin": admin.session, "member": member.session} {
		rec := do(e, "GET", mine, "", session)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: GET my-tasks = %d, want 200", who, rec.Code)
		}
		body := rec.Body.String()
		for _, want := range []string{
			`x-data="projectMyTasks"`, "Tudo o que está com você", `x-for="s in statuses"`, `:aria-expanded="open[s.value].toString()"`,
			`class="fold-toggle"`, "Você não tem tarefas neste projeto.", "Ver a lista de tarefas", `@click="startTask(t)"`, `class="clock"`,
			// A aba é a atual, e fica entre o Início e a Lista de tarefas.
			`/my-tasks" aria-current="page"`, "Minhas tarefas",
			// A prioridade e o status têm cor própria; "sem prioridade" também tem selo (pontilhado).
			`class="badge" :class="priorityClass(t.priority)" x-text="WTT.fmt.priority(t.priority)"`, `class="badge num" :class="statusClass(s.value)"`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: the my tasks tab does not contain %q", who, want)
			}
		}
		if strings.Contains(body, `x-show="t.priority !== 'none'"`) {
			t.Errorf("%s: the my tasks tab still hides the priority badge when there is none", who)
		}
		home, mineTab, list := strings.Index(body, ">Início</a>"), strings.Index(body, `/my-tasks"`), strings.Index(body, `/tasks"`)
		if !(home < mineTab && mineTab < list) {
			t.Errorf("%s: the tabs are not Início, Minhas tarefas, Lista de tarefas", who)
		}
		// As outras abas do projeto também levam a ela.
		if other := do(e, "GET", "/projects/"+projectID+"/tasks", "", session).Body.String(); !strings.Contains(other, `href="/projects/`+projectID+`/my-tasks"`) {
			t.Errorf("%s: the task list has no link to Minhas tarefas", who)
		}
	}
	if rec := do(e, "GET", mine, "", outsider.session); rec.Code != http.StatusNotFound {
		t.Errorf("another organization GET my-tasks = %d, want 404", rec.Code)
	}
}

// O Início do projeto é a mesma tela para admin e membro: o relógio e as tarefas de
// quem olha (o aviso de quem está sem valor por hora fica acima dos cartões), o tempo
// dele e as suas sessões, com filtros de data e tarefa, páginas e só o valor que a
// pessoa ganha, sem a coluna de pessoa nem custo, receita e margem. O Ponto, que era
// outra aba, está aqui dentro, e o endereço dele leva para cá.
func TestPages_ProjectMyOverviewTab(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	projectID := createProject(t, e, admin, "Projeto Alfa")
	overview := "/projects/" + projectID + "/overview"
	const warning = "ainda não tem valor por hora neste projeto"
	pages := map[string]string{}

	for who, session := range map[string]string{"admin": admin.session, "member": member.session} {
		rec := do(e, "GET", overview, "", session)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s GET %s = %d, want 200", who, overview, rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `href="`+overview+`" aria-current="page"`) {
			t.Errorf("%s: the tab bar does not mark Visão geral as the current tab", who)
		}
		first, tasks := strings.Index(body, `href="`+overview+`"`), strings.Index(body, `href="/projects/`+projectID+`/tasks"`)
		if first < 0 || tasks < 0 || first > tasks {
			t.Errorf("%s: Visão geral is not the first tab", who)
		}
		if n := strings.Count(body, warning); n != 1 {
			t.Errorf("%s: the page has the rate warning %d times, want once", who, n)
		}
		if at, clock := strings.Index(body, warning), strings.Index(body, `class="clock"`); at < 0 || clock < 0 || at > clock {
			t.Errorf("%s: the rate warning is not above the clock card", who)
		}
		pages[who] = body
		for _, want := range []string{
			`class="grid-aside items-stretch"`, "Suas tarefas", `class="task-cards"`, `class="task-card card-link"`, "startTask(t)", "sessionValue()", "nesta sessão", "Início</a>",
			`x-data="projectMyOverview"`, "Seu tempo neste projeto", "Nesta semana",
			`aria-label="Filtrar por data"`, `aria-label="Filtrar por tarefa"`, "<h3>Totais</h3>", `<div class="k">Tempo</div>`, `<div class="k">Seu valor</div>`,
			`class="pager"`, "<table>",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: the overview does not contain %q", who, want)
			}
		}
		// O cartão de tarefa é o link: não tem o botão Ver detalhes. Mostra o status, a prioridade, o
		// prazo e as etiquetas (a ordem e os estilos ficam no teste do pacote web).
		if strings.Contains(body, "Ver detalhes") {
			t.Errorf("%s: the task cards still have the View details button", who)
		}
		for _, want := range []string{"statusClass(t.status)", "priorityClass(t.priority)", "cardDeadlineLabel(t)", "t.labels", "Sem etiquetas"} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: the task cards do not contain %q", who, want)
			}
		}
		for _, gone := range []string{`aria-label="Filtrar por pessoa"`, `<div class="k">Custo</div>`, `<div class="k">Receita</div>`, "Margem (receita menos custo)", "<th>Pessoa</th>"} {
			if strings.Contains(body, gone) {
				t.Errorf("%s: the personal overview shows %q", who, gone)
			}
		}
		// A aba Ponto deixou de existir: nem na barra, nem como página.
		if strings.Contains(body, `/time-tracking`) || strings.Contains(body, ">Ponto</a>") {
			t.Errorf("%s: the tab bar still links to the Ponto tab", who)
		}
		old := do(e, "GET", "/projects/"+projectID+"/time-tracking", "", session)
		if old.Code != http.StatusSeeOther || old.Header().Get("Location") != overview {
			t.Errorf("%s: the old Ponto address answers %d to %q, want a redirect to %s", who, old.Code, old.Header().Get("Location"), overview)
		}
	}
	if !strings.Contains(pages["admin"], `href="/projects/`+projectID+`/management/teams">Colaboradores</a>`) || !strings.Contains(pages["member"], "Peça a um admin para definir") {
		t.Error("the rate warning should send the admin to the collaborators tab and the member to an admin")
	}
}

// A aba Colaboradores de fora da Gestão é a mesma para todos, e só para ler: as
// duas visões e as páginas, sem valor por hora, sem margem e sem nenhuma ação,
// nem para o admin, que edita na Gestão.
func TestPages_CollaboratorsTabIsReadOnly(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	projectID := createProject(t, e, admin, "Projeto Alfa")
	tab := "/projects/" + projectID + "/collaborators"

	for who, session := range map[string]string{"admin": admin.session, "member": member.session} {
		rec := do(e, "GET", tab, "", session)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s GET %s = %d, want 200", who, tab, rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `href="`+tab+`" aria-current="page"`) {
			t.Errorf("%s: the tab bar does not mark Colaboradores as the current tab", who)
		}
		for _, want := range []string{`x-data="projectTeams"`, `role="tablist"`, `id="view-people"`, `id="view-teams"`, `x-for="c in peoplePage().rows"`, `"readonly":true`} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: the read-only tab does not contain %q", who, want)
			}
		}
		for _, gone := range []string{
			"Adicionar pessoa", "Novo time", `x-teleport="#modal-root"`, "Margem por hora", "Valor das horas registradas",
			`title="Editar time"`, `title="Editar colaborador"`, "openPerson(", "openEdit(",
		} {
			if strings.Contains(body, gone) {
				t.Errorf("%s: the read-only collaborators tab still has %q", who, gone)
			}
		}
	}
}

// A aba Configurações do projeto só mostra: os campos ficam no modal Editar
// projeto, só de admins, junto com a exclusão. A duração da sprint é uma lista
// de opções, e a jornada semanal saiu do projeto: é da pessoa, e aparece na aba
// Colaboradores da organização e no perfil.
func TestPages_ProjectSettingsAndWeeklyHours(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	projectID := createProject(t, e, admin, "Projeto Alfa")
	settings := "/projects/" + projectID + "/management/settings"

	adminPage := do(e, "GET", settings, "", admin.session).Body.String()
	// A aba mora na Gestão, só de admins: o membro recebe o 404.
	if rec := do(e, "GET", settings, "", member.session); rec.Code != http.StatusNotFound {
		t.Errorf("member GET %s = %d, want 404", settings, rec.Code)
	}
	for who, body := range map[string]string{"admin": adminPage} {
		for _, want := range []string{`x-data="projectSettings"`, `<dl class="facts">`, `x-for="f in routine()"`, "<dt>Cliente</dt>"} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: the settings tab does not contain %q", who, want)
			}
		}
		for _, gone := range []string{"Jornada semanal", "weekly_hours", `type="number"`, `class="grid-aside"`} {
			if strings.Contains(body, gone) {
				t.Errorf("%s: the settings tab still contains %q", who, gone)
			}
		}
	}

	// Só o admin tem o botão Editar e o modal, que fica fora da página, no #modal-root.
	modal := strings.Index(adminPage, `x-show="$store.modal.name === 'project-edit'"`)
	if modal < 0 || !strings.Contains(adminPage, `@click="openEdit()"`) {
		t.Fatal("admin does not have the edit button and the edit project modal")
	}
	if teleport := strings.LastIndex(adminPage[:modal], `<template x-teleport="#modal-root">`); teleport < 0 {
		t.Error("the edit project form is not teleported to the modal")
	}
	for _, field := range []string{
		`<input id="ps-name"`, `<select id="ps-customer"`, `<input id="ps-bill-rate"`,
		`<select id="ps-sprint" x-model.number="draft.sprint_duration_days">`, `x-for="o in sprintChoices()"`,
		`<input id="routine-daily"`, `<select id="routine-weekly-day"`, `<input id="routine-weekly-time"`, `<textarea id="ps-description"`,
		`<select id="meeting-day"`, `<input id="meeting-time"`,
		"Excluir projeto", "<dt>Valor cobrado por hora</dt>",
	} {
		at := strings.Index(adminPage, field)
		if at < 0 {
			t.Errorf("admin: the settings tab does not contain %q", field)
		} else if at < modal && strings.HasPrefix(field, "<") && !strings.HasPrefix(field, "<dt>") {
			t.Errorf("admin: %q is on the page, outside the modal", field)
		}
	}

	// O Novo projeto oferece as mesmas durações e não pergunta a jornada.
	for _, path := range []string{"/orgs/" + admin.orgID, "/orgs/" + admin.orgID + "/projects"} {
		body := do(e, "GET", path, "", admin.session).Body.String()
		if !strings.Contains(body, `<select id="project-sprint" x-model.number="draft.sprint_duration_days">`) || !strings.Contains(body, `x-for="o in WTT.sprintOptions"`) {
			t.Errorf("%s: the new project form does not offer the sprint durations in a select", path)
		}
		// Daily e weekly são marcas que revelam o horário, e os campos vêm em três grupos.
		for _, want := range []string{
			`x-model="draft.has_daily"`, `x-model="draft.has_weekly"`, `<input id="routine-daily" type="time" :required="draft.has_daily"`,
			`<select id="routine-weekly-day"`, `<input id="routine-weekly-time" type="time"`,
			"<legend>Projeto</legend>", "<legend>Cliente e cobrança</legend>", "<legend>Rotina do time</legend>",
			`x-model="draft.has_meeting"`, `<select id="meeting-day"`, `<input id="meeting-time" type="time"`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: the new project form does not contain %q", path, want)
			}
		}
		// A reunião com o cliente fica no grupo Cliente e cobrança, e não na rotina do time.
		billing, routine, meeting := strings.Index(body, "<legend>Cliente e cobrança</legend>"), strings.Index(body, "<legend>Rotina do time</legend>"), strings.Index(body, `<select id="meeting-day"`)
		if !(billing < meeting && meeting < routine) {
			t.Errorf("%s: the customer meeting is not inside Cliente e cobrança (billing %d, meeting %d, routine %d)", path, billing, meeting, routine)
		}
		if strings.Contains(body, "project-weekly-hours") || strings.Contains(body, "weekly_hours") {
			t.Errorf("%s: the new project form or the project card still has the weekly hours", path)
		}
	}

	// A jornada é da pessoa: a lista mostra, e um modal aberto pelo lápis altera.
	// O papel continua mudando pelo botão da linha, e o modal não o tem.
	people := do(e, "GET", "/orgs/"+admin.orgID+"/people", "", admin.session).Body.String()
	for _, want := range []string{
		"<th>Jornada semanal</th>", `x-show="$store.modal.name === 'person-edit'"`,
		`<input id="person-weekly-hours" type="number" min="1" max="168"`, `@click="openEdit(p)"`,
	} {
		if !strings.Contains(people, want) {
			t.Errorf("the organization people tab does not contain %q", want)
		}
	}
	if !strings.Contains(people, "setRole(") || strings.Contains(people, `id="person-role"`) {
		t.Error("the role should change on the row button, and not in the weekly hours modal")
	}

	// As permissões da organização estão no mesmo modal, mas só o dono as vê: um admin que
	// não é o dono edita a jornada e nada mais.
	for _, want := range []string{`id="person-permissions-label"`, `x-model="draft.permissions"`, "canGrant(editing)"} {
		if !strings.Contains(people, want) {
			t.Errorf("the owner does not see %q in the people modal", want)
		}
	}
	otherAdmin := invite(t, e, admin, "outro-admin@test.com", "admin")
	if peopleAsAdmin := do(e, "GET", "/orgs/"+admin.orgID+"/people", "", otherAdmin.session).Body.String(); strings.Contains(peopleAsAdmin, "person-permissions-label") ||
		strings.Contains(peopleAsAdmin, `<option value="admin">`) || !strings.Contains(peopleAsAdmin, `id="person-weekly-hours"`) {
		t.Error("an admin who is not the owner should edit the weekly hours only, and not the permissions or the admin role")
	}

	// Cada pessoa lê a própria jornada no perfil, sem campo para alterar.
	profile := do(e, "GET", "/profile", "", member.session).Body.String()
	if !strings.Contains(profile, "<dt>Jornada semanal</dt>") || strings.Contains(profile, "weekly-hours") {
		t.Error("the profile should show the weekly hours as text, without a field")
	}

	// O JavaScript que as páginas carregam conhece as opções e a rota nova.
	for path, wants := range map[string][]string{
		"/static/app.js":           {"sprintOptions", "labels.sprint.long_", "labels.sprint.short_"},
		"/static/pages/project.js": {"'project-edit'", "sprintChoices()"},
		"/static/pages/org.js":     {"/weekly-hours'", "'person-edit'"},
	} {
		body := do(e, "GET", path, "", "").Body.String()
		for _, want := range wants {
			if !strings.Contains(body, want) {
				t.Errorf("%s does not contain %q", path, want)
			}
		}
		if strings.Contains(body, "weekly_hours: Number(this.draft.weekly_hours)") || strings.Contains(body, "form.weekly_hours") {
			t.Errorf("%s still sends the weekly hours of a project", path)
		}
	}
}

func TestPages_RedirectWithoutSession(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	projectID := createProject(t, e, admin, "Projeto")

	for _, path := range append(pagePaths(admin.orgID, projectID), managementPagePaths(projectID)...) {
		rec := do(e, "GET", path, "", "")
		if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/login?next=") {
			t.Errorf("GET %s without session = %d to %q, want 303 to /login", path, rec.Code, rec.Header().Get("Location"))
		}
	}
	if rec := do(e, "GET", "/projects/"+projectID, "", admin.session); rec.Header().Get("Location") != "/projects/"+projectID+"/overview" {
		t.Errorf("GET /projects/:id redirects the admin to %q, want the Visão geral", rec.Header().Get("Location"))
	}
}

func TestPages_StaticAssets(t *testing.T) {
	e := newServer(t)
	for path, contentType := range map[string]string{
		"/static/app.css":          "text/css",
		"/static/app.js":           "javascript",
		"/static/alpine.min.js":    "javascript",
		"/static/marked.min.js":    "javascript",
		"/static/purify.min.js":    "javascript",
		"/static/pages/auth.js":    "javascript",
		"/static/pages/org.js":     "javascript",
		"/static/pages/project.js": "javascript",
	} {
		rec := do(e, "GET", path, "", "")
		body, _ := io.ReadAll(rec.Body)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Type"), contentType) || len(body) == 0 {
			t.Errorf("GET %s = %d %q (%d bytes)", path, rec.Code, rec.Header().Get("Content-Type"), len(body))
		}
	}
}

// Todos os arquivos em templates/pages viram uma página carregada na inicialização.
func TestPages_AllTemplatesLoad(t *testing.T) {
	r, err := template.New(web.FS)
	if err != nil {
		t.Fatalf("load templates: %v", err)
	}
	got := r.Pages()
	sort.Strings(got)
	for _, name := range []string{
		"login", "signup", "invite", "notfound",
		"org_projects", "org_people", "org_settings", "org_about", "org_customers", "profile",
		"project_overview", "project_my_overview", "project_tasks", "project_teams", "project_integrations", "project_settings", "task_detail",
	} {
		i := sort.SearchStrings(got, name)
		if i == len(got) || got[i] != name {
			t.Errorf("page template %q not loaded (loaded: %v)", name, got)
		}
	}
}

func TestPages_TaskDetail(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	other := signup(t, e, "Outra", "bia@outra.com")
	projectID := createProject(t, e, admin, "Projeto Alfa")
	rec := do(e, "POST", "/api/projects/"+projectID+"/teams", `{"name":"Time"}`, admin.session)
	teamID := decode(t, rec)["id"].(string)
	allocate(t, e, admin, projectID, admin.id, 0)
	do(e, "POST", "/api/teams/"+teamID+"/members", `{"person_id":"`+admin.id+`"}`, admin.session)
	rec = do(e, "POST", "/api/projects/"+projectID+"/tasks", `{"name":"Tela de login","assignee_id":"`+admin.id+`"}`, admin.session)
	taskID := decode(t, rec)["id"].(string)

	rec = do(e, "GET", "/tasks/"+taskID, "", admin.session)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Projeto Alfa") || !strings.Contains(rec.Body.String(), taskID) {
		t.Errorf("GET /tasks/:id = %d, want 200 with the project name and task id in the page", rec.Code)
	}
	if rec := do(e, "GET", "/tasks/"+taskID, "", other.session); rec.Code != http.StatusNotFound {
		t.Errorf("task page from another org = %d, want 404", rec.Code)
	}
}

// Na aba Integrações o cartão só mostra a integração. Criar, editar, desativar e
// excluir ficam num modal só, que desenha os campos de cada plataforma a partir dos
// tipos que o servidor entrega no window.BOOT. A tela da tarefa e a lista de tarefas
// usam os mesmos tipos para o rótulo do item vinculado.
func TestPages_ProjectIntegrationsTab(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	projectID := createProject(t, e, admin, "Projeto Alfa")
	// Os três tipos, na ordem da tela, cada um com os campos do seu metadata.
	types := []string{
		`"integration_types":[{"type":"github","label":"GitHub"`, `"key":"repo","label":"Repositório"`,
		`{"type":"gitlab","label":"GitLab"`, `"key":"project_url"`,
		`{"type":"trello","label":"Trello"`, `"key":"api_key"`, `"key":"board_id"`,
	}

	pages := map[string]string{}
	if rec := do(e, "GET", "/projects/"+projectID+"/management/integrations", "", member.session); rec.Code != http.StatusNotFound {
		t.Errorf("member GET the integrations tab = %d, want 404 (the tab is in the Gestão)", rec.Code)
	}
	for who, session := range map[string]string{"admin": admin.session} {
		body := do(e, "GET", "/projects/"+projectID+"/management/integrations", "", session).Body.String()
		pages[who] = body
		for _, want := range append([]string{`x-for="f in facts(it)"`, `x-show="incomplete(it)"`, "it.has_token"}, types...) {
			if !strings.Contains(body, want) {
				t.Errorf("%s: the integrations tab does not contain %q", who, want)
			}
		}
		if github, trello := strings.Index(body, `{"type":"github"`), strings.Index(body, `{"type":"trello"`); github > trello {
			t.Errorf("%s: the integration types are out of order", who)
		}
		// O formulário solto na página e as ações no cartão saíram.
		for _, gone := range []string{`x-show="creating"`, "toggle(it)", "startEdit(", "saveEdit(", "remove(it)", "draft.config", "has_config"} {
			if strings.Contains(body, gone) {
				t.Errorf("%s: the integrations tab still has %q", who, gone)
			}
		}
	}
	for _, adminOnly := range []string{
		"Nova integração", `x-teleport="#modal-root"`, `x-show="$store.modal.name === 'integration-form'"`,
		`role="radiogroup" aria-label="Plataforma"`, `x-model="draft.type"`,
		`id="int-name"`, `id="int-token"`, `autocomplete="new-password"`, `:required="!draft.id"`,
		`x-for="f in typeOf(draft.type).metadata"`, `x-model="draft.metadata[f.key]"`, `x-model="draft.enabled"`,
		`@click="openEdit(it)"`, `title="Editar integração"`, "Excluir integração", `@click="remove()"`,
	} {
		if !strings.Contains(pages["admin"], adminOnly) {
			t.Errorf("admin does not see %q in the integrations tab", adminOnly)
		}
	}

	// Uma tarefa precisa de um responsável que esteja num time do projeto.
	rec := do(e, "POST", "/api/projects/"+projectID+"/teams", `{"name":"Time"}`, admin.session)
	teamID := decode(t, rec)["id"].(string)
	do(e, "PUT", "/api/projects/"+projectID+"/allocations/"+admin.id, `{"pay_rate_cents":0}`, admin.session)
	do(e, "POST", "/api/teams/"+teamID+"/members", `{"person_id":"`+admin.id+`"}`, admin.session)
	rec = do(e, "POST", "/api/projects/"+projectID+"/tasks", `{"name":"Tela de login","assignee_id":"`+admin.id+`"}`, admin.session)
	taskID := decode(t, rec)["id"].(string)

	detail := do(e, "GET", "/tasks/"+taskID, "", member.session).Body.String()
	for _, want := range append([]string{
		`x-text="linkType().item_label || $t('tasks.item')"`, `:inputmode="linkType().item_numeric ? 'numeric' : 'text'"`,
		`:placeholder="linkType().item_placeholder || ''"`, "typeLabel(i.type)",
	}, types...) {
		if !strings.Contains(detail, want) {
			t.Errorf("the task page does not contain %q", want)
		}
	}
	// O rótulo do campo só existe dentro dos tipos, não escrito no template.
	if all, inTypes := strings.Count(detail, "Número da issue"), strings.Count(detail, `"item_label":"Número da issue"`); all != inTypes {
		t.Errorf("the task page has the issue label %d time(s) outside the integration types", all-inTypes)
	}
	// A página da tarefa só mostra: o formulário de edição fica num modal, aberto pelo lápis.
	for _, want := range []string{
		`x-show="$store.modal.name === 'task-edit'"`, `aria-label="Editar tarefa"`, `@click="openEdit()"`, `id="task-name"`, "Voltar à lista",
		// A descrição é Markdown: a página a renderiza e o modal tem as duas etapas e a prévia.
		`x-html="WTT.markdown(task && task.description)"`, `class="wizard-steps"`, `submitStep('save')`, `id="task-preview"`, "Pré-visualizar",
	} {
		if !strings.Contains(detail, want) {
			t.Errorf("the task page does not contain %q", want)
		}
	}
	// Os detalhes (status, prioridade, responsável, prazo, etiquetas, criação) ficam num cartão à
	// parte do nome e da descrição, na outra coluna.
	for _, want := range []string{
		`class="task-layout"`, `class="card task-main"`, `class="card task-details"`, `class="detail-list"`, `class="task-description"`,
		"<h2 id=\"task-details-title\">Detalhes</h2>", "<dt>Status</dt>", "<dt>Prioridade</dt>", "<dt>Responsável</dt>", "<dt>Prazo</dt>", "<dt>Etiquetas</dt>", "<dt>Criada em</dt>",
		// Prioridade e status com cor própria, no selo e nos selects do modal Editar tarefa.
		`:class="statusClass(task && task.status)"`, `:class="priorityClass(task && task.priority)" x-text="task && WTT.fmt.priority(task.priority)"`,
		`class="select-tone" :class="statusClass(draft.status)"`, `class="select-tone" :class="priorityClass(draft.priority)"`,
		// Pegar a tarefa sem bater o ponto, e a atualização rápida: um modal só com status, prioridade e etiquetas.
		`@click="claim()"`, `x-show="task && !task.assignee_id"`, "Pegar tarefa", `@click="openQuick()"`, "Atualização rápida",
		`x-show="$store.modal.name === 'task-quick'"`, `@submit.prevent="saveQuick()"`, `x-show="errors.claim"`,
		`id="quick-task-status"`, `id="quick-task-priority"`, `for="quick-task-priority"`, `id="quick-task-labels-label"`,
	} {
		if !strings.Contains(detail, want) {
			t.Errorf("the task page does not contain %q", want)
		}
	}
	if strings.Contains(detail, `x-show="task && task.priority !== 'none'"`) {
		t.Error("the task page still hides the priority badge when there is none")
	}
	// Os campos de atributos estão no modal Editar e no de atualização rápida, na mesma página: cada id
	// aparece uma vez só (o do segundo leva o prefixo quick-), senão o `for` dos rótulos aponta para o errado.
	for _, id := range []string{"task-status", "task-priority", "task-labels-label", "quick-task-status", "quick-task-priority", "quick-task-labels-label"} {
		if n := strings.Count(detail, `id="`+id+`"`); n != 1 {
			t.Errorf("the task page has id=%q %d times, want once", id, n)
		}
	}
	// A tarefa é uma tela própria: tem cabeçalho com o caminho, o título e as ações, e não mostra a
	// barra de abas do projeto nem o nome do projeto como título da página.
	for _, want := range []string{`class="task-head"`, `aria-label="Você está em"`, `<h1 class="task-title"`, ">Tela de login</h1>", "Voltar à lista"} {
		if !strings.Contains(detail, want) {
			t.Errorf("the task page does not contain %q", want)
		}
	}
	for _, not := range []string{`class="tabs"`, `data-project-name`, `/tasks" aria-current="page"`} {
		if strings.Contains(detail, not) {
			t.Errorf("the task page still has the project tabs or heading: %q", not)
		}
	}
	main, details, description := strings.Index(detail, `class="card task-main"`), strings.Index(detail, `class="card task-details"`), strings.Index(detail, `class="task-description"`)
	if !(main < description && description < details) {
		t.Error("the details card is not after the card with the name and the description")
	}
	if strings.Contains(detail[main:details], `class="detail-list"`) || strings.Contains(detail[main:details], `<dl class="facts">`) {
		t.Error("the attributes are still inside the card with the name and the description")
	}
	// O marked e o DOMPurify só vêm nas páginas que mostram ou escrevem Markdown.
	for _, lib := range []string{`/static/marked.min.js`, `/static/purify.min.js`} {
		if !strings.Contains(detail, lib) {
			t.Errorf("the task page does not load %s", lib)
		}
		if home := do(e, "GET", "/projects/"+projectID+"/overview", "", member.session).Body.String(); strings.Contains(home, lib) {
			t.Errorf("the Início page loads %s without using it", lib)
		}
	}
	if at, modal := strings.Index(detail, `id="task-name"`), strings.Index(detail, `x-show="$store.modal.name === 'task-edit'"`); at < modal {
		t.Error("the task name field is on the page, outside the edit modal")
	}
	// A linha da lista inteira leva à página da tarefa, sem botão Detalhes, sem Iniciar e sem a
	// coluna do item externo; a dica acima da tabela diz que a linha abre a tarefa.
	board := do(e, "GET", "/projects/"+projectID+"/tasks", "", member.session).Body.String()
	if !strings.Contains(board, `class="row-link" @click="if (!$event.target.closest('a, button')) location.href = '/tasks/' + t.id"`) {
		t.Error("the board row does not open the task page when clicked, or opens it when the Pegar button is clicked")
	}
	// Pegar a tarefa, só na lista das sem responsável: associa a tarefa à pessoa sem bater o ponto.
	for _, want := range []string{`<td class="actions" x-show="key === &#39;free&#39;">`, `@click="claim(t)"`, `<th class="actions" x-show="key === &#39;free&#39;">`, `x-show="errors.claim"`, " Pegar</button>"} {
		if !strings.Contains(board, want) {
			t.Errorf("the board does not contain %q", want)
		}
	}
	for _, not := range []string{`>Detalhes <`, `@click="start(t)"`, "<th>Item externo</th>", "t.external_item_id", "externalLabel", "integration_types"} {
		if strings.Contains(board, not) {
			t.Errorf("the board still has %q", not)
		}
	}
	if !strings.Contains(board, "Clique em uma tarefa para abrir os detalhes.") {
		t.Error("the board does not say that clicking a task opens it")
	}
	// Os chips dos filtros, os selos das linhas e os selects do modal Nova tarefa levam a cor de
	// cada prioridade e de cada status; "sem prioridade" tem selo, e não um traço.
	for _, want := range []string{
		`class="chip" :class="priorityClass(p.value)"`, `class="chip" :class="statusClass(s.value)"`,
		`class="badge" :class="priorityClass(t.priority)" x-text="WTT.fmt.priority(t.priority)"`, `class="badge" :class="statusClass(t.status)"`,
		`class="select-tone" :class="priorityClass(draft.priority)"`,
		// A descrição da seção Com responsável diz que são tarefas de quem não é a pessoa.
		`x-text="key === 'free' ? $t('tasks.section_free_hint') : $t('tasks.section_taken_hint')"`,
	} {
		if !strings.Contains(board, want) {
			t.Errorf("the board does not contain %q", want)
		}
	}
	if strings.Contains(board, `x-show="t.priority !== 'none'"`) {
		t.Error("the board still hides the priority badge when there is none")
	}
	// As etiquetas só servem de filtro na lista: as linhas não as desenham sob o nome.
	if strings.Contains(board, `class="label-list"`) || strings.Contains(board, `x-for="l in t.labels"`) {
		t.Error("the board rows still draw the labels of each task")
	}
	if home := do(e, "GET", "/projects/"+projectID+"/overview", "", member.session).Body.String(); strings.Contains(home, "integration_types") {
		t.Error("the Início tab gets the integration types without using them")
	}
}
