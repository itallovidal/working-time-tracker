package task_test

import (
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/validate"
)

// codeAndField devolve o código do erro e o parâmetro field, ou "" quando o erro não é de negócio.
func codeAndField(err error) (code, field string) {
	var e *apperr.Error
	if !errors.As(err, &e) {
		return "", ""
	}
	field, _ = e.Params["field"].(string)
	return e.Code, field
}

// validationFixture devolve o serviço, o projeto e uma tarefa para as edições.
func validationFixture(t *testing.T) (*task.Service, string, *task.Task) {
	t.Helper()
	orgSvc, personSvc, projSvc, _, _, taskSvc := setupDeps(t)
	org, _ := orgSvc.Create("Org")
	if _, err := personSvc.Create(org.ID.String(), "John", "john@test.com"); err != nil {
		t.Fatalf("create person: %v", err)
	}
	proj, err := projSvc.Create(org.ID.String(), "Project", "", 0, project.Routine{})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	tk, err := taskSvc.Create(proj.ID.String(), "Base", "texto", "", nil)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	return taskSvc, proj.ID.String(), tk
}

// O nome da tarefa é aparado e vai de 1 a 255 caracteres, na criação e na edição.
func TestService_TaskName_Validation(t *testing.T) {
	svc, pid, tk := validationFixture(t)

	cases := []struct {
		name     string
		in       string
		wantCode string // vazio: aceito
		wantName string
	}{
		{"empty", "", "task.name_required", ""},
		{"spaces only", "   \t ", "task.name_required", ""},
		{"trimmed", "  Tarefa  ", "", "Tarefa"},
		{"at the limit", strings.Repeat("a", validate.MaxTaskName), "", strings.Repeat("a", validate.MaxTaskName)},
		{"limit plus one", strings.Repeat("a", validate.MaxTaskName+1), "request.field_too_long", ""},
		{"limit counts characters", strings.Repeat("ç", validate.MaxTaskName), "", strings.Repeat("ç", validate.MaxTaskName)},
		{"spaces do not count", " " + strings.Repeat("a", validate.MaxTaskName) + " ", "", strings.Repeat("a", validate.MaxTaskName)},
	}
	for _, tc := range cases {
		t.Run("create "+tc.name, func(t *testing.T) {
			got, err := svc.Create(pid, tc.in, "", "", nil)
			checkNameResult(t, got, err, tc.wantCode, tc.wantName)
		})
		t.Run("update "+tc.name, func(t *testing.T) {
			got, err := svc.UpdateAs("", tk.ID.String(), tc.in, "", nil, nil, task.Attrs{})
			checkNameResult(t, got, err, tc.wantCode, tc.wantName)
		})
	}
}

func checkNameResult(t *testing.T, got *task.Task, err error, wantCode, wantName string) {
	t.Helper()
	if wantCode == "" {
		if err != nil {
			t.Fatalf("err = %v, want accepted", err)
		}
		if got.Name != wantName {
			t.Errorf("name = %q, want %q", got.Name, wantName)
		}
		return
	}
	code, field := codeAndField(err)
	if code != wantCode || field != "name" {
		t.Errorf("err = %v (field %q), want %s on name", err, field, wantCode)
	}
}

// A descrição vai até 65.536 caracteres; acima, o código antigo diz o campo.
func TestService_TaskDescription_Validation(t *testing.T) {
	svc, pid, tk := validationFixture(t)

	max := validate.MaxTaskDescription
	if _, err := svc.Create(pid, "D", strings.Repeat("é", max), "", nil); err != nil {
		t.Errorf("description at the limit: %v", err)
	}
	_, err := svc.Create(pid, "D", strings.Repeat("é", max+1), "", nil)
	if code, field := codeAndField(err); code != "task.description_too_long" || field != "long_description" {
		t.Errorf("description over the limit = %v (field %q)", err, field)
	}
	d := strings.Repeat("a", max+1)
	_, err = svc.PatchAs("", tk.ID.String(), "Base", &d, nil, validate.Optional[time.Time]{}, task.Attrs{})
	if code, field := codeAndField(err); code != "task.description_too_long" || field != "long_description" {
		t.Errorf("patch description over the limit = %v (field %q)", err, field)
	}
}

// Omitir a descrição mantém a que a tarefa tem; só o texto vazio a esvazia.
func TestService_PatchAs_KeepsTheDescription(t *testing.T) {
	svc, _, tk := validationFixture(t)

	got, err := svc.PatchAs("", tk.ID.String(), "Renomeada", nil, nil, validate.Optional[time.Time]{}, task.Attrs{})
	if err != nil || got.Name != "Renomeada" || got.Description != "texto" {
		t.Fatalf("patch without description = %+v, %v; want the description kept", got, err)
	}
	empty := ""
	got, err = svc.PatchAs("", tk.ID.String(), "Renomeada", &empty, nil, validate.Optional[time.Time]{}, task.Attrs{})
	if err != nil || got.Description != "" {
		t.Errorf("patch with an empty description = %+v, %v; want it emptied", got, err)
	}
}

