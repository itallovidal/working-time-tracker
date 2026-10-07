package web

import (
	"regexp"
	"strings"
	"testing"
)

// Os cartões das tarefas da pessoa, no Início do projeto, têm a linha da esquerda na cor da
// prioridade, o selo de status, o prazo e as etiquetas. O navegador não avisa quando um nome some
// do JavaScript nem quando a linha perde a cor, então estes testes leem os arquivos.
func TestTaskCardsShowPriorityStatusDeadlineAndLabels(t *testing.T) {
	read := func(name string) string {
		t.Helper()
		raw, err := FS.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	css, page, js := read("static/app.css"), read("templates/pages/project_my_overview.gohtml"), read("static/pages/project.js")

	// A linha é a cor da prioridade (`--tone` vem de `.tone` e `.prio-*`; sem prioridade, `--tone-line`
	// é o cinza), e o cartão não pinta nada por conta própria.
	m := regexp.MustCompile(`(?m)^\.task-card \{([^}]*)\}`).FindStringSubmatch(css)
	if m == nil {
		t.Fatal("app.css has no .task-card")
	}
	if !strings.Contains(m[1], "border-left: 4px solid var(--tone-line, var(--tone));") {
		t.Errorf("the task card's left line is not the priority colour (--tone-line, then --tone): %s", m[1])
	}
	if regexp.MustCompile(`(?m)^\.task-card[^{]*\{[^}]*#[0-9a-fA-F]{3,6}`).MatchString(css) {
		t.Error("a .task-card rule uses a literal colour: it has to come from the tokens, to work in both themes")
	}

	// O que o cartão mostra.
	start := strings.Index(page, `<div class="task-cards"`)
	if start < 0 {
		t.Fatal("the project home has no task cards")
	}
	cards := page[start:]
	cards = cards[:strings.Index(cards, "</section>")]
	for _, want := range []string{
		`:class="priorityClass(t.priority)"`,     // a linha
		`class="sr-only"`,                        // a prioridade também em texto
		`:class="statusClass(t.status)"`,         // o selo de status
		`WTT.fmt.taskStatus(t.status)`,           // com o nome do status
		`hasDeadline(t)`, `cardDeadlineLabel(t)`, // o prazo, só quando há
		`class="label-list"`, `x-for="l in t.labels"`, `l.name`, // as etiquetas
		`isRunning(t)`, `startTask(t)`, // o que já havia
	} {
		if !strings.Contains(cards, want) {
			t.Errorf("the task card does not contain %q", want)
		}
	}

	// Todo nome que o cartão chama existe no JavaScript do projeto.
	alpine := regexp.MustCompile(`(?s)\{\{.*?\}\}`).ReplaceAllString(cards, "")
	names := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?:^|[^.\w$])([a-z][A-Za-z]*)\(`).FindAllStringSubmatch(alpine, -1) {
		names[m[1]] = true
	}
	for _, want := range []string{"priorityClass", "statusClass", "hasDeadline", "cardDeadlineClass", "cardDeadlineLabel", "isRunning", "startTask"} {
		if !names[want] {
			t.Errorf("the task card no longer calls %s(): update this list with what it uses", want)
		}
	}
	for name := range names {
		if !regexp.MustCompile(`(?m)^\s+` + name + `(\(|,|:)`).MatchString(js) {
			t.Errorf("the task card calls %s, which project.js does not define", name)
		}
	}
	// E os ajudantes do prazo e das cores chegam ao componente do Início.
	home := js[strings.Index(js, "Alpine.data('projectMyOverview'"):]
	home = home[:strings.Index(home, "\n  }));")]
	if !strings.Contains(home, "...taskBadges") {
		t.Error("projectMyOverview does not spread taskBadges, which has priorityClass and statusClass")
	}
}
