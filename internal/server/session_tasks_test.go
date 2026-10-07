package server_test

import (
	"net/http"
	"strings"
	"testing"
)

// As tarefas da sessão pela API: quem bateu o ponto e os admins mexem, os outros não; os
// valores de cada tarefa seguem as regras dos valores da sessão; os erros voltam com o código.
func TestWorkSessions_TasksOfTheSession(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	bia := invite(t, e, admin, "bia@test.com", "member")
	caio := invite(t, e, admin, "caio@test.com", "member")
	projectID := createProject(t, e, admin, "Projeto")
	prj := "/api/projects/" + projectID
	allocate(t, e, admin, projectID, bia.id, 2000)
	allocate(t, e, admin, projectID, caio.id, 2500)
	if rec := do(e, "PUT", prj+"/billing", `{"bill_rate_cents":10000}`, admin.session); rec.Code != http.StatusOK {
		t.Fatalf("set billing = %d: %s", rec.Code, rec.Body.String())
	}
	taskA := decode(t, do(e, "POST", prj+"/tasks", `{"name":"A"}`, admin.session))["id"].(string)
	taskB := decode(t, do(e, "POST", prj+"/tasks", `{"name":"B"}`, admin.session))["id"].(string)

	opened := decode(t, do(e, "POST", prj+"/work-sessions/clock-in", `{"task_id":"`+taskA+`"}`, bia.session))
	sid := opened["id"].(string)
	tasksPath := prj + "/work-sessions/" + sid + "/tasks"

	// Quem bateu o ponto adiciona; outra pessoa do projeto, não.
	rec := do(e, "POST", tasksPath, `{"task_id":"`+taskB+`"}`, caio.session)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"code":"work_session.not_yours"`) {
		t.Errorf("another member adding a task = %d %s, want 403 not_yours", rec.Code, rec.Body.String())
	}
	rec = do(e, "POST", tasksPath, `{"task_id":"`+taskB+`"}`, bia.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("owner adding a task = %d: %s", rec.Code, rec.Body.String())
	}
	session := decode(t, rec)
	links, _ := session["tasks"].([]any)
	if len(links) != 2 {
		t.Fatalf("tasks after adding = %v, want A and B", session["tasks"])
	}
	linkB := links[1].(map[string]any)

	// O valor de cada tarefa segue a regra da sessão: quem bateu o ponto vê o que ganhou e não o
	// valor cobrado; o admin vê os dois.
	for _, l := range links {
		link := l.(map[string]any)
		if link["pay_amount_cents"] == nil || link["bill_amount_cents"] != nil {
			t.Errorf("member sees the task amounts %v, want what she earns and no bill amount", link)
		}
	}
	adminView := decodeList(t, do(e, "GET", prj+"/work-sessions?task_id="+taskB, "", admin.session))
	if len(adminView) != 1 {
		t.Fatalf("sessions with B = %d, want 1", len(adminView))
	}
	for _, l := range adminView[0]["tasks"].([]any) {
		if link := l.(map[string]any); link["pay_amount_cents"] == nil || link["bill_amount_cents"] == nil {
			t.Errorf("admin does not see both amounts of a task: %v", link)
		}
	}

	// Um admin mexe na sessão de outra pessoa. Parar a B deixa a A em andamento.
	linkPath := tasksPath + "/" + linkB["id"].(string)
	if rec := do(e, "PATCH", linkPath, `{"stop":true}`, caio.session); rec.Code != http.StatusForbidden {
		t.Errorf("another member stopping a task = %d, want 403", rec.Code)
	}
	if rec := do(e, "PATCH", linkPath, `{"stop":true}`, admin.session); rec.Code != http.StatusOK {
		t.Fatalf("admin stopping a task = %d: %s", rec.Code, rec.Body.String())
	}
	// A A é a única em andamento, e a sessão guarda uma tarefa: nada de tirar a última.
	linkA := links[0].(map[string]any)["id"].(string)
	rec = do(e, "PATCH", tasksPath+"/"+linkA, `{"stop":true}`, bia.session)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"code":"work_session.last_task"`) {
		t.Errorf("stopping the last task in progress = %d %s, want 400 last_task", rec.Code, rec.Body.String())
	}
	if rec := do(e, "DELETE", linkPath, "", bia.session); rec.Code != http.StatusOK || len(decode(t, rec)["tasks"].([]any)) != 1 {
		t.Errorf("owner removing B = %d %s, want 200 with A left", rec.Code, rec.Body.String())
	}
	rec = do(e, "DELETE", tasksPath+"/"+linkA, "", bia.session)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"code":"work_session.last_task"`) {
		t.Errorf("removing the only task = %d %s, want 400 last_task", rec.Code, rec.Body.String())
	}

	// Com a sessão encerrada o intervalo se corrige, e o fim nulo é "até o fim da sessão".
	do(e, "POST", prj+"/work-sessions/clock-out", `{}`, bia.session)
	rec = do(e, "POST", tasksPath, `{"task_id":"`+taskB+`","until_at":null}`, bia.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("adding B to the closed session = %d: %s", rec.Code, rec.Body.String())
	}
	linkB2 := decode(t, rec)["tasks"].([]any)[1].(map[string]any)
	path2 := tasksPath + "/" + linkB2["id"].(string)
	rec = do(e, "PATCH", path2, `{"from_at":"2000-01-01T00:00:00Z"}`, bia.session)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"code":"work_session.invalid_interval"`) {
		t.Errorf("an interval before the session = %d %s, want 400 invalid_interval", rec.Code, rec.Body.String())
	}
	if rec := do(e, "PATCH", path2, `{"until_at":null}`, bia.session); rec.Code != http.StatusOK {
		t.Errorf("clearing the end of an interval = %d: %s", rec.Code, rec.Body.String())
	}

	// Sessão ou intervalo que não existem, e a sessão por outro projeto.
	if rec := do(e, "POST", prj+"/work-sessions/00000000-0000-0000-0000-000000000000/tasks", `{"task_id":"`+taskB+`"}`, bia.session); rec.Code != http.StatusNotFound {
		t.Errorf("unknown session = %d, want 404", rec.Code)
	}
	if rec := do(e, "DELETE", tasksPath+"/00000000-0000-0000-0000-000000000000", "", bia.session); rec.Code != http.StatusNotFound {
		t.Errorf("unknown interval = %d, want 404", rec.Code)
	}
	other := createProject(t, e, admin, "Outro")
	if rec := do(e, "POST", "/api/projects/"+other+"/work-sessions/"+sid+"/tasks", `{"task_id":"`+taskB+`"}`, admin.session); rec.Code != http.StatusNotFound {
		t.Errorf("session through another project = %d, want 404", rec.Code)
	}
	if rec := do(e, "POST", tasksPath, `{}`, bia.session); rec.Code != http.StatusBadRequest {
		t.Errorf("adding without a task = %d, want 400", rec.Code)
	}
}
