package web

import (
	"regexp"
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
