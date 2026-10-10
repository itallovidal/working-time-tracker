package web

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// Os formulários seguem uma regra só, na tela e no servidor. Estes testes cuidam do que o navegador não avisa:
// um tamanho escrito à mão no template (que passaria a divergir do servidor), um formulário que volta a usar o
// balão nativo do navegador, no idioma dele, e um campo opcional que diz "Opcional" no placeholder em vez do rótulo.

func templates(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := fs.WalkDir(FS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".gohtml") {
			return err
		}
		b, err := FS.ReadFile(path)
		if err != nil {
			return err
		}
		out[path] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("no templates found")
	}
	return out
}

func TestTemplates_MaxlengthComesFromTheLimits(t *testing.T) {
	literal := regexp.MustCompile(`\bmaxlength="\d+"`)
	for path, src := range templates(t) {
		for _, m := range literal.FindAllString(src, -1) {
			t.Errorf("%s has %s: use {{limit \"...\"}} (internal/validate/limits.go) so the screen and the server share the number", path, m)
		}
	}
}

func TestTemplates_EveryFormIsCheckedByTheApp(t *testing.T) {
	form := regexp.MustCompile(`<form\b[^>]*>`)
	for path, src := range templates(t) {
		for _, tag := range form.FindAllString(src, -1) {
			if !strings.Contains(tag, "novalidate") {
				t.Errorf("%s has %s without novalidate: the app checks the fields (messages in the page language), not the browser", path, tag)
			}
		}
	}
}

func TestTemplates_OptionalIsSaidInTheLabel(t *testing.T) {
	for path, src := range templates(t) {
		if strings.Contains(src, `placeholder="{{.T "common.optional"}}"`) {
			t.Errorf("%s uses the Optional placeholder: say (opcional) in the label with the field_label partial", path)
		}
	}
}
