package web

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Os cartões das tarefas da pessoa, no Início do projeto, seguem um desenho: o nome; o status e a
// prioridade como selos; o prazo e o Iniciar; uma divisória; e as etiquetas. O cartão inteiro abre
// a tarefa, com o mouse virando mão e o fundo escurecendo, e não tem sublinhado nem linha lateral.
// O navegador não avisa quando um nome some do JavaScript nem quando um estilo se perde, então
// estes testes leem os arquivos.
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

	// A ordem de cima para baixo: nome, status, prioridade, prazo, Iniciar, divisória, etiquetas.
	order := []string{
		`class="task-card-name"`,
		`:class="statusClass(t.status)"`,
		`:class="priorityClass(t.priority)"`,
		`cardDeadlineLabel(t)`,
		`startTask(t)`,
		`class="task-card-foot"`,
		`x-for="l in t.labels"`,
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
		`hasDeadline(t)`,                                                                      // o prazo, só quando há
		`isRunning(t)`,                                                                        // o "Em andamento" no lugar do Iniciar
		`card-link`, `location.href = '/tasks/' + t.id`, `$event.target.closest('a, button')`, // o cartão inteiro abre a tarefa, e o botão e o link não
	} {
		if !strings.Contains(cards, want) {
			t.Errorf("the task card does not contain %q", want)
		}
	}
	// O que saiu: o botão Ver detalhes (o cartão é o link), o texto só para leitor de tela e a dica da
	// linha lateral (a prioridade é um selo, com texto).
	for _, gone := range []string{"view_details", "sr-only", ":title="} {
		if strings.Contains(cards, gone) {
			t.Errorf("the task card still has %q", gone)
		}
	}

	// Os estilos: sem linha lateral nem cor literal, o nome sem sublinhado, e o cartão clicável com a
	// mão e um fundo mais escuro (um token que existe nos dois temas).
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
	if !strings.Contains(rule(".card-link:hover"), "background: var(--surface-hover);") {
		t.Error("the clickable card does not change its background on hover")
	}
	dark := strings.Index(css, "@media (prefers-color-scheme: dark)")
	if dark < 0 {
		t.Fatal("app.css has no dark theme")
	}
	hover := regexp.MustCompile(`--surface-hover: (#[0-9a-fA-F]{6});`)
	light, night := hover.FindStringSubmatch(css[:dark]), hover.FindStringSubmatch(css[dark:])
	if light == nil || night == nil {
		t.Fatal("--surface-hover is not defined in both the light and the dark theme")
	}
	// "Mais escuro" é para valer nos dois temas: o fundo sob o mouse é menos luminoso que o do cartão.
	surface := regexp.MustCompile(`--surface: (#[0-9a-fA-F]{6});`)
	for theme, c := range map[string][2]string{
		"light": {light[1], surface.FindStringSubmatch(css[:dark])[1]},
		"dark":  {night[1], surface.FindStringSubmatch(css[dark:])[1]},
	} {
		if luminance(t, c[0]) >= luminance(t, c[1]) {
			t.Errorf("%s theme: the hover background %s is not darker than the card %s", theme, c[0], c[1])
		}
	}

	// Com subgrid, o cartão ocupa uma linha da grade por filho: o `span` do CSS é o número de filhos.
	children := len(regexp.MustCompile(`class="task-card-[a-z]+"`).FindAllString(cards, -1))
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
