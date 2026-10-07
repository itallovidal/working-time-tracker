package web

import (
	"regexp"
	"strings"
	"testing"
)

// A página inicial deixou de mostrar a equipe por horas (quem trabalhou mais, quem trabalhou
// menos), mas o bloco ficou guardado: o parcial partials/team_hours.gohtml e o `teamHours()` do
// org.js. Código guardado apodrece sem ninguém ver, então este teste confere que as duas metades
// continuam se entendendo: todo nome que o parcial chama ou lê existe no JavaScript, e o
// `teamHours()` não está espalhado no orgHome (que seria voltar a comparar sem ninguém decidir).
func TestStoredTeamHoursStillMatchesTheScript(t *testing.T) {
	read := func(name string) string {
		t.Helper()
		raw, err := FS.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	partial, js := read("templates/partials/team_hours.gohtml"), read("static/pages/org.js")

	// O que o Go resolve (`{{...}}`) não é do Alpine.
	alpine := regexp.MustCompile(`(?s)\{\{.*?\}\}`).ReplaceAllString(partial, "")
	names := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?:^|[^.\w$])([a-z][A-Za-z]*)\(`).FindAllStringSubmatch(alpine, -1) {
		names[m[1]] = true
	}
	for _, m := range regexp.MustCompile(`@click="(rankingAt)\b`).FindAllStringSubmatch(alpine, -1) {
		names[m[1]] = true
	}
	for _, want := range []string{"rankingView", "barWidth", "secondsOf", "initialsOf", "current"} {
		if !names[want] {
			t.Errorf("the stored partial no longer calls %s(): update this list with what it uses", want)
		}
	}
	for name := range names {
		if !regexp.MustCompile(`(?m)^\s+` + name + `(\(|,|:)`).MatchString(js) {
			t.Errorf("team_hours.gohtml uses %s, which org.js does not define", name)
		}
	}

	// O mixin está definido e não entra no orgHome.
	if !strings.Contains(js, "const teamHours = () => ({") {
		t.Error("org.js no longer defines the teamHours() mixin that the stored partial needs")
	}
	home := js[strings.Index(js, "Alpine.data('orgHome'"):]
	home = home[:strings.Index(home, "\n  }));")]
	if strings.Contains(home, "...teamHours()") {
		t.Error("orgHome spreads teamHours(): the home page would compare how long each person worked again")
	}

	// E a página inicial não inclui o parcial.
	if strings.Contains(read("templates/pages/org_home.gohtml"), `template "team_hours"`) {
		t.Error("org_home includes the stored hours ranking")
	}
}
