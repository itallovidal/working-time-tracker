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

// A descrição da tarefa não tem maxlength: ele cortaria em silêncio o que fosse colado e a regra nunca avisaria.
// No lugar, o contador mostra o tamanho e a regra (contada sem aparar, como o servidor) confere no passo a passo e no
// modal Editar, com o erro embaixo do campo.
func TestTaskDescription_IsCheckedAndNotCut(t *testing.T) {
	tpl := templates(t)["templates/partials/task_fields.gohtml"]
	textarea := regexp.MustCompile(`<textarea id="task-description"[^>]*>`).FindString(tpl)
	if textarea == "" {
		t.Fatal("the task description textarea is not in templates/partials/task_fields.gohtml")
	}
	if strings.Contains(textarea, "maxlength") {
		t.Errorf("%s has maxlength: it cuts pasted text without a word; the counter and taskDescriptionRule do the checking", textarea)
	}
	if !strings.Contains(tpl, `descriptionLength() + '/{{limit "task_description"}}'`) {
		t.Error("task_fields.gohtml has no counter of the description built from the task_description limit")
	}

	raw, err := FS.ReadFile("static/pages/project.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(raw)
	for _, want := range []string{
		`WTT.rules.maxChars('task_description')`, // a regra, sem aparar
		`description: [self.draft.description, taskDescriptionRule]`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("project.js lacks %q", want)
		}
	}
	// O passo 1 do passo a passo e o modal Editar conferem nome e descrição pelo mesmo helper.
	if n := strings.Count(js, "checkTaskText(this)"); n != 2 {
		t.Errorf("checkTaskText(this) is called %d times in project.js, want 2 (taskWizard.next and taskDetail.save)", n)
	}
	app, err := FS.ReadFile("static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(app), "maxChars: (n) =>") {
		t.Error("app.js has no rules.maxChars")
	}
}