// O prazo: ausente mantém, null apaga, um valor troca, e uma data antes de 1971 é recusada com o campo.
func TestService_Deadline_Validation(t *testing.T) {
	svc, pid, tk := validationFixture(t)
	id := tk.ID.String()
	day := time.Date(2030, 5, 20, 12, 0, 0, 0, time.UTC)

	got, err := svc.PatchAs("", id, "Base", nil, nil, validate.Optional[time.Time]{Set: true, Value: &day}, task.Attrs{})
	if err != nil || !got.Deadline.Equal(day) {
		t.Fatalf("set the deadline = %v, %v", got, err)
	}
	// Ausente mantém.
	got, err = svc.PatchAs("", id, "Base", nil, nil, validate.Optional[time.Time]{}, task.Attrs{})
	if err != nil || !got.Deadline.Equal(day) {
		t.Errorf("absent deadline = %v, %v; want it kept", got.Deadline, err)
	}
	// Em UpdateAttrs também.
	got, err = svc.PatchAttrsAs("", id, task.Attrs{}, nil, validate.Optional[time.Time]{})
	if err != nil || !got.Deadline.Equal(day) {
		t.Errorf("absent deadline on attributes = %v, %v; want it kept", got.Deadline, err)
	}
	// null apaga (o tempo zero é a tarefa sem prazo).
	got, err = svc.PatchAs("", id, "Base", nil, nil, validate.Optional[time.Time]{Set: true}, task.Attrs{})
	if err != nil || got.HasDeadline() {
		t.Errorf("null deadline = %v, %v; want it cleared", got.Deadline, err)
	}
	if _, err = svc.PatchAttrsAs("", id, task.Attrs{}, nil, validate.Optional[time.Time]{Set: true, Value: &day}); err != nil {
		t.Fatalf("set again: %v", err)
	}
	got, err = svc.PatchAttrsAs("", id, task.Attrs{}, nil, validate.Optional[time.Time]{Set: true})
	if err != nil || got.HasDeadline() {
		t.Errorf("null deadline on attributes = %v, %v; want it cleared", got.Deadline, err)
	}

	old := time.Date(1970, 12, 31, 0, 0, 0, 0, time.UTC)
	boundary := time.Date(1971, 1, 1, 0, 0, 0, 0, time.UTC)
	zero := time.Time{}
	rejected := []struct {
		name string
		call func() error
	}{
		{"create before 1971", func() error { _, err := svc.Create(pid, "X", "", "", &old); return err }},
		{"create zero time", func() error { _, err := svc.Create(pid, "X", "", "", &zero); return err }},
		{"patch before 1971", func() error {
			_, err := svc.PatchAs("", id, "Base", nil, nil, validate.Optional[time.Time]{Set: true, Value: &old}, task.Attrs{})
			return err
		}},
		{"attributes before 1971", func() error {
			_, err := svc.PatchAttrsAs("", id, task.Attrs{}, nil, validate.Optional[time.Time]{Set: true, Value: &old})
			return err
		}},
	}
	for _, tc := range rejected {
		code, field := codeAndField(tc.call())
		if code != "request.field_invalid" || field != "deadline" {
			t.Errorf("%s = %s on %q, want request.field_invalid on deadline", tc.name, code, field)
		}
	}
	if _, err := svc.Create(pid, "No limite", "", "", &boundary); err != nil {
		t.Errorf("a deadline on 1971-01-01 = %v, want accepted", err)
	}
}

