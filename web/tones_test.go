package web

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Prioridade e status têm cores próprias (`.tone` e uma variante `.prio-*` ou `.status-*`
// no app.css). Estes testes cuidam do que o navegador não avisa: um valor novo em app.js sem
// cor, uma cor repetida entre os dois grupos e uma cor que só existe num dos temas.

// valuesIn devolve os valores da lista de app.js que começa em `const <name> = [`.
func valuesIn(t *testing.T, js, name string) []string {
	t.Helper()
	m := regexp.MustCompile(`const ` + name + ` = \[([^\]]*)\]`).FindStringSubmatch(js)
	if m == nil {
		t.Fatalf("app.js has no list %s", name)
	}
	var values []string
	for _, v := range regexp.MustCompile(`'([a-z_]+)'`).FindAllStringSubmatch(m[1], -1) {
		values = append(values, v[1])
	}
	if len(values) == 0 {
		t.Fatalf("app.js list %s is empty", name)
	}
	return values
}

func TestPriorityAndStatusHaveTheirOwnColors(t *testing.T) {
	cssRaw, err := FS.ReadFile("static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	jsRaw, err := FS.ReadFile("static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	css, js := string(cssRaw), string(jsRaw)

	// Cada prioridade e cada status de app.js tem a sua classe, e cada uma pede uma cor diferente.
	tones := map[string]string{} // classe -> cor (--tone)
	variant := regexp.MustCompile(`(?m)^\.(prio|status)-([a-z_]+) \{ --tone: var\(--([a-z0-9-]+)\); --tone-soft: var\(--([a-z0-9-]+)\);`)
	for _, m := range variant.FindAllStringSubmatch(css, -1) {
		tones[m[1]+"-"+m[2]] = m[3]
	}
	var palette []string
	for group, name := range map[string]string{"prio": "priorities", "status": "taskStatuses"} {
		for _, v := range valuesIn(t, js, name) {
			color, ok := tones[group+"-"+v]
			if !ok {
				t.Errorf("app.js has %s %q, and app.css has no .%s-%s with a color", name, v, group, v)
				continue
			}
			palette = append(palette, color)
		}
	}
	seen := map[string]bool{}
	for _, c := range palette {
		if seen[c] {
			t.Errorf("the color --%s is used by two priorities or statuses: each one needs its own", c)
		}
		seen[c] = true
	}

	// As cores existem no tema claro e no escuro, cada uma com o fundo suave.
	dark := strings.Index(css, "@media (prefers-color-scheme: dark)")
	if dark < 0 {
		t.Fatal("app.css has no dark theme")
	}
	light, darkBlock := css[:dark], css[dark:]
	for c := range seen {
		for _, token := range []string{"--" + c + ":", "--" + c + "-soft:"} {
			if c == "ink-2" && strings.HasSuffix(token, "-soft:") {
				continue // "sem prioridade" usa o fundo da superfície
			}
			if !strings.Contains(light, token) || !strings.Contains(darkBlock, token) {
				t.Errorf("%s is not defined in both the light and the dark theme", token)
			}
		}
	}
}

// luminance e contrast seguem a fórmula do WCAG para cor sRGB (#rrggbb).
func luminance(t *testing.T, hex string) float64 {
	t.Helper()
	var channels [3]float64
	for i := range channels {
		v, err := strconv.ParseUint(hex[1+2*i:3+2*i], 16, 8)
		if err != nil {
			t.Fatalf("color %q is not #rrggbb", hex)
		}
		c := float64(v) / 255
		if c <= 0.03928 {
			channels[i] = c / 12.92
		} else {
			channels[i] = math.Pow((c+0.055)/1.055, 2.4)
		}
	}
	return 0.2126*channels[0] + 0.7152*channels[1] + 0.0722*channels[2]
}

func contrast(t *testing.T, a, b string) float64 {
	t.Helper()
	la, lb := luminance(t, a), luminance(t, b)
	return (math.Max(la, lb) + 0.05) / (math.Min(la, lb) + 0.05)
}

// A página inicial dá cor aos cartões de projeto e aos números da visão geral com as classes
// `.hue-*`, que não têm o sentido de prioridade nem de status. O teste cuida do que o navegador
// não avisa: um tom sorteado por app.js que não existe no app.css, um tom usado no modelo sem
// classe, uma cor que falta num dos temas e texto que não se lê em cima da cor (o tom sobre o
// fundo suave, que é como a sigla do projeto e os ícones da visão geral aparecem).
func TestHomeHuesAreColoredAndReadable(t *testing.T) {
	read := func(name string) string {
		t.Helper()
		raw, err := FS.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	css, orgJS, home := read("static/app.css"), read("static/pages/org.js"), read("templates/pages/org_home.gohtml")

	hues := map[string]bool{}
	for _, h := range valuesIn(t, orgJS, "projectHues") {
		hues[h] = true
	}
	for _, m := range regexp.MustCompile(`hue-([a-z]+)`).FindAllStringSubmatch(home, -1) {
		hues[m[1]] = true
	}
	for h := range hues {
		rule := regexp.MustCompile(`(?m)^\.hue-` + h + ` \{ --tone: var\(--` + h + `\); --tone-soft: var\(--` + h + `-soft\); \}`)
		if !rule.MatchString(css) {
			t.Errorf("the hue %q is used by the home page, and app.css has no .hue-%s with --%s and --%s-soft", h, h, h, h)
		}
	}

	dark := strings.Index(css, "@media (prefers-color-scheme: dark)")
	if dark < 0 {
		t.Fatal("app.css has no dark theme")
	}
	tokens := regexp.MustCompile(`--([a-z0-9-]+): (#[0-9a-fA-F]{6});`)
	for theme, block := range map[string]string{"light": css[:dark], "dark": css[dark:]} {
		colors := map[string]string{}
		for _, m := range tokens.FindAllStringSubmatch(block, -1) {
			colors[m[1]] = m[2]
		}
		for h := range hues {
			tone, soft := colors[h], colors[h+"-soft"]
			if tone == "" || soft == "" {
				t.Errorf("%s theme: --%s or --%s-soft is not defined", theme, h, h)
				continue
			}
			if c := contrast(t, tone, soft); c < 4.5 {
				t.Errorf("%s theme: the %s tone %s on its soft background %s has a contrast of %.2f:1, want at least 4.5:1", theme, h, tone, soft, c)
			}
		}
	}
}

// Os cartões de projeto levam cor só num toque: a sigla em tom suave, os ícones e a borda ao
// passar o mouse. Faixa no topo, sigla de fundo cheio e selos coloridos deixaram a lista muito
// colorida e foram tirados; o teste segura isso, para um ajuste de estilo não trazê-los de volta.
func TestProjectCardsStayCalm(t *testing.T) {
	raw, err := FS.ReadFile("static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(raw)
	rule := func(selector string) (string, bool) {
		m := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(selector) + ` \{([^}]*)\}`).FindStringSubmatch(css)
		if m == nil {
			return "", false
		}
		return m[1], true
	}

	if _, found := rule(".project-card::before"); found {
		t.Error("the project card has a colored stripe (.project-card::before): the cards should be neutral")
	}
	card, found := rule(".project-card")
	if !found {
		t.Fatal("app.css has no .project-card")
	}
	if !strings.Contains(card, "background: var(--surface);") {
		t.Errorf("the project card background is not the neutral surface: %s", card)
	}
	mark, found := rule(".project-mark")
	if !found {
		t.Fatal("app.css has no .project-mark")
	}
	if !strings.Contains(mark, "background: var(--tone-soft);") || strings.Contains(mark, "background: var(--tone);") {
		t.Errorf("the project mark should be a soft tint with the tone as text, not a solid fill: %s", mark)
	}
	stat, found := rule(".project-stat")
	if !found {
		t.Fatal("app.css has no .project-stat")
	}
	if strings.Contains(stat, "var(--tone") {
		t.Errorf("the people and task counts should be neutral chips, not tinted: %s", stat)
	}
}
