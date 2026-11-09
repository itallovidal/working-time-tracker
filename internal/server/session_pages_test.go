package server_test

import (
	"net/http"
	"strings"
	"testing"
)

// O modal da sessão mora no modal do layout, dentro do #modal-root e sem teleporte, e só em páginas
// de quem está logado; a pilha das sessões, fora da barra, tem um cartão por sessão aberta, que abre a sessão e para o
// ponto dela, e a linha das sessões de outros projetos.
func TestPages_SessionModalIsInTheLayout(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	projectID := createProject(t, e, admin, "Projeto Alfa")
	const modal = `x-show="$store.modal.name === 'session'"`

	for who, session := range map[string]string{"admin": admin.session, "member": member.session} {
		for _, path := range []string{"/orgs/" + admin.orgID, "/projects/" + projectID + "/overview", "/profile"} {
			body := do(e, "GET", path, "", session).Body.String()
			if n := strings.Count(body, modal); n != 1 {
				t.Errorf("%s %s: the session modal is in the page %d times, want once", who, path, n)
			}
			root, at := strings.Index(body, `id="modal-root"`), strings.Index(body, modal)
			if root < 0 || at < root {
				t.Errorf("%s %s: the session modal is not inside #modal-root", who, path)
			}
			for _, want := range []string{`class="session-dock"`, `x-for="s in cards()"`, `@click="details(s)"`, `@click="stop(s)"`, `class="session-dock-others"`} {
				if !strings.Contains(body, want) {
					t.Errorf("%s %s: the active session dock does not contain %q", who, path, want)
				}
			}
			if dock, topbar := strings.Index(body, `class="session-dock"`), strings.Index(body, "</header>"); dock < topbar {
				t.Errorf("%s %s: the session dock is inside the top bar", who, path)
			}
			if strings.Contains(body, "session.task.name") {
				t.Errorf("%s %s: the page still reads the old session.task", who, path)
			}
		}
	}

	// Sem login não há sessão para mostrar.
	if rec := do(e, "GET", "/login", "", ""); rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), modal) {
		t.Errorf("GET /login = %d with the session modal %v, want 200 without it", rec.Code, strings.Contains(rec.Body.String(), modal))
	}
}

// As telas de sessões mostram as tarefas de cada sessão, e a linha abre o modal. O Início, a
// Gestão e a página da tarefa põem a tarefa na sessão aberta do projeto em vez de iniciar outra; a sessão de outro
// projeto não bloqueia nada (Decisão 26), então não há mais otherProject().
func TestPages_SessionTasksOnTheScreens(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	projectID := createProject(t, e, admin, "Projeto Alfa")
	allocate(t, e, admin, projectID, admin.id, 0)
	rec := do(e, "POST", "/api/projects/"+projectID+"/tasks", `{"name":"Tela de login","assignee_id":"`+admin.id+`"}`, admin.session)
	taskID := decode(t, rec)["id"].(string)

	page := func(path string) string {
		t.Helper()
		rec := do(e, "GET", path, "", admin.session)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200", path, rec.Code)
		}
		return rec.Body.String()
	}
	check := func(path string, want, gone []string) {
		t.Helper()
		body := page(path)
		for _, w := range want {
			if !strings.Contains(body, w) {
				t.Errorf("%s does not contain %q", path, w)
			}
		}
		for _, g := range gone {
			if strings.Contains(body, g) {
				t.Errorf("%s still contains %q", path, g)
			}
		}
	}

	oldShape := []string{"s.task_id", "s.task.name", "session.task."}
	check("/projects/"+projectID+"/overview",
		[]string{"<th>Tarefas</th>", `x-for="t in firstTasks(s)"`, "moreTasks(s)", `@click="openSession(s)"`, "seconds(s)", `@click="openSession()"`, "Tarefas da sessão", "sessionHere()", "$t('session.add_to_session')"},
		append(oldShape, "<th>Tarefa</th>"))
	check("/projects/"+projectID+"/management/overview",
		[]string{"<th>Tarefas</th>", `x-for="t in firstTasks(s)"`, `@click="openSession(s)"`, "seconds(s)"},
		append(oldShape, "<th>Tarefa</th>"))
	check("/projects/"+projectID+"/my-tasks",
		[]string{"sessionHere()", "here()", "$t('session.add_to_session')", `@click="startTask(t)"`},
		append(oldShape, "otherProject()", "$store.clock.session)", "$store.clock.session.", "$store.clock.session &&"))
	check("/tasks/"+taskID,
		[]string{`@click="addToSession()"`, `@click="openSession()"`, `@click="openSession(s)"`, "taskTime(s)", "otherTasks(s)", "sessionHere()", "Adicionar à sessão"},
		append(oldShape, "otherProject()"))
}
