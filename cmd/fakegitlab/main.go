// Command fakegitlab é o GitLab falso dos testes (testutil.GitLab) como servidor de desenvolvimento, para ver
// a conexão e a sincronização das issues numa instância do sistema sem tocar no gitlab.com. Aponte o servidor
// para ele com GITLAB_URL (o endereço do fake) e GITLAB_API_URL (o endereço com /api/v4), e use
// GITLAB_CLIENT_ID e GITLAB_CLIENT_SECRET iguais aos de testutil. Atende o OAuth (a tela de autorização que
// já volta autorizada, e a troca e a renovação do token) e a API no mesmo endereço, e tem um painel de
// controle em /_fake para mexer nas issues como se fosse outra pessoa no site do GitLab.
//
//	go run ./cmd/fakegitlab -addr :8093
//
// Painel (JSON):
//
//	GET  /_fake/issues                  as issues de group/project
//	POST /_fake/issues                  {"title","description","labels","assignees","state"}: cria uma issue
//	POST /_fake/issues/{iid}            {"title","description","labels","assignees","state"}: edita; só o que vier
//	POST /_fake/users                   {"username","email"}: cadastra um usuário
//	POST /_fake/access                  {"level":30}: muda o nível de acesso da conta no projeto (10 Guest, 15
//	                                    Planner, 20 Reporter, 30 Developer, 40 Maintainer, 50 Owner); só o
//	                                    Owner apaga a issue de uma tarefa excluída, os outros a fecham
//	POST /_fake/lifetime                {"seconds":60}: quanto vale o token de acesso emitido daqui em diante
//	POST /_fake/revoke                  revoga os tokens do OAuth, como quem tira a autorização do app
//	GET  /_fake/requests                o que o servidor pediu
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strconv"
	"time"

	"working-time-tracker/testutil"
)

const project = "group/project"

func main() {
	addr := flag.String("addr", ":8093", "endereço do servidor")
	flag.Parse()

	fake := testutil.NewGitLab()
	seed(fake)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /_fake/issues", func(w http.ResponseWriter, r *http.Request) {
		var out []testutil.GitLabIssue
		for n := 1; n < 500; n++ {
			if i, ok := fake.Issue(project, n); ok {
				out = append(out, i)
			}
		}
		write(w, out)
	})
	mux.HandleFunc("POST /_fake/issues", func(w http.ResponseWriter, r *http.Request) {
		var in patch
		read(r, &in)
		i := testutil.GitLabIssue{}
		in.apply(&i)
		write(w, map[string]int{"iid": fake.AddIssue(project, i)})
	})
	mux.HandleFunc("POST /_fake/issues/{iid}", func(w http.ResponseWriter, r *http.Request) {
		n, err := strconv.Atoi(r.PathValue("iid"))
		if _, ok := fake.Issue(project, n); err != nil || !ok {
			http.Error(w, "no such issue", http.StatusNotFound)
			return
		}
		var in patch
		read(r, &in)
		fake.EditIssue(project, n, func(i *testutil.GitLabIssue) { in.apply(i) })
		write(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /_fake/users", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Username, Email string }
		read(r, &in)
		fake.AddUser(in.Username, in.Email)
		write(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /_fake/access", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Level int }
		read(r, &in)
		fake.SetAccess(project, in.Level)
		write(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /_fake/lifetime", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Seconds int }
		read(r, &in)
		fake.TokenLifetime(time.Duration(max(in.Seconds, 1)) * time.Second)
		write(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /_fake/revoke", func(w http.ResponseWriter, r *http.Request) {
		fake.RevokeTokens()
		write(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /_fake/requests", func(w http.ResponseWriter, r *http.Request) { write(w, fake.Requests()) })
	mux.Handle("/", fake)

	log.Printf("fake GitLab on %s (project %s, OAuth app %s, API at /api/v4)", *addr, project, testutil.GitLabClientID)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

// patch é o que o painel aceita numa issue; o que não vem fica como está.
type patch struct {
	Title       *string   `json:"title"`
	Description *string   `json:"description"`
	State       *string   `json:"state"`
	Labels      *[]string `json:"labels"`
	Assignees   *[]string `json:"assignees"`
}

func (p patch) apply(i *testutil.GitLabIssue) {
	if p.Title != nil {
		i.Title = *p.Title
	}
	if p.Description != nil {
		i.Description = *p.Description
	}
	if p.State != nil {
		i.State = *p.State
	}
	if p.Labels != nil {
		i.Labels = *p.Labels
	}
	if p.Assignees != nil {
		i.Assignees = *p.Assignees
	}
}

// read lê o corpo de um pedido do painel. O corpo vazio, ou que não é JSON, deixa os campos com o valor
// zero: a rota segue com eles, e o erro fica de fora.
func read(r *http.Request, into any) { _ = json.NewDecoder(r.Body).Decode(into) }

func write(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writing response: %v", err)
	}
}

// seed deixa o projeto com issues de vários jeitos: com e sem responsável, com Markdown, com responsável que
// não publica e-mail, fechada e um incidente (que não é issue e fica de fora). Os usuários são as pessoas do
// seed do sistema (cmd/seed), com o e-mail público.
func seed(fake *testutil.GitLab) {
	fake.AddProject(project)
	for username, email := range map[string]string{
		"ana-souza": "ana@example.com", "bruno-lima": "bruno@example.com", "carla-mendes": "carla@example.com",
		"helena-costa": "helena@example.com", "sem-email": "", "visitante": "visitante@fora.com",
	} {
		fake.AddUser(username, email)
	}
	add := func(i testutil.GitLabIssue) { fake.AddIssue(project, i) }
	add(testutil.GitLabIssue{Title: "Ajustar o relatório de horas", Description: "## O que acontece\r\n\r\nO total da semana **não soma** o sábado.\r\n\r\n- conferir o fuso\r\n- conferir o arredondamento", Labels: []string{"bug", "relatorio"}, Assignees: []string{"ana-souza"}})
	add(testutil.GitLabIssue{Title: "Exportar as horas em CSV", Description: "Uma coluna por dia.", Labels: []string{"feature"}})
	add(testutil.GitLabIssue{Title: "Revisar a tela de projetos no celular", Labels: []string{"ux"}, Assignees: []string{"bruno-lima"}})
	add(testutil.GitLabIssue{Title: "Índice na tabela de sessões", Description: "A consulta do mês está lenta.", Labels: []string{"performance"}, Assignees: []string{"sem-email"}})
	add(testutil.GitLabIssue{Title: "Acesso de quem é de fora", Assignees: []string{"visitante"}})
	add(testutil.GitLabIssue{Title: "Trocar o provedor de e-mail", State: "closed", Labels: []string{"infra"}})
	add(testutil.GitLabIssue{Title: "Fora do ar no domingo", Type: "incident", Labels: []string{"incidente"}})
}
