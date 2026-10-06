package i18n

import (
	"regexp"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
)

var pluralForms = map[string]bool{"zero": true, "one": true, "two": true, "few": true, "many": true, "other": true}

// flatten lista as mensagens de um catálogo como chave -> texto. Uma mensagem com
// plural vira uma entrada por forma: "chave#one", "chave#other".
func flatten(prefix string, node any, out map[string]string) {
	switch v := node.(type) {
	case string:
		out[prefix] = v
	case map[string]any:
		if _, ok := v["other"].(string); ok {
			for form, text := range v {
				if s, ok := text.(string); ok && pluralForms[form] {
					out[prefix+"#"+form] = s
				}
			}
			return
		}
		for k, child := range v {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			flatten(key, child, out)
		}
	}
}

func catalog(t *testing.T) *Catalog {
	t.Helper()
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return c
}

func flatOf(t *testing.T, c *Catalog, lang string) map[string]string {
	t.Helper()
	out := map[string]string{}
	flatten("", c.Tree(lang), out)
	return out
}

var placeholderRe = regexp.MustCompile(`\{\{\s*\.(\w+)\s*\}\}`)

func placeholders(s string) string {
	var names []string
	for _, m := range placeholderRe.FindAllStringSubmatch(s, -1) {
		names = append(names, m[1])
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

// Os idiomas têm de ter exatamente as mesmas chaves, as mesmas formas de plural
// e os mesmos placeholders: uma chave que falta mostraria o texto da chave na tela.
func TestCatalogs_HaveTheSameKeysAndPlaceholders(t *testing.T) {
	c := catalog(t)
	base := flatOf(t, c, Default)
	if len(base) == 0 {
		t.Fatal("empty catalog")
	}
	for _, lang := range Supported() {
		if lang == Default {
			continue
		}
		other := flatOf(t, c, lang)
		for key, text := range base {
			got, ok := other[key]
			if !ok {
				t.Errorf("%s is missing %q", lang, key)
				continue
			}
			if placeholders(text) != placeholders(got) {
				t.Errorf("%s: %q has placeholders [%s], %s has [%s]", lang, key, placeholders(got), Default, placeholders(text))
			}
			if strings.TrimSpace(got) == "" {
				t.Errorf("%s: %q is empty", lang, key)
			}
		}
		for key := range other {
			if _, ok := base[key]; !ok {
				t.Errorf("%s has %q, which %s does not", lang, key, Default)
			}
		}
	}
}

func TestT(t *testing.T) {
	c := catalog(t)
	cases := []struct {
		lang, key string
		args      []any
		want      string
	}{
		{PTBR, "auth.login.submit", nil, "Entrar"},
		{EN, "auth.login.submit", nil, "Sign in"},
		{EN, "auth.login.no_account", nil, "Don't have an account yet?"},
		{PTBR, "session.in_progress", []any{"name", "Tela de login"}, "Em andamento: Tela de login"},
		{EN, "session.in_progress", []any{"name", "Login screen"}, "In progress: Login screen"},
		{"fr", "auth.login.submit", nil, "Entrar"}, // idioma desconhecido cai no padrão
		{PTBR, "no.such.key", nil, "no.such.key"},  // chave que falta aparece como está
	}
	for _, tc := range cases {
		if got := c.T(tc.lang, tc.key, tc.args...); got != tc.want {
			t.Errorf("T(%q, %q) = %q, want %q", tc.lang, tc.key, got, tc.want)
		}
	}
}

func TestT_Plural(t *testing.T) {
	files := fstest.MapFS{
		"l/pt-BR.yaml": {Data: []byte("people:\n  one: \"{{.count}} pessoa\"\n  other: \"{{.count}} pessoas\"\nplain: Só texto\n")},
		"l/en.yaml":    {Data: []byte("people:\n  one: \"{{.count}} person\"\n  other: \"{{.count}} people\"\nplain: Plain\n")},
	}
	c, err := LoadFS(files, "l")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		lang  string
		count int
		want  string
	}{
		{EN, 1, "1 person"}, {EN, 0, "0 people"}, {EN, 5, "5 people"},
		{PTBR, 1, "1 pessoa"}, {PTBR, 3, "3 pessoas"},
		// No CLDR do português o zero também é "one": quem precisa de "nenhuma pessoa" usa outra chave.
		{PTBR, 0, "0 pessoa"},
	}
	for _, tc := range cases {
		if got := c.T(tc.lang, "people", "count", tc.count); got != tc.want {
			t.Errorf("T(%q, people, %d) = %q, want %q", tc.lang, tc.count, got, tc.want)
		}
	}
	// Uma mensagem sem plural aceita o par "count" sem quebrar.
	if got := c.T(EN, "plain", "count", 2); got != "Plain" {
		t.Errorf("plain with count = %q", got)
	}
}

func TestMatch(t *testing.T) {
	c := catalog(t)
	cases := []struct{ cookie, accept, want string }{
		{"", "", PTBR},
		{"en", "", EN},
		{"EN", "", EN},
		{"pt-BR", "en-US,en;q=0.9", PTBR}, // o cookie vence o cabeçalho
		{"xx", "en-US,en;q=0.9", EN},      // cookie inválido é ignorado
		{"", "en-GB", EN},
		{"", "pt-PT,pt;q=0.9", PTBR},
		{"", "es-ES,es;q=0.9", PTBR}, // nenhum idioma suportado
		{"", "isto não é um cabeçalho", PTBR},
		{"", "fr;q=0.9,en;q=0.8", EN},
	}
	for _, tc := range cases {
		if got := c.Match(tc.cookie, tc.accept); got != tc.want {
			t.Errorf("Match(%q, %q) = %q, want %q", tc.cookie, tc.accept, got, tc.want)
		}
	}
}

func TestScript(t *testing.T) {
	c := catalog(t)
	for _, lang := range Supported() {
		body, hash, ok := c.Script(lang)
		if !ok || len(hash) != 12 || !strings.HasPrefix(string(body), "window.I18N=") {
			t.Errorf("Script(%q) = %q, %q, %v", lang, body, hash, ok)
		}
		if c.Hash(lang) != hash {
			t.Errorf("Hash(%q) differs from Script's", lang)
		}
	}
	_, ptHash, _ := c.Script(PTBR)
	_, enHash, _ := c.Script(EN)
	if ptHash == enHash {
		t.Error("the two languages have the same hash")
	}
	if _, _, ok := c.Script("fr"); ok {
		t.Error("Script returned a catalog for an unsupported language")
	}
}

func TestKey(t *testing.T) {
	if Key("pt-BR") != "pt_br" || Key("en") != "en" {
		t.Errorf("Key: %q %q", Key("pt-BR"), Key("en"))
	}
}
