package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"working-time-tracker/ent/apitoken"
	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/testutil"
)

// O servidor MCP (/mcp) de ponta a ponta: o servidor de verdade num endereço local e o cliente do SDK, como o
// Claude Code fala com ele.

// bearer põe o token do Claude em toda requisição do cliente MCP.
type bearer struct {
	token string
	base  http.RoundTripper
}

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(r)
}

// mcpEndpoint sobe o servidor num endereço local e devolve o endereço do /mcp.
func mcpEndpoint(t *testing.T, e *echo.Echo) string {
	t.Helper()
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	return srv.URL + "/mcp"
}

// connectMCP conecta o cliente do SDK com o token.
func connectMCP(t *testing.T, endpoint, token string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint:   endpoint,
		HTTPClient: &http.Client{Transport: bearer{token: token, base: http.DefaultTransport}},
		MaxRetries: -1,
	}
	cs, err := client.Connect(t.Context(), transport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// callTool chama a ferramenta e devolve a saída estruturada; um erro da ferramenta falha o teste.
func callTool(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) map[string]any {
	t.Helper()
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("%s %v: tool error %s", name, args, toolText(res))
	}
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("%s: structured content %s: %v", name, b, err)
	}
	return out
}

// toolError chama a ferramenta esperando um erro dela, e devolve o texto do erro.
func toolError(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if !res.IsError {
		t.Fatalf("%s %v: no error, want one (got %v)", name, args, res.StructuredContent)
	}
	return toolText(res)
}

func toolText(res *mcp.CallToolResult) string {
	var parts []string
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, " ")
}

// wantCode confere que o erro da ferramenta traz o código.
func wantCode(t *testing.T, text, code string) {
	t.Helper()
	if !strings.Contains(text, "("+code) {
		t.Errorf("error %q, want code %s", text, code)
	}
}