// A prioridade vazia vale "none" na criação e na edição; a inválida diz o campo.
func TestService_Priority_EmptyIsNone(t *testing.T) {
	svc, pid, tk := validationFixture(t)
	empty, bad, high := "", "ultra", "high"

	created, err := svc.CreateAs("", pid, "P", "", "", nil, task.Attrs{Priority: &empty})
	if err != nil || created.Priority != "none" {
		t.Errorf("create with an empty priority = %+v, %v; want none", created, err)
	}
	if _, err := svc.UpdateAs("", tk.ID.String(), "Base", "", nil, nil, task.Attrs{Priority: &high}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.UpdateAs("", tk.ID.String(), "Base", "", nil, nil, task.Attrs{Priority: &empty})
	if err != nil || got.Priority != "none" {
		t.Errorf("update with an empty priority = %+v, %v; want none", got, err)
	}
	got, err = svc.UpdateAttrs(tk.ID.String(), task.Attrs{Priority: &high})
	if err != nil || got.Priority != "high" {
		t.Fatal(err)
	}
	got, err = svc.UpdateAttrs(tk.ID.String(), task.Attrs{Priority: &empty})
	if err != nil || got.Priority != "none" {
		t.Errorf("attributes with an empty priority = %+v, %v; want none", got, err)
	}
	for name, call := range map[string]func() error{
		"create": func() error {
			_, err := svc.CreateAs("", pid, "P", "", "", nil, task.Attrs{Priority: &bad})
			return err
		},
		"update": func() error {
			_, err := svc.UpdateAs("", tk.ID.String(), "Base", "", nil, nil, task.Attrs{Priority: &bad})
			return err
		},
		"attributes": func() error { _, err := svc.UpdateAttrs(tk.ID.String(), task.Attrs{Priority: &bad}); return err },
	} {
		if code, field := codeAndField(call()); code != "task.invalid_priority" || field != "priority" {
			t.Errorf("%s with a bad priority = %s on %q, want task.invalid_priority on priority", name, code, field)
		}
	}
}

// O item externo: aparado, até 128 caracteres, no formato do tipo; o link é http(s), obrigatório e até 2.048.
func TestService_LinkExternalItem_Validation(t *testing.T) {
	svc, pid, _ := validationFixture(t)
	github := createIntegrationOf(t, pid, "github")
	trello := createIntegrationOf(t, pid, "trello")
	const link = "https://example.com/item"

	longPath := "https://example.com/" + strings.Repeat("a", validate.MaxItemURL)
	atLimit := "https://example.com/" + strings.Repeat("a", validate.MaxItemURL-len("https://example.com/"))

	cases := []struct {
		name            string
		integration     string
		item, url       string
		wantCode, field string
		wantItem        string
	}{
		{"no integration", "", "42", link, "task.link_fields_required", "integration_id", ""},
		{"integration not a uuid", "abc", "42", link, "task.integration_not_found", "integration_id", ""},
		{"empty item", github, "", link, "task.link_fields_required", "external_item_id", ""},
		{"item of spaces", github, "   ", link, "task.link_fields_required", "external_item_id", ""},
		{"empty link", github, "42", "", "task.link_fields_required", "external_item_url", ""},
		{"link of spaces", github, "42", "   ", "task.link_fields_required", "external_item_url", ""},
		{"number with letters", github, "abc", link, "request.field_invalid", "external_item_id", ""},
		{"negative number", github, "-3", link, "request.field_invalid", "external_item_id", ""},
		{"zero", github, "0", link, "request.field_invalid", "external_item_id", ""},
		{"number trimmed and canonical", github, " 007 ", link, "", "", "7"},
		{"link with another scheme", trello, "AbCd1234", "javascript:alert(1)", "request.field_invalid", "external_item_url", ""},
		{"link with a space", trello, "AbCd1234", "https://exa mple.com/x", "request.field_invalid", "external_item_url", ""},
		{"link of a file", trello, "AbCd1234", "file:///etc/passwd", "request.field_invalid", "external_item_url", ""},
		{"link over the limit", trello, "AbCd1234", longPath, "request.field_too_long", "external_item_url", ""},
		{"item over the limit", trello, strings.Repeat("a", validate.MaxExternalItem+1), link, "request.field_too_long", "external_item_id", ""},
		{"item at the limit", trello, strings.Repeat("b", validate.MaxExternalItem), link, "", "", strings.Repeat("b", validate.MaxExternalItem)},
		{"link at the limit", trello, "AtLimit1", atLimit, "", "", "AtLimit1"},
		{"card address becomes the short link", trello, "https://trello.com/c/PorUrl01/3-nome", link, "", "", "PorUrl01"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Cada caso numa tarefa nova: o vínculo é um por integração.
			fresh, err := svc.Create(pid, "T", "", "", nil)
			if err != nil {
				t.Fatal(err)
			}
			got, err := svc.LinkExternalItem(fresh.ID.String(), tc.integration, tc.item, tc.url)
			if tc.wantCode == "" {
				if err != nil {
					t.Fatalf("err = %v, want accepted", err)
				}
				if len(got.Links) != 1 || got.Links[0].ItemID != tc.wantItem {
					t.Errorf("links = %+v, want the item %q", got.Links, tc.wantItem)
				}
				return
			}
			code, field := codeAndField(err)
			if code != tc.wantCode || field != tc.field {
				t.Errorf("err = %v (field %q), want %s on %s", err, field, tc.wantCode, tc.field)
			}
		})
	}

	// O link sai como o servidor o normalizou: sem esquema, assume https.
	fresh, _ := svc.Create(pid, "Sem esquema", "", "", nil)
	got, err := svc.LinkExternalItem(fresh.ID.String(), github, "9", "github.com/o/r/issues/9")
	if err != nil || got.Links[0].URL != "https://github.com/o/r/issues/9" {
		t.Errorf("link without scheme = %+v, %v; want https assumed", got, err)
	}
}

