package page

import (
	"strings"
	"testing"

	"working-time-tracker/internal/domain/permission"
)

// tabsOf resume a barra do projeto em "id" por aba, com "*" na aberta: "home* mine tasks management".
func tabsOf(d Data) string {
	var out []string
	for _, t := range d.ProjectTabs() {
		id := t.ID
		if t.Current {
			id += "*"
		}
		out = append(out, id)
	}
	return strings.Join(out, " ")
}

// A barra do projeto vem de uma lista só: na equipe a Visão geral fica dentro da Gestão, com Colaboradores; no modo
// solo ela é uma aba do projeto, e a Gestão fica com Integrações e Configurações. Cada aba aparece conforme a
// permissão da pessoa, e só uma fica marcada.
func TestProjectTabs(t *testing.T) {
	all := permission.Set{All: true}
	member := permission.Set{}
	onlyIntegrations := permission.Set{Keys: []string{permission.IntegrationsManage}}
	page := func(solo bool, access permission.Set, management bool, tab string) Data {
		return Data{Me: identity("admin", solo, solo), Project: &Crumb{ID: "p1", Name: "P"}, Access: access, Management: management, Tab: tab}
	}
	for _, c := range []struct {
		name string
		d    Data
		want string
	}{
		{"team home", page(false, all, false, "home"), "home* mine tasks teams management"},
		{"team member", page(false, member, false, "tasks"), "home mine tasks* teams"},
		{"team overview", page(false, all, true, "overview"), "overview* teams integrations settings back"},
		{"team integrations only", page(false, onlyIntegrations, true, "integrations"), "overview integrations* back"},
		{"solo home", page(true, all, false, "home"), "home* tasks overview management"},
		{"solo tasks", page(true, member, false, "tasks"), "home tasks*"},
		{"solo overview", page(true, all, true, "overview"), "home tasks overview* management"},
		{"solo settings", page(true, all, true, "settings"), "integrations settings* back"},
	} {
		if got := tabsOf(c.d); got != c.want {
			t.Errorf("%s: tabs = %q, want %q", c.name, got, c.want)
		}
	}

	// Os endereços: a Visão geral continua em /management/overview, e o Início em /overview. No solo a aba das tarefas
	// é Tarefas, que junta Minhas tarefas e a Lista de tarefas (Decisão 28), no endereço da lista.
	for _, tab := range page(true, all, false, "home").ProjectTabs() {
		want := map[string]string{
			"home": "/projects/p1/overview", "tasks": "/projects/p1/tasks",
			"overview": "/projects/p1/management/overview", "management": "/projects/p1/management",
		}[tab.ID]
		if tab.Href != want {
			t.Errorf("solo %s: href = %q, want %q", tab.ID, tab.Href, want)
		}
		if tab.ID == "tasks" && tab.Label != "project.head.tasks_solo" {
			t.Errorf("solo tasks: label = %q, want project.head.tasks_solo", tab.Label)
		}
	}
	for _, tab := range page(false, all, false, "home").ProjectTabs() {
		if tab.ID == "tasks" && tab.Label != "project.head.tasks" {
			t.Errorf("team tasks: label = %q, want project.head.tasks", tab.Label)
		}
	}
}

// A raiz da Gestão leva para a primeira aba de dentro dela: a Visão geral na equipe, Integrações no modo solo.
func TestFirstManagementTab(t *testing.T) {
	all := permission.Set{All: true}
	if got := (Data{Me: identity("admin", true, false), Access: all}).firstManagementTab(); got != "overview" {
		t.Errorf("team: first tab = %q, want overview", got)
	}
	if got := (Data{Me: identity("admin", true, true), Access: all}).firstManagementTab(); got != "integrations" {
		t.Errorf("solo: first tab = %q, want integrations", got)
	}
}
