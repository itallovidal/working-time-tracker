package web

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Os cartões das tarefas da pessoa, no Início do projeto e no painel do timer, seguem um desenho: o nome
// com a prioridade; o status e o prazo; e, depois de uma divisória, o sinal das etiquetas e das integrações
// com o Iniciar e a seta que abre o cartão. O cartão inteiro abre a tarefa; é só borda e, com o mouse
// em cima, ganha o fundo; não tem sublinhado nem linha lateral. O navegador não avisa quando um nome some
// do JavaScript nem quando um estilo se perde, então estes testes leem os arquivos.
func TestTaskCardsFollowTheirLayout(t *testing.T) {
	read := func(name string) string {
		t.Helper()
		raw, err := FS.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	css, partial, js := read("static/app.css"), read("templates/partials/task_card.gohtml"), read("static/pages/project.js")

	// O cartão é o parcial `task_card`, usado no Início e dentro do painel do timer.
	start := strings.Index(partial, `{{define "task_card"}}`)
	if start < 0 {
		t.Fatal("there is no task_card partial")
	}
	cards := partial[start:]

	// A ordem de cima para baixo: nome, prioridade, status, prazo e, no pé, o sinal das integrações, as
	// etiquetas por extenso, o Iniciar e a seta.
	order := []string{
		`class="task-card-head"`,
		`class="task-card-name"`,
		`:class="priorityClass(t.priority)"`,
		`:class="statusClass(t.status)"`,
		`cardDeadlineLabel(t)`,
		`class="task-card-foot"`,
		`x-for="l in t.links"`,
		`x-for="l in t.labels"`,
		`startTask(t)`,
		`open = !open`,
	}
	last := -1
	for _, want := range order {
		at := strings.Index(cards, want)
		if at < 0 {
			t.Errorf("the task card does not contain %q", want)
			continue
		}
		if at < last {
			t.Errorf("the task card has %q before something that should come before it: the order is %q", want, order)
		}
		last = at
	}
	for _, want := range []string{
		`WTT.fmt.taskStatus(t.status)`, `WTT.fmt.priority(t.priority)`, // os selos com o nome do status e da prioridade
		`hasDeadline(t)`,                                // o prazo, só quando há
		`isRunning(t)`,                                  // o tempo e o Parar tarefa no lugar do Iniciar
		`'is-running': isRunning(t)`,                    // o cartão em andamento ganha o destaque
		`x-data="{ open: false }"`,                      // abrir e fechar é só da tela
		`:aria-expanded="open.toString()"`,              // a seta diz se o cartão está aberto
		`:aria-controls="'card-more-' + t.id"`,          // e o que ela abre
		`$store.clock.taskElapsed(`,                     // o tempo da tarefa na sessão
		`linkIcon(l)`, `linkLabel(l)`, `hasExternal(t)`, // cada item ligado: o ícone da plataforma, e o nome por extenso quando aberto
		`card-link`, `location.href = '/tasks/' + t.id`, `$event.target.closest('a, button')`, // o cartão inteiro abre a tarefa, e o botão e o link não
	} {
		if !strings.Contains(cards, want) {
			t.Errorf("the task card does not contain %q", want)
		}
	}
	// O que saiu: o botão Ver detalhes (o cartão é o link), o texto só para leitor de tela (o nome da
	// plataforma vai no aria-label do item) e o selo único "GitHub #16 + Trello X".
	for _, gone := range []string{"view_details", "sr-only", "externalLabel(t)", "task-card-badges", "task-card-bottom"} {
		if strings.Contains(cards, gone) {
			t.Errorf("the task card still has %q", gone)
		}
	}

	// Os estilos: sem linha lateral nem cor literal, o nome sem sublinhado, e o cartão clicável com a
	// mão, só borda e, sob o mouse, com o fundo.
	rule := func(selector string) string {
		t.Helper()
		m := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(selector) + ` \{([^}]*)\}`).FindStringSubmatch(css)
		if m == nil {
			t.Fatalf("app.css has no %s rule", selector)
		}
		return m[1]
	}
	if strings.Contains(rule(".task-card"), "border-left") {
		t.Error("the task card has a left line again: the priority is a badge now")
	}
	if regexp.MustCompile(`(?m)^\.(task-card|card-link)[^{]*\{[^}]*#[0-9a-fA-F]{3,6}`).MatchString(css) {
		t.Error("a .task-card or .card-link rule uses a literal colour: it has to come from the tokens, to work in both themes")
	}
	if !strings.Contains(rule(".task-card-name"), "text-decoration: none;") {
		t.Error("the task name is underlined: the card is the link, and it has no underline")
	}
	if !strings.Contains(rule(".card-link"), "cursor: pointer;") {
		t.Error("the clickable card does not have the pointer cursor")
	}
	// O cartão é só borda: o fundo é transparente, e é o mouse em cima que o preenche, com um token que
	// existe nos dois temas (e não mais com um fundo mais escuro que o cartão).
	if !strings.Contains(rule(".task-card"), "background: transparent;") {
		t.Error("the task card has a background again: it is only a border, and the hover fills it")
	}
	hover := rule(".card-link:hover")
	if !strings.Contains(hover, "background: var(--surface);") {
		t.Error("the clickable card does not get its background on hover")
	}
	if strings.Contains(css, "--surface-hover") {
		t.Error("the --surface-hover token is back: the hover fills with --surface (or --surface-2 inside the clock panel)")
	}
	if !strings.Contains(css, ".clock .card-link:hover { background: var(--surface-2); }") {
		t.Error("the card inside the clock panel has no hover fill: the panel is already --surface, so it needs a tone above")
	}

	// Com subgrid, o cartão ocupa uma linha da grade por filho: o `span` do CSS é o número de filhos.
	// As partes são os filhos diretos do cartão, que são os três `task-card-*` abaixo; o que está dentro
	// deles (o nome, os sinais, a ação) não entra na conta.
	var children int
	for _, part := range []string{"task-card-head", "task-card-meta", "task-card-foot"} {
		if !strings.Contains(cards, `class="`+part+`"`) {
			t.Errorf("the task card has no %s part", part)
			continue
		}
		children++
	}
	if !strings.Contains(css, "grid-row: span "+strconv.Itoa(children)+";") {
		t.Errorf("the card has %d children and app.css has no `grid-row: span %d` for the subgrid: the rows of neighbouring cards would not line up", children, children)
	}

	// Todo nome que o cartão chama existe no JavaScript do projeto, e o Início recebe os ajudantes.
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
	for _, page := range []string{"projectMyOverview", "projectMyTasks"} {
		component := js[strings.Index(js, "Alpine.data('"+page+"'"):]
		component = component[:strings.Index(component, "\n  }));")]
		if !strings.Contains(component, "...taskClock()") {
			t.Errorf("%s does not spread taskClock(), which has what the task card calls", page)
		}
	}
}
