package page

import (
	"slices"
	"testing"

	"working-time-tracker/internal/domain/permission"
)

// O ?next= do login só pode apontar para um caminho deste site. O navegador
// remove TAB e quebra de linha antes de ler a URL, então "/\t/host" viraria
// "//host", um endereço de outro site.
func TestSafeNext(t *testing.T) {
	cases := map[string]string{
		"/projects/x/tasks?only=mine": "/projects/x/tasks?only=mine",
		"/%09/x":                      "/%09/x",
		"":                            "/",
		"//evil.example":              "/",
		"/\\evil.example":             "/",
		"/\t/evil.example":            "/",
		"/\n/evil.example":            "/",
		"/\r/evil.example":            "/",
		"/x\\/evil.example":           "/",
		"https://evil.example":        "/",
		"javascript:alert(1)":         "/",
	}
	for next, want := range cases {
		if got := safeNext(next); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", next, got, want)
		}
	}
}

// A barra de abas da Gestão só mostra a aba que a pessoa abre: as permissões vêm de ManagementTabs, as mesmas que a
// rota de cada aba pede. O grupo financeiro não cuida das integrações, e quem só cuida das etiquetas abre só a Visão
// geral.
func TestOpensTab(t *testing.T) {
	preset := func(id string) permission.Set {
		p, _ := permission.PresetByID(id)
		return permission.Set{Keys: p.Permissions}
	}
	cases := []struct {
		who  string
		set  permission.Set
		want []string
	}{
		{"owner or admin", permission.Set{All: true}, []string{"overview", "teams", "integrations", "settings"}},
		{"project admin", preset(permission.PresetAdmin), []string{"overview", "teams", "integrations", "settings"}},
		{"manager", preset(permission.PresetManager), []string{"overview", "teams", "integrations", "settings"}},
		{"finance", preset(permission.PresetFinance), []string{"overview", "teams", "settings"}},
		{"labels only", permission.Set{Keys: []string{permission.LabelsManage}}, []string{"overview"}},
		{"member", preset(permission.PresetMember), nil},
	}
	for _, c := range cases {
		var got []string
		for _, tab := range ManagementTabs {
			if (Data{Access: c.set}).OpensTab(tab.ID) {
				got = append(got, tab.ID)
			}
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s opens %v, want %v", c.who, got, c.want)
		}
	}
	if (Data{Access: permission.Set{All: true}}).OpensTab("nope") {
		t.Error("a tab that does not exist should not open")
	}
}
