package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
)

// errorOf devolve o código e os parâmetros do corpo de uma resposta de erro.
func errorOf(t *testing.T, body string) (string, map[string]any) {
	t.Helper()
	var got struct {
		Error struct {
			Code   string         `json:"code"`
			Params map[string]any `json:"params"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
	return got.Error.Code, got.Error.Params
}

type httpCase struct {
	name, method, path, body string
	status                   int
	code, field              string
}

func runHTTPCases(t *testing.T, e *echo.Echo, session string, cases []httpCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(e, tc.method, tc.path, tc.body, session)
			if rec.Code != tc.status {
				t.Fatalf("%s %s = %d %s, want %d", tc.method, tc.path, rec.Code, rec.Body.String(), tc.status)
			}
			if tc.code == "" {
				return
			}
			code, params := errorOf(t, rec.Body.String())
			if code != tc.code {
				t.Errorf("code = %q, want %q (%s)", code, tc.code, rec.Body.String())
			}
			if got, _ := params["field"].(string); got != tc.field {
				t.Errorf("params.field = %q, want %q (%s)", got, tc.field, rec.Body.String())
			}
		})
	}
}

// Tarefas pela API: cada recusa devolve o código, o status e o campo do corpo que errou.
func TestValidation_Tasks(t *testing.T) {
	app, _ := syncServer(t)
	e := app.Echo
	admin := signup(t, e, "Org", "ana@test.com")
	prj := "/api/projects/" + createProject(t, e, admin, "Alfa")
	tasks := prj + "/tasks"

	created := decode(t, do(e, "POST", tasks, `{"name":"Base","description":"texto","deadline":"2031-04-05T00:00:00Z","priority":"high"}`, admin.session))
	id := created["id"].(string)
	task := "/api/tasks/" + id

	runHTTPCases(t, e, admin.session, []httpCase{
		{"create without a name", "POST", tasks, `{}`, 400, "task.name_required", "name"},
		{"create with a name of spaces", "POST", tasks, `{"name":"    "}`, 400, "task.name_required", "name"},
		{"create with a name at the limit", "POST", tasks, `{"name":"` + strings.Repeat("a", 255) + `"}`, 201, "", ""},
		{"create with a name over the limit", "POST", tasks, `{"name":"` + strings.Repeat("a", 256) + `"}`, 400, "request.field_too_long", "name"},
		{"create with a description at the limit", "POST", tasks, `{"name":"D","description":"` + strings.Repeat("a", 65536) + `"}`, 201, "", ""},
		{"create with a description over the limit", "POST", tasks, `{"name":"D","description":"` + strings.Repeat("a", 65537) + `"}`, 400, "task.description_too_long", "long_description"},
		{"create with a deadline before 1971", "POST", tasks, `{"name":"D","deadline":"1970-06-01T00:00:00Z"}`, 400, "request.field_invalid", "deadline"},
		{"create with a bad priority", "POST", tasks, `{"name":"D","priority":"ultra"}`, 400, "task.invalid_priority", "priority"},
		{"create with an empty priority", "POST", tasks, `{"name":"D","priority":""}`, 201, "", ""},
		{"create with a bad assignee", "POST", tasks, `{"name":"D","assignee_id":"x"}`, 400, "task.invalid_assignee", "assignee_id"},
		{"create with a bad label", "POST", tasks, `{"name":"D","label_ids":["x"]}`, 400, "task.label_other_project", "label_ids"},

		{"edit without a name", "PATCH", task, `{"description":"x"}`, 400, "task.name_required", "name"},
		{"edit with a name over the limit", "PATCH", task, `{"name":"` + strings.Repeat("a", 256) + `"}`, 400, "request.field_too_long", "name"},
		{"edit with a description at the limit", "PATCH", task, `{"name":"Base","description":"` + strings.Repeat("a", 65536) + `"}`, 200, "", ""},
		{"edit with a description over the limit", "PATCH", task, `{"name":"Base","description":"` + strings.Repeat("a", 65537) + `"}`, 400, "task.description_too_long", "long_description"},
		// O servidor conta a descrição como veio, sem aparar: a quebra de linha da ponta entra na conta, e a tela
		// (rules.maxChars) conta igual.
		{"edit with a description over the limit by a trailing newline", "PATCH", task, `{"name":"Base","description":"` + strings.Repeat("a", 65536) + `\n"}`, 400, "task.description_too_long", "long_description"},
		{"edit with a deadline before 1971", "PATCH", task, `{"name":"Base","deadline":"1960-01-01T00:00:00Z"}`, 400, "request.field_invalid", "deadline"},
		{"edit with a bad status", "PATCH", task, `{"name":"Base","status":"x"}`, 400, "task.invalid_status", "status"},
		{"edit with an empty priority", "PATCH", task, `{"name":"Base","priority":""}`, 200, "", ""},
		{"attributes with a bad priority", "PATCH", task + "/attributes", `{"priority":"x"}`, 400, "task.invalid_priority", "priority"},
		{"attributes with a deadline before 1971", "PATCH", task + "/attributes", `{"deadline":"1970-01-01T00:00:00Z"}`, 400, "request.field_invalid", "deadline"},
		{"attributes with a bad assignee", "PATCH", task + "/attributes", `{"assignee_id":"x"}`, 400, "task.invalid_assignee", "assignee_id"},

		{"list with a search over the limit", "GET", tasks + "?q=" + strings.Repeat("a", 101), "", 400, "task.query_too_long", "q"},
		{"list with a search at the limit", "GET", tasks + "?q=" + strings.Repeat("a", 100), "", 200, "", ""},
		{"list with a bad priority", "GET", tasks + "?priority=x", "", 400, "task.invalid_priority_filter", "priority"},
	})
}

// deadlineOf lê o prazo da resposta; cleared diz se é o "sem prazo" (o tempo zero, ou antes de 1971).
func deadlineOf(t *testing.T, task map[string]any) (day string, cleared bool) {
	t.Helper()
	raw, _ := task["deadline"].(string)
	at, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatalf("deadline %q: %v", raw, err)
	}
	return at.UTC().Format("2006-01-02"), at.Year() < 1971
}

// Editar a tarefa: a descrição omitida mantém, e o prazo ausente mantém, null apaga.
func TestValidation_TaskPatchKeepsWhatIsOmitted(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	prj := "/api/projects/" + createProject(t, e, admin, "Alfa")
	created := decode(t, do(e, "POST", prj+"/tasks", `{"name":"Base","description":"texto","deadline":"2031-04-05T12:00:00Z"}`, admin.session))
	task := "/api/tasks/" + created["id"].(string)

	got := decode(t, do(e, "PATCH", task, `{"name":"Nova"}`, admin.session))
	if day, cleared := deadlineOf(t, got); got["name"] != "Nova" || got["description"] != "texto" || day != "2031-04-05" || cleared {
		t.Errorf("patch with only the name = %v; want the description and the deadline kept", got)
	}
	got = decode(t, do(e, "PATCH", task, `{"name":"Nova","description":""}`, admin.session))
	if day, _ := deadlineOf(t, got); got["description"] != "" || day != "2031-04-05" {
		t.Errorf("patch with an empty description = %v; want it emptied and the deadline kept", got)
	}
	got = decode(t, do(e, "PATCH", task+"/attributes", `{"priority":"low"}`, admin.session))
	if day, _ := deadlineOf(t, got); day != "2031-04-05" {
		t.Errorf("attributes without a deadline = %v; want it kept", got["deadline"])
	}
	got = decode(t, do(e, "PATCH", task+"/attributes", `{"deadline":null}`, admin.session))
	if _, cleared := deadlineOf(t, got); !cleared {
		t.Errorf("attributes with a null deadline = %v; want it cleared", got["deadline"])
	}
	do(e, "PATCH", task, `{"name":"Nova","deadline":"2032-01-02T12:00:00Z"}`, admin.session)
	got = decode(t, do(e, "PATCH", task, `{"name":"Nova","deadline":null}`, admin.session))
	if _, cleared := deadlineOf(t, got); !cleared {
		t.Errorf("patch with a null deadline = %v; want it cleared", got["deadline"])
	}
	got = decode(t, do(e, "PATCH", task, `{"name":" Nome com espaços "}`, admin.session))
	if got["name"] != "Nome com espaços" {
		t.Errorf("the name is saved trimmed, got %q", got["name"])
	}
}

// Etiquetas, vínculo com item externo e integrações: o código, o status (409 onde é conflito) e o campo.
func TestValidation_LinksLabelsAndIntegrations(t *testing.T) {
	app, _ := syncServer(t)
	e := app.Echo
	admin := signup(t, e, "Org", "ana@test.com")
	prj := "/api/projects/" + createProject(t, e, admin, "Alfa")

	integ := decode(t, do(e, "POST", prj+"/integrations", `{"type":"github","display_name":"Repo","token":"tok","metadata":{"repo":"owner/repo"}}`, admin.session))
	integID := integ["id"].(string)
	a := decode(t, do(e, "POST", prj+"/tasks", `{"name":"A"}`, admin.session))["id"].(string)
	b := decode(t, do(e, "POST", prj+"/tasks", `{"name":"B"}`, admin.session))["id"].(string)
	linkA := "/api/tasks/" + a + "/link-external-item"
	linkB := "/api/tasks/" + b + "/link-external-item"
	good := func(item string) string {
		return `{"integration_id":"` + integID + `","external_item_id":"` + item + `","external_item_url":"https://github.com/owner/repo/issues/` + item + `"}`
	}

	runHTTPCases(t, e, admin.session, []httpCase{
		{"link without the integration", "POST", linkA, `{"external_item_id":"1","external_item_url":"https://github.com/o/r/issues/1"}`, 400, "task.link_fields_required", "integration_id"},
		{"link without the item", "POST", linkA, `{"integration_id":"` + integID + `","external_item_url":"https://github.com/o/r/issues/1"}`, 400, "task.link_fields_required", "external_item_id"},
		{"link with an item of spaces", "POST", linkA, `{"integration_id":"` + integID + `","external_item_id":"  ","external_item_url":"https://github.com/o/r/issues/1"}`, 400, "task.link_fields_required", "external_item_id"},
		{"link without the address", "POST", linkA, `{"integration_id":"` + integID + `","external_item_id":"1"}`, 400, "task.link_fields_required", "external_item_url"},
		{"link with a bad integration", "POST", linkA, `{"integration_id":"x","external_item_id":"1","external_item_url":"https://github.com/o/r/issues/1"}`, 400, "task.integration_not_found", "integration_id"},
		{"link with an item that is not a number", "POST", linkA, `{"integration_id":"` + integID + `","external_item_id":"abc","external_item_url":"https://github.com/o/r/issues/1"}`, 400, "request.field_invalid", "external_item_id"},
		{"link with a number too big", "POST", linkA, `{"integration_id":"` + integID + `","external_item_id":"` + strings.Repeat("1", 129) + `","external_item_url":"https://github.com/o/r/issues/1"}`, 400, "request.field_invalid", "external_item_id"},
		{"link with a script address", "POST", linkA, `{"integration_id":"` + integID + `","external_item_id":"1","external_item_url":"javascript:alert(1)"}`, 400, "request.field_invalid", "external_item_url"},
		{"link with an address over the limit", "POST", linkA, `{"integration_id":"` + integID + `","external_item_id":"1","external_item_url":"https://example.com/` + strings.Repeat("a", 2049) + `"}`, 400, "request.field_too_long", "external_item_url"},
		{"link", "POST", linkA, good("1"), 200, "", ""},
		{"link an item that has an owner", "POST", linkB, good("1"), 409, "task.item_taken", "external_item_id"},
		{"link a second item in the integration", "POST", linkA, good("2"), 409, "task.already_linked", "integration_id"},

		{"label without a name", "POST", prj + "/labels", `{"name":"  "}`, 400, "label.name_required", "name"},
		{"label over the limit", "POST", prj + "/labels", `{"name":"` + strings.Repeat("a", 51) + `"}`, 400, "label.name_too_long", "name"},
		{"label", "POST", prj + "/labels", `{"name":"Bug"}`, 201, "", ""},
		{"label with a name that is taken", "POST", prj + "/labels", `{"name":"bug"}`, 409, "label.name_taken", "name"},

		{"integration with a name of spaces", "POST", prj + "/integrations", `{"type":"github","display_name":"   ","token":"tok","metadata":{"repo":"owner/repo"}}`, 400, "integration.name_required", "display_name"},
		{"integration with a name over the limit", "POST", prj + "/integrations", `{"type":"github","display_name":"` + strings.Repeat("a", 121) + `","token":"tok","metadata":{"repo":"owner/repo"}}`, 400, "request.field_too_long", "display_name"},
		{"integration with a token over the limit", "POST", prj + "/integrations", `{"type":"github","display_name":"X","token":"` + strings.Repeat("t", 513) + `","metadata":{"repo":"owner/repo"}}`, 400, "request.field_too_long", "token"},
		{"integration with a bad repository", "POST", prj + "/integrations", `{"type":"github","display_name":"X","token":"tok","metadata":{"repo":"not a repo"}}`, 400, "integration.github_invalid_repo", "repo"},
		{"integration without a type", "POST", prj + "/integrations", `{"display_name":"X","token":"tok"}`, 400, "integration.type_required", "type"},
		{"edit the name to spaces", "PATCH", "/api/integrations/" + integID, `{"display_name":"   "}`, 400, "integration.name_required", "display_name"},
		{"edit the name over the limit", "PATCH", "/api/integrations/" + integID, `{"display_name":"` + strings.Repeat("a", 121) + `"}`, 400, "request.field_too_long", "display_name"},
		{"edit the token over the limit", "PATCH", "/api/integrations/" + integID, `{"token":"` + strings.Repeat("t", 513) + `"}`, 400, "request.field_too_long", "token"},
		{"edit the repository", "PATCH", "/api/integrations/" + integID, `{"metadata":{"repo":"not a repo"}}`, 400, "integration.github_invalid_repo", "repo"},
	})

	// O nome recusado não foi gravado, e o aceito é gravado aparado.
	got := decode(t, do(e, "GET", "/api/integrations/"+integID, "", admin.session))
	if got["display_name"] != "Repo" {
		t.Errorf("name after the refused edits = %q, want Repo", got["display_name"])
	}
	got = decode(t, do(e, "PATCH", "/api/integrations/"+integID, `{"display_name":"  Novo nome  "}`, admin.session))
	if got["display_name"] != "Novo nome" {
		t.Errorf("name after an edit with spaces = %q, want it trimmed", got["display_name"])
	}
}

// Sessões: o intervalo diz o campo, a lista recusa o id malformado e parar o ponto compara o projeto.
func TestValidation_WorkSessions(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	bia := invite(t, e, admin, "bia@test.com", "member")
	one := createProject(t, e, admin, "Um")
	two := createProject(t, e, admin, "Dois")
	allocate(t, e, admin, one, bia.id, 2000)
	allocate(t, e, admin, two, bia.id, 2000)
	prj := "/api/projects/" + one
	taskA := decode(t, do(e, "POST", prj+"/tasks", `{"name":"A"}`, admin.session))["id"].(string)
	taskB := decode(t, do(e, "POST", prj+"/tasks", `{"name":"B"}`, admin.session))["id"].(string)

	opened := decode(t, do(e, "POST", prj+"/work-sessions/clock-in", `{"task_id":"`+taskA+`"}`, bia.session))
	sid := opened["id"].(string)
	tasksPath := prj + "/work-sessions/" + sid + "/tasks"

	runHTTPCases(t, e, bia.session, []httpCase{
		{"clock-in without a task", "POST", prj + "/work-sessions/clock-in", `{}`, 400, "work_session.task_required", "task_id"},
		{"clock-out through another project", "POST", "/api/projects/" + two + "/work-sessions/clock-out", `{}`, 400, "work_session.open_in_other_project", ""},
		{"list with a malformed task", "GET", prj + "/work-sessions?task_id=abc", "", 400, "work_session.invalid_task_filter", "task_id"},
		{"total with a malformed task", "GET", prj + "/work-sessions/total?task_id=abc", "", 400, "work_session.invalid_task_filter", "task_id"},
		{"add without a task", "POST", tasksPath, `{}`, 400, "work_session.task_required", "task_id"},
		{"add an unknown task", "POST", tasksPath, `{"task_id":"00000000-0000-0000-0000-000000000000"}`, 400, "work_session.task_not_found", "task_id"},
		{"add with a start before the session", "POST", tasksPath, `{"task_id":"` + taskB + `","from_at":"2000-01-01T00:00:00Z"}`, 400, "work_session.invalid_interval", "from_at"},
		{"add with an end before the start", "POST", tasksPath, `{"task_id":"` + taskB + `","until_at":"2000-01-01T00:00:00Z"}`, 400, "work_session.invalid_interval", "until_at"},
		{"add the same task again", "POST", tasksPath, `{"task_id":"` + taskA + `"}`, 400, "work_session.task_overlap", "from_at"},
		{"add with a start that is not a date", "POST", tasksPath, `{"task_id":"` + taskB + `","from_at":"ontem"}`, 400, "request.invalid_body", "from_at"},
	})

	// Quem vê as sessões de todos recebe o 400; um membro, que só vê as próprias, recebe antes o 403 de outra pessoa.
	runHTTPCases(t, e, admin.session, []httpCase{
		{"list with a malformed person", "GET", prj + "/work-sessions?person_id=abc", "", 400, "work_session.invalid_person_filter", "person_id"},
		{"total with a malformed person", "GET", prj + "/work-sessions/total?person_id=abc", "", 400, "work_session.invalid_person_filter", "person_id"},
	})

	// Pelo projeto certo o ponto é fechado.
	if rec := do(e, "POST", prj+"/work-sessions/clock-out", `{}`, bia.session); rec.Code != http.StatusOK {
		t.Errorf("clock-out through its own project = %d: %s", rec.Code, rec.Body.String())
	}
}
