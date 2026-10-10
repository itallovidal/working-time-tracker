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

// Um tamanho escrito à mão na tela diverge do servidor sem ninguém ver: o `: 32` do :maxlength do documento e o
// rules.max(32) do JavaScript escaparam do teste do maxlength literal. Estes dois olham as mesmas coisas nos
// atributos ligados do Alpine e nas regras do JavaScript.
func TestLimits_NoNumberWrittenByHandOnTheScreen(t *testing.T) {
	action := regexp.MustCompile(`(?s)\{\{.*?\}\}`)
	bound := regexp.MustCompile(`:maxlength="[^"]*\b\d+\b[^"]*"`)
	for path, src := range templates(t) {
		for _, m := range bound.FindAllString(action.ReplaceAllString(src, ""), -1) {
			t.Errorf("%s has %s: take the number from {{limit \"...\"}} (internal/validate/limits.go)", path, m)
		}
	}

	literal := regexp.MustCompile(`\bmax(Chars)?\(\s*\d+\s*\)`)
	err := fs.WalkDir(FS, "static", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".js") || strings.Contains(path, "/vendor/") {
			return err
		}
		b, err := FS.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range literal.FindAllString(string(b), -1) {
			t.Errorf("%s has %s: pass the name of a limit (rules.max('name')), which comes from internal/validate/limits.go", path, m)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// O documento fiscal é conferido na tela, pelo país, antes do envio: o cliente (e a etapa dos clientes das
// boas-vindas) e as Configurações da organização. As regras são a porta do que o servidor faz (cnpj.go e digits.go).
func TestTaxId_IsCheckedOnTheScreen(t *testing.T) {
	read := func(name string) string {
		b, err := FS.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	app, org := read("static/app.js"), read("static/pages/org.js")
	for _, want := range []string{"taxId: (kind, code)", "const taxIdValid = {", "cnpj(x) {", "ein(x) {", "generic(x) {", "limits.tax_id"} {
		if !strings.Contains(app, want) {
			t.Errorf("app.js lacks %q", want)
		}
	}
	for _, want := range []string{
		`rules.taxId(WTT.countries.legalId(b.country).field, 'customer.invalid_document')`, // cliente e boas-vindas
		`rules.taxId(legal, 'organization.invalid_' + legal)`,                              // Configurações da organização
	} {
		if !strings.Contains(org, want) {
			t.Errorf("org.js lacks %q", want)
		}
	}
	if !strings.Contains(templates(t)["templates/partials/customer_document.gohtml"], `{{limit "tax_id"}}`) {
		t.Error("customer_document.gohtml does not take the generic document size from the tax_id limit")
	}
}

// A confirmação da senha do cadastro é um campo como os outros: o erro (vazia ou diferente) sai embaixo dele e o
// check leva o foco. Antes, a confirmação vazia só mexia o foco, sem dizer o que errou.
func TestPasswordConfirmation_IsAFieldWithItsError(t *testing.T) {
	signup := templates(t)["templates/pages/signup.gohtml"]
	for _, want := range []string{`data-field="confirm_password"`, `{{template "field_error" "confirm_password"}}`} {
		if !strings.Contains(signup, want) {
			t.Errorf("signup.gohtml lacks %s", want)
		}
	}
	read := func(name string) string {
		b, err := FS.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	auth, org, app := read("static/pages/auth.js"), read("static/pages/org.js"), read("static/app.js")
	if !strings.Contains(auth, "confirm_password: [this.confirmation, rules.confirmation(this.password)]") {
		t.Error("auth.js: the signup does not check the confirmation with rules.confirmation")
	}
	if strings.Contains(auth, "$refs.confirm.focus()") {
		t.Error("auth.js moves the focus to the confirmation by hand: the check does it, with the error under the field")
	}
	if !strings.Contains(org, "WTT.rules.confirmation(next)") {
		t.Error("org.js: the password change does not check the confirmation with rules.confirmation")
	}
	if !strings.Contains(app, "confirmation: (other) =>") {
		t.Error("app.js has no rules.confirmation")
	}
}

// Estes campos aceitam vazio e dizem "(opcional)" no rótulo, pelo partial field_label. Antes o rótulo era escrito
// à mão e não dizia nada (só uma opção "Não informado", ou uma dica ao lado).
func TestOptionalFields_SayItInTheLabel(t *testing.T) {
	tpl := templates(t)
	for _, c := range []struct{ file, id string }{
		{"templates/pages/org_settings.gohtml", "org-work-mode"},
		{"templates/partials/onboarding.gohtml", "ob-work-mode"},
		{"templates/partials/person_edit_modal.gohtml", "person-weekly-hours"},
		{"templates/partials/person_edit_modal.gohtml", "person-payment-frequency"},
		{"templates/pages/project_teams.gohtml", "collab-team"},
		{"templates/partials/session_modal.gohtml", "session-until"},
	} {
		src, ok := tpl[c.file]
		if !ok {
			t.Errorf("%s not found", c.file)
			continue
		}
		if strings.Contains(src, `<label for="`+c.id+`">`) {
			t.Errorf("%s: the label of %s is written by hand; use the field_label partial with Optional", c.file, c.id)
		}
		at := strings.Index(src, `"For" "`+c.id+`"`)
		if at < 0 {
			t.Errorf("%s: no field_label call for %s", c.file, c.id)
			continue
		}
		call := src[at : at+strings.Index(src[at:], "}}")]
		if !strings.Contains(call, `"Optional" true`) {
			t.Errorf("%s: the field_label of %s is not marked Optional", c.file, c.id)
		}
	}
}

// Os textos de erro que dizem o que a tela faz: a faixa das horas semanais vem do código da API (e não do "está
// inválido" genérico), e a busca tem o erro embaixo do campo, com o que vem na URL cortado no teto.
func TestErrorTexts_TheScreenSaysWhatTheServerSays(t *testing.T) {
	read := func(name string) string {
		b, err := FS.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	app, org, project := read("static/app.js"), read("static/pages/org.js"), read("static/pages/project.js")
	if !strings.Contains(app, "integer: (min, max, code) =>") {
		t.Error("app.js: rules.integer does not take the code of the API error")
	}
	if !strings.Contains(org, "rules.integer(0, 168, 'person.invalid_week_hours')") {
		t.Error("org.js: the weekly hours do not show the API text for the range")
	}
	for _, want := range []string{
		"Array.from(p.get('q') || '').slice(0, WTT.limits.search).join('')", // a URL entra cortada no teto
		"e.params.field === 'q'", // o erro da busca vai ao campo
	} {
		if !strings.Contains(project, want) {
			t.Errorf("project.js lacks %q", want)
		}
	}
	tasks := templates(t)["templates/pages/project_tasks.gohtml"]
	for _, want := range []string{`data-field="q"`, `{{template "field_error" "q"}}`} {
		if !strings.Contains(tasks, want) {
			t.Errorf("project_tasks.gohtml lacks %s", want)
		}
	}
}