// Um item já ligado a outra tarefa, ou uma tarefa já ligada à integração, é conflito: os códigos antigos
// ganham o campo, e o handler os responde com 409.
func TestService_LinkExternalItem_ConflictsSayTheField(t *testing.T) {
	svc, pid, _ := validationFixture(t)
	trello := createIntegrationOf(t, pid, "trello")
	a, _ := svc.Create(pid, "A", "", "", nil)
	b, _ := svc.Create(pid, "B", "", "", nil)
	const link = "https://trello.com/c/Same0001"

	if _, err := svc.LinkExternalItem(a.ID.String(), trello, "Same0001", link); err != nil {
		t.Fatal(err)
	}
	_, err := svc.LinkExternalItem(b.ID.String(), trello, "Same0001", link)
	if !errors.Is(err, task.ErrItemTaken) {
		t.Errorf("item of another task = %v, want task.item_taken", err)
	}
	if code, field := codeAndField(err); code != "task.item_taken" || field != "external_item_id" {
		t.Errorf("item of another task: code %q field %q, want task.item_taken on external_item_id", code, field)
	}
	_, err = svc.LinkExternalItem(a.ID.String(), trello, "Other002", link)
	if !errors.Is(err, task.ErrAlreadyLinked) {
		t.Errorf("a second item in the same integration = %v, want task.already_linked", err)
	}
	if code, field := codeAndField(err); code != "task.already_linked" || field != "integration_id" {
		t.Errorf("a second item in the same integration: code %q field %q, want task.already_linked on integration_id", code, field)
	}
	if got := apperr.Registered()["task.item_taken"].Status; got != 409 {
		t.Errorf("task.item_taken is declared with %d, want 409", got)
	}
	if got := apperr.Registered()["task.already_linked"].Status; got != 409 {
		t.Errorf("task.already_linked is declared with %d, want 409", got)
	}
	if got := apperr.Registered()["label.name_taken"].Status; got != 409 {
		t.Errorf("label.name_taken is declared with %d, want 409", got)
	}
}

// ClipName corta o título de uma plataforma em 255 caracteres sem partir um caractere no meio.
func TestClipName(t *testing.T) {
	max := validate.MaxTaskName
	cases := []struct{ name, in, want string }{
		{"short", "  Olá  ", "Olá"},
		{"at the limit", strings.Repeat("a", max), strings.Repeat("a", max)},
		{"over by one", strings.Repeat("a", max+1), strings.Repeat("a", max)},
		{"multibyte", strings.Repeat("ç", max+10), strings.Repeat("ç", max)},
		{"emoji", strings.Repeat("😀", max+3), strings.Repeat("😀", max)},
		{"cut leaves no trailing space", strings.Repeat("a", max-1) + " bbb", strings.Repeat("a", max-1)},
	}
	for _, tc := range cases {
		got := task.ClipName(tc.in)
		if got != tc.want || !utf8.ValidString(got) {
			t.Errorf("%s: ClipName = %d runes, want %d valid ones", tc.name, utf8.RuneCountInString(got), utf8.RuneCountInString(tc.want))
		}
	}
	if got := task.ClipName("\xff\xfe" + strings.Repeat("a", max+5)); !utf8.ValidString(got) || utf8.RuneCountInString(got) > max {
		t.Errorf("invalid UTF-8 input gave %d runes, valid=%v", utf8.RuneCountInString(got), utf8.ValidString(got))
	}
}

// O nome do rótulo diz o campo, e repetido (sem diferenciar caixa) é label.name_taken.
func TestService_Labels_FieldsAndLimits(t *testing.T) {
	svc, pid, _ := validationFixture(t)

	if _, err := svc.CreateLabel(pid, strings.Repeat("a", 50)); err != nil {
		t.Errorf("label at the limit: %v", err)
	}
	cases := []struct{ name, in, code string }{
		{"empty", "", "label.name_required"},
		{"spaces", "   ", "label.name_required"},
		{"over", strings.Repeat("a", 51), "label.name_too_long"},
		{"taken with another case", strings.ToUpper(strings.Repeat("a", 50)), "label.name_taken"},
	}
	for _, tc := range cases {
		_, err := svc.CreateLabel(pid, tc.in)
		if code, field := codeAndField(err); code != tc.code || field != "name" {
			t.Errorf("%s = %s on %q, want %s on name", tc.name, code, field, tc.code)
		}
	}
}