// rawMCP manda um initialize ao /mcp direto, com os cabeçalhos dados, e devolve a resposta.
func rawMCP(e *echo.Echo, headers map[string]string, cookie string) *httptest.ResponseRecorder {
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestMCP_NeedsAValidToken(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "dono@example.com")
	revokedID, revoked := createToken(t, e, owner, "revogado")
	_, expired := createToken(t, e, owner, "vencido")
	_, valid := createToken(t, e, owner, "valido")
	do(e, "DELETE", "/api/auth/tokens/"+revokedID, "", owner.session)
	if _, err := testClient.APIToken.Update().Where(apitoken.NameEQ("vencido")).
		SetExpiresAt(time.Now().Add(-time.Minute)).Save(t.Context()); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		headers map[string]string
		cookie  string
	}{
		{"no token", nil, ""},
		{"only the session cookie", nil, owner.session},
		{"the session token as bearer", map[string]string{"Authorization": "Bearer " + owner.session}, ""},
		{"unknown token", map[string]string{"Authorization": "Bearer wtt_nao-existe"}, ""},
		{"revoked token", map[string]string{"Authorization": "Bearer " + revoked}, ""},
		{"expired token", map[string]string{"Authorization": "Bearer " + expired}, ""},
		{"not bearer", map[string]string{"Authorization": "Basic " + valid}, ""},
	}
	for _, c := range cases {
		rec := rawMCP(e, c.headers, c.cookie)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s = %d %s, want 401", c.name, rec.Code, rec.Body.String())
			continue
		}
		if got := rec.Header().Get("WWW-Authenticate"); !strings.HasPrefix(got, "Bearer") || !strings.Contains(got, `scope="mcp"`) {
			t.Errorf("%s: WWW-Authenticate = %q, want a Bearer challenge", c.name, got)
		}
	}

	rec := rawMCP(e, map[string]string{"Authorization": "Bearer " + valid}, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"working-time-tracker"`) {
		t.Fatalf("initialize with a valid token = %d %s", rec.Code, rec.Body.String())
	}
	// Um navegador em outro site não chama o /mcp, nem com o token.
	rec = rawMCP(e, map[string]string{"Authorization": "Bearer " + valid, "Origin": "https://outro.example", "Sec-Fetch-Site": "cross-site"}, "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("cross-site request = %d, want 403", rec.Code)
	}
	// O uso fica registrado.
	row := testClient.APIToken.Query().Where(apitoken.NameEQ("valido")).OnlyX(t.Context())
	if row.LastUsedAt == nil || time.Since(*row.LastUsedAt) > time.Minute {
		t.Errorf("last_used_at = %v, want now", row.LastUsedAt)
	}
}

func TestMCP_ListsTheTools(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "dono@example.com")
	_, token := createToken(t, e, owner, "claude")
	cs := connectMCP(t, mcpEndpoint(t, e), token)

	res, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	readOnly := map[string]bool{}
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
		readOnly[tool.Name] = tool.Annotations != nil && tool.Annotations.ReadOnlyHint
	}
	slices.Sort(names)
	want := []string{
		"add_task_to_session", "claim_task", "clock_in", "clock_out", "create_task", "get_active_session", "get_task",
		"list_labels", "list_projects", "list_tasks", "my_hours", "my_tasks", "remove_session_task",
		"stop_session_task", "sync_task", "update_task", "whoami",
	}
	if !slices.Equal(names, want) {
		t.Fatalf("tools = %v, want %v", names, want)
	}
	for _, name := range []string{"whoami", "list_projects", "list_tasks", "my_tasks", "get_task", "list_labels", "get_active_session", "my_hours"} {
		if !readOnly[name] {
			t.Errorf("%s is not marked read-only", name)
		}
	}
	for _, name := range []string{"create_task", "update_task", "clock_in", "clock_out"} {
		if readOnly[name] {
			t.Errorf("%s is marked read-only", name)
		}
	}
	if !strings.Contains(cs.InitializeResult().Instructions, "add_task_to_session") {
		t.Errorf("instructions do not explain the session: %q", cs.InitializeResult().Instructions)
	}
}

func TestMCP_Tasks(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "dono@example.com")
	member := invite(t, e, owner, "membro@example.com", "member")
	projectID := createProject(t, e, owner, "Alfa")
	prj := "/api/projects/" + projectID
	allocate(t, e, owner, projectID, member.id, 5000)
	if rec := do(e, "POST", prj+"/labels", `{"name":"bug"}`, owner.session); rec.Code != http.StatusCreated {
		t.Fatalf("create label = %d: %s", rec.Code, rec.Body.String())
	}
	_, token := createToken(t, e, member, "claude")
	cs := connectMCP(t, mcpEndpoint(t, e), token)

	who := callTool(t, cs, "whoami", nil)
	if who["role"] != "member" || who["email"] != "membro@example.com" || who["open_session"] != nil || who["token_expires_at"] == "" {
		t.Errorf("whoami = %v", who)
	}
	projects := callTool(t, cs, "list_projects", nil)["projects"].([]any)
	if len(projects) != 1 || projects[0].(map[string]any)["id"] != projectID {
		t.Fatalf("list_projects = %v, want Alfa", projects)
	}
	if labels := callTool(t, cs, "list_labels", map[string]any{"project_id": projectID})["labels"]; !slices.Equal(toStrings(labels), []string{"bug"}) {
		t.Errorf("list_labels = %v", labels)
	}

	// Criar: a etiqueta pelo nome, o prazo pelo dia, e a pessoa como responsável.
	created := callTool(t, cs, "create_task", map[string]any{
		"project_id": projectID, "name": "Escrever os testes", "description": "Com **Markdown**",
		"priority": "high", "deadline": "2026-12-01", "labels": []string{"Bug"}, "assign_to_me": true,
	})
	taskID := created["id"].(string)
	if created["status"] != "backlog" || created["priority"] != "high" || created["deadline"] != "2026-12-01" ||
		!slices.Equal(toStrings(created["labels"]), []string{"bug"}) || created["description"] != "Com **Markdown**" {
		t.Errorf("create_task = %v", created)
	}
	if a, _ := created["assignee"].(map[string]any); a == nil || a["is_you"] != true || a["id"] != member.id {
		t.Errorf("assignee = %v, want the member", created["assignee"])
	}
	api := decode(t, do(e, "GET", "/api/tasks/"+taskID, "", owner.session))
	if api["name"] != "Escrever os testes" || api["assignee_id"] != member.id {
		t.Errorf("task through the API = %v", api)
	}
	if deadline, _ := time.Parse(time.RFC3339, api["deadline"].(string)); deadline.In(saoPaulo(t)).Format("2006-01-02 15:04") != "2026-12-01 23:59" {
		t.Errorf("deadline = %v, want 23:59 of the day in the organization's time zone", api["deadline"])
	}
	wantCode(t, toolError(t, cs, "create_task", map[string]any{"project_id": projectID, "name": "x", "labels": []string{"feature"}}), "mcp.unknown_label")
	wantCode(t, toolError(t, cs, "create_task", map[string]any{"project_id": projectID, "name": "x", "deadline": "amanhã"}), "mcp.invalid_date")
	wantCode(t, toolError(t, cs, "create_task", map[string]any{"project_id": projectID, "name": "  "}), "task.name_required")

	// Atualizar: só o que vem muda; o nome fica.
	updated := callTool(t, cs, "update_task", map[string]any{
		"task_id": taskID, "status": "awaiting_closure", "clear_deadline": true, "labels": []string{}, "assignee": "none",
	})
	if updated["name"] != "Escrever os testes" || updated["status"] != "awaiting_closure" || updated["deadline"] != "" ||
		len(toStrings(updated["labels"])) != 0 || updated["assignee"] != nil || updated["priority"] != "high" {
		t.Errorf("update_task = %v", updated)
	}
	if renamed := callTool(t, cs, "update_task", map[string]any{"task_id": taskID, "name": "Testes do MCP", "description": ""}); renamed["name"] != "Testes do MCP" {
		t.Errorf("rename = %v", renamed)
	}
	if api := decode(t, do(e, "GET", "/api/tasks/"+taskID, "", owner.session)); api["description"] != "" {
		t.Errorf("description = %q, want it cleared", api["description"])
	}
	// Um status fora da lista nem chega à ferramenta: o esquema dela o recusa.
	if text := toolError(t, cs, "update_task", map[string]any{"task_id": taskID, "status": "done"}); !strings.Contains(text, "enum") {
		t.Errorf("invalid status error = %q", text)
	}

	// Pegar: a sem responsável vira da pessoa; a de outra pessoa, não.
	free := decode(t, do(e, "POST", prj+"/tasks", `{"name":"Livre"}`, owner.session))["id"].(string)
	taken := decode(t, do(e, "POST", prj+"/tasks", `{"name":"Da dona","assignee_id":"`+owner.id+`"}`, owner.session))["id"].(string)
	if claimed := callTool(t, cs, "claim_task", map[string]any{"task_id": free}); claimed["assignee"].(map[string]any)["is_you"] != true {
		t.Errorf("claim_task = %v", claimed)
	}
	wantCode(t, toolError(t, cs, "claim_task", map[string]any{"task_id": taken}), "task.already_assigned")

	// Listar: sem status, só as abertas; com filtros.
	list := callTool(t, cs, "list_tasks", map[string]any{"project_id": projectID})
	if list["total"] != 3.0 {
		t.Errorf("list_tasks total = %v, want the three open tasks", list["total"])
	}
	mine := callTool(t, cs, "list_tasks", map[string]any{"project_id": projectID, "assignee": "me"})
	if items := mine["items"].([]any); len(items) != 1 || items[0].(map[string]any)["id"] != free {
		t.Errorf("list_tasks assignee=me = %v", mine["items"])
	}
	callTool(t, cs, "update_task", map[string]any{"task_id": free, "status": "closed"})
	if closed := callTool(t, cs, "list_tasks", map[string]any{"project_id": projectID, "statuses": []string{"closed"}}); closed["total"] != 1.0 {
		t.Errorf("closed tasks = %v, want 1", closed["total"])
	}
	if res, _ := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "list_tasks", Arguments: map[string]any{"project_id": projectID, "statuses": []string{"done"}}}); res == nil || !res.IsError {
		t.Errorf("a status outside the list was accepted")
	}
	myTasks := callTool(t, cs, "my_tasks", map[string]any{"state": "closed"})
	if items := myTasks["items"].([]any); len(items) != 1 || items[0].(map[string]any)["project"].(map[string]any)["name"] != "Alfa" {
		t.Errorf("my_tasks closed = %v", myTasks)
	}
	got := callTool(t, cs, "get_task", map[string]any{"task_id": taken})
	if got["name"] != "Da dona" || got["assignee"].(map[string]any)["is_you"] != false {
		t.Errorf("get_task = %v", got)
	}
}

func TestMCP_Session(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "dono@example.com")
	member := invite(t, e, owner, "membro@example.com", "member")
	alfa := createProject(t, e, owner, "Alfa")
	beta := createProject(t, e, owner, "Beta")
	allocate(t, e, owner, alfa, member.id, 5000)
	allocate(t, e, owner, beta, member.id, 5000)
	newTask := func(project, name string) string {
		return decode(t, do(e, "POST", "/api/projects/"+project+"/tasks", `{"name":"`+name+`"}`, owner.session))["id"].(string)
	}
	a, b, c := newTask(alfa, "A"), newTask(alfa, "B"), newTask(alfa, "C")
	other := newTask(beta, "Outra")
	_, token := createToken(t, e, member, "claude")
	cs := connectMCP(t, mcpEndpoint(t, e), token)

	wantCode(t, toolError(t, cs, "clock_out", nil), "work_session.not_open")
	if active := callTool(t, cs, "get_active_session", nil); active["open"] != false || active["session"] != nil {
		t.Errorf("get_active_session with the clock stopped = %v", active)
	}

	opened := callTool(t, cs, "clock_in", map[string]any{"task_id": a})
	if opened["open"] != true || opened["project"].(map[string]any)["name"] != "Alfa" || running(opened) != "A" {
		t.Fatalf("clock_in = %v", opened)
	}
	if task := decode(t, do(e, "GET", "/api/tasks/"+a, "", owner.session)); task["status"] != "in_progress" || task["assignee_id"] != member.id {
		t.Errorf("task after clock_in = %v, want in_progress and the member's", task)
	}
	text := toolError(t, cs, "clock_in", map[string]any{"task_id": b})
	wantCode(t, text, "work_session.already_open")
	if !strings.Contains(text, "add_task_to_session") || !strings.Contains(text, `"Alfa"`) {
		t.Errorf("already open error does not say what to do: %q", text)
	}
	if text := toolError(t, cs, "clock_in", map[string]any{"task_id": other}); !strings.Contains(text, "clock_out first") {
		t.Errorf("already open in another project does not say to clock out: %q", text)
	}

	// Pôr outra tarefa: as duas rodam; de novo, sem trocar, é recusado.
	if added := callTool(t, cs, "add_task_to_session", map[string]any{"task_id": b}); running(added) != "A,B" {
		t.Errorf("add_task_to_session = %v, want A and B running", added["tasks"])
	}
	wantCode(t, toolError(t, cs, "add_task_to_session", map[string]any{"task_id": b}), "mcp.task_already_active")
	// O id em maiúsculas é o mesmo id (o Claude pode mandá-lo assim): a tarefa continua reconhecida na sessão.
	wantCode(t, toolError(t, cs, "add_task_to_session", map[string]any{"task_id": strings.ToUpper(b)}), "mcp.task_already_active")
	wantCode(t, toolError(t, cs, "add_task_to_session", map[string]any{"task_id": other}), "work_session.task_other_project")

	// Trocar: só C roda, e A e B ficam com o tempo deles. Também com o id em maiúsculas.
	switched := callTool(t, cs, "add_task_to_session", map[string]any{"task_id": strings.ToUpper(c), "switch": true})
	if running(switched) != "C" || len(switched["tasks"].([]any)) != 3 {
		t.Errorf("switch = %v, want only C running of three", switched["tasks"])
	}
	stopLast := toolError(t, cs, "stop_session_task", map[string]any{"task_id": strings.ToUpper(c)})
	wantCode(t, stopLast, "work_session.last_task")
	if !strings.Contains(stopLast, "clock_out") {
		t.Errorf("last task error does not point to clock_out: %q", stopLast)
	}
	wantCode(t, toolError(t, cs, "stop_session_task", map[string]any{"task_id": a}), "mcp.task_not_in_session")

	// Voltar para A e tirar B da sessão.
	callTool(t, cs, "add_task_to_session", map[string]any{"task_id": a})
	if stopped := callTool(t, cs, "stop_session_task", map[string]any{"task_id": c}); running(stopped) != "A" {
		t.Errorf("stop_session_task = %v, want only A running", stopped["tasks"])
	}
	removed := callTool(t, cs, "remove_session_task", map[string]any{"task_id": strings.ToUpper(b)})
	for _, st := range removed["tasks"].([]any) {
		if st.(map[string]any)["name"] == "B" {
			t.Errorf("B still in the session: %v", removed["tasks"])
		}
	}
	wantCode(t, toolError(t, cs, "remove_session_task", map[string]any{"task_id": b}), "mcp.task_not_in_session")

	if hours := callTool(t, cs, "my_hours", nil); hours["working_now"] != true || hours["timezone"] != "America/Sao_Paulo" || len(hours["days"].([]any)) != 7 {
		t.Errorf("my_hours = %v", hours)
	}
	if who := callTool(t, cs, "whoami", nil); who["open_session"] == nil {
		t.Errorf("whoami without the open session: %v", who)
	}

	closed := callTool(t, cs, "clock_out", nil)
	if closed["open"] != false || closed["ended_at"] == "" || running(closed) != "" {
		t.Errorf("clock_out = %v", closed)
	}
	if rec := do(e, "GET", "/api/work-sessions/active", "", member.session); strings.TrimSpace(rec.Body.String()) != "null" {
		t.Errorf("the API still sees an open session: %s", rec.Body.String())
	}
}

// Quem perde o acesso ao projeto com o ponto aberto não mexe mais na sessão pelo MCP, como na API: as ferramentas que
// mudam a sessão respondem project.not_found, e só um admin a encerra. Ler a sessão continua valendo. Tirar a pessoa
// do projeto pela tela já encerra a sessão dela (collaborator.Store.Remove), então o teste apaga só a alocação, para
// chegar ao estado que a checagem cobre.
func TestMCP_SessionAfterLeavingTheProject(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "dono@example.com")
	member := invite(t, e, owner, "membro@example.com", "member")
	alfa := createProject(t, e, owner, "Alfa")
	allocate(t, e, owner, alfa, member.id, 5000)
	a := decode(t, do(e, "POST", "/api/projects/"+alfa+"/tasks", `{"name":"A"}`, owner.session))["id"].(string)
	_, token := createToken(t, e, member, "claude")
	cs := connectMCP(t, mcpEndpoint(t, e), token)

	callTool(t, cs, "clock_in", map[string]any{"task_id": a})
	if _, err := testDB.Exec(`DELETE FROM allocations WHERE project_id = $1 AND person_id = $2`, alfa, member.id); err != nil {
		t.Fatalf("delete the allocation: %v", err)
	}
	for _, tool := range []string{"clock_out", "stop_session_task", "remove_session_task", "add_task_to_session"} {
		args := map[string]any{"task_id": a}
		if tool == "clock_out" {
			args = nil
		}
		wantCode(t, toolError(t, cs, tool, args), "project.not_found")
	}
	if active := callTool(t, cs, "get_active_session", nil); active["open"] != true {
		t.Errorf("get_active_session after leaving the project = %v, want the open session", active)
	}
}

// running lista os nomes das tarefas que rodam na sessão, separados por vírgula.
func running(session map[string]any) string {
	var names []string
	for _, st := range session["tasks"].([]any) {
		if m := st.(map[string]any); m["running"] == true {
			names = append(names, m["name"].(string))
		}
	}
	return strings.Join(names, ",")
}

// Cada um só alcança o que alcança pela API: outra organização e um projeto de que a pessoa não é dão o mesmo
// not_found de um id que não existe.
func TestMCP_Isolation(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "dono@example.com")
	member := invite(t, e, owner, "membro@example.com", "member")
	mine := createProject(t, e, owner, "Do membro")
	allocate(t, e, owner, mine, member.id, 5000)
	notMine := createProject(t, e, owner, "Só da dona")
	hidden := decode(t, do(e, "POST", "/api/projects/"+notMine+"/tasks", `{"name":"Escondida"}`, owner.session))["id"].(string)
	stranger := signup(t, e, "Outra", "outra@example.com")
	foreign := createProject(t, e, stranger, "De fora")
	foreignTask := decode(t, do(e, "POST", "/api/projects/"+foreign+"/tasks", `{"name":"De fora"}`, stranger.session))["id"].(string)

	endpoint := mcpEndpoint(t, e)
	_, token := createToken(t, e, member, "claude")
	cs := connectMCP(t, endpoint, token)

	if projects := callTool(t, cs, "list_projects", nil)["projects"].([]any); len(projects) != 1 {
		t.Errorf("list_projects = %v, want only the member's", projects)
	}
	for _, taskID := range []string{hidden, foreignTask, "00000000-0000-0000-0000-000000000000", "nao-e-uuid"} {
		for _, tool := range []string{"get_task", "claim_task", "sync_task", "clock_in"} {
			wantCode(t, toolError(t, cs, tool, map[string]any{"task_id": taskID}), "task.not_found")
		}
		wantCode(t, toolError(t, cs, "update_task", map[string]any{"task_id": taskID, "name": "x"}), "task.not_found")
	}
	for _, projectID := range []string{notMine, foreign} {
		wantCode(t, toolError(t, cs, "list_tasks", map[string]any{"project_id": projectID}), "project.not_found")
		wantCode(t, toolError(t, cs, "create_task", map[string]any{"project_id": projectID, "name": "x"}), "project.not_found")
		wantCode(t, toolError(t, cs, "list_labels", map[string]any{"project_id": projectID}), "project.not_found")
	}
	if n := testClient.Task.Query().CountX(t.Context()); n != 2 {
		t.Errorf("%d tasks, want the two of the setup", n)
	}

	// A dona alcança o projeto em que não está o membro, e não o da outra organização.
	_, ownerToken := createToken(t, e, owner, "claude da dona")
	ocs := connectMCP(t, endpoint, ownerToken)
	if got := callTool(t, ocs, "get_task", map[string]any{"task_id": hidden}); got["name"] != "Escondida" {
		t.Errorf("owner get_task = %v", got)
	}
	wantCode(t, toolError(t, ocs, "get_task", map[string]any{"task_id": foreignTask}), "task.not_found")
}

// Nenhuma saída tem dinheiro, nem para o dono, com cobrança e valor por hora no projeto.
func TestMCP_NoMoney(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "dono@example.com")
	projectID := createProject(t, e, owner, "Alfa")
	setBilling(t, e, owner, "/api/projects/"+projectID, 20000)
	taskID := decode(t, do(e, "POST", "/api/projects/"+projectID+"/tasks", `{"name":"A"}`, owner.session))["id"].(string)
	_, token := createToken(t, e, owner, "claude")
	cs := connectMCP(t, mcpEndpoint(t, e), token)

	outputs := []map[string]any{
		callTool(t, cs, "clock_in", map[string]any{"task_id": taskID}),
		callTool(t, cs, "whoami", nil),
		callTool(t, cs, "get_active_session", nil),
		callTool(t, cs, "list_projects", nil),
		callTool(t, cs, "list_tasks", map[string]any{"project_id": projectID}),
		callTool(t, cs, "get_task", map[string]any{"task_id": taskID}),
		callTool(t, cs, "my_tasks", nil),
		callTool(t, cs, "my_hours", nil),
		callTool(t, cs, "clock_out", nil),
	}
	for _, out := range outputs {
		b, _ := json.Marshal(out)
		for _, word := range []string{"cents", "amount", "rate", "revenue", "margin", "billing"} {
			if strings.Contains(string(b), word) {
				t.Errorf("output has %q: %s", word, b)
			}
		}
	}
}

func TestMCP_SyncTask(t *testing.T) {
	app, fake := syncServer(t)
	e := app.Echo
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	prj := createProject(t, e, admin, "Alfa")
	allocate(t, e, admin, prj, member.id, 5000)
	issue := fake.AddIssue("owner/repo", testutil.GitHubIssue{Title: "Corrigir login"})
	rec := do(e, "POST", "/api/projects/"+prj+"/integrations", `{"type":"github","display_name":"Repo","token":"tok","metadata":{"repo":"owner/repo"}}`, admin.session)
	integ := decode(t, rec)["id"].(string)
	do(e, "PATCH", "/api/integrations/"+integ, `{"sync_issues":true}`, admin.session)
	if rec := do(e, "POST", "/api/integrations/"+integ+"/sync", "", admin.session); rec.Code != http.StatusOK {
		t.Fatalf("first round = %d: %s", rec.Code, rec.Body.String())
	}
	tasks := decodeList(t, do(e, "GET", "/api/projects/"+prj+"/tasks", "", admin.session))
	if len(tasks) != 1 {
		t.Fatalf("%d tasks, want the issue's", len(tasks))
	}
	linked := tasks[0]["id"].(string)
	plain := decode(t, do(e, "POST", "/api/projects/"+prj+"/tasks", `{"name":"Sem issue"}`, admin.session))["id"].(string)
	_, token := createToken(t, e, member, "claude")
	cs := connectMCP(t, mcpEndpoint(t, e), token)

	got := callTool(t, cs, "get_task", map[string]any{"task_id": linked})
	links := got["links"].([]any)
	if len(links) != 1 || links[0].(map[string]any)["platform"] != "github" {
		t.Errorf("links = %v, want the GitHub issue", got["links"])
	}

	fake.EditIssue("owner/repo", issue, func(i *testutil.GitHubIssue) { i.Title = "Corrigir o login" })
	sum := callTool(t, cs, "sync_task", map[string]any{"task_id": linked})
	if sum["updated"] != 1.0 || sum["errors"] != 0.0 || sum["task"].(map[string]any)["name"] != "Corrigir o login" {
		t.Errorf("sync_task = %v, want the new title", sum)
	}
	wantCode(t, toolError(t, cs, "sync_task", map[string]any{"task_id": plain}), "task.no_external_item")
}

func toStrings(v any) []string {
	out := []string{}
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

func saoPaulo(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}
