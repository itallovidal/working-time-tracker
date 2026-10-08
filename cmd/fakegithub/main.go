// Command fakegithub é o GitHub falso dos testes (testutil.GitHub) como servidor de desenvolvimento,
// para ver a conexão e a sincronização das issues numa instância do sistema sem tocar no GitHub de
// verdade. Aponte o servidor para ele com GITHUB_URL e GITHUB_API_URL, e use GITHUB_CLIENT_ID e
// GITHUB_CLIENT_SECRET iguais aos de testutil. Atende o site (a tela de autorização que já volta
// autorizada) e a API no mesmo endereço, e tem um painel de controle em /_fake para mexer nas issues
// como se fosse outra pessoa no site do GitHub.
//
//	go run ./cmd/fakegithub -addr :8091
//
// Painel (JSON):
//
//	GET  /_fake/issues                  as issues de owner/repo
//	POST /_fake/issues                  {"title","body","labels","assignees","state"}: cria uma issue
//	POST /_fake/issues/{n}              {"title","body","labels","assignees","state"}: edita; só o que vier
//	POST /_fake/users                   {"login","email"}: cadastra um usuário
//	POST /_fake/push                    {"push":false}: tira ou devolve a permissão de escrita
//	POST /_fake/admin                   {"admin":false}: tira ou devolve o ser admin do repositório (sem isso,
//	                                    a issue de uma tarefa excluída só é fechada, e não apagada)
//	GET  /_fake/requests                o que o servidor pediu
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strconv"

	"working-time-tracker/testutil"
)

const repo = "owner/repo"

func main() {
	addr := flag.String("addr", ":8091", "endereço do servidor")
	flag.Parse()

	fake := testutil.NewGitHub()
	seed(fake)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /_fake/issues", func(w http.ResponseWriter, r *http.Request) {
		var out []testutil.GitHubIssue
		for n := 1; n < 500; n++ {
			if i, ok := fake.Issue(repo, n); ok {
				out = append(out, i)
			}
		}
		write(w, out)
	})
	mux.HandleFunc("POST /_fake/issues", func(w http.ResponseWriter, r *http.Request) {
		var in patch
		json.NewDecoder(r.Body).Decode(&in)
		i := testutil.GitHubIssue{}
		in.apply(&i)
		write(w, map[string]int{"number": fake.AddIssue(repo, i)})
	})
	mux.HandleFunc("POST /_fake/issues/{n}", func(w http.ResponseWriter, r *http.Request) {
		n, err := strconv.Atoi(r.PathValue("n"))
		if _, ok := fake.Issue(repo, n); err != nil || !ok {
			http.Error(w, "no such issue", http.StatusNotFound)
			return
		}
		var in patch
		json.NewDecoder(r.Body).Decode(&in)
		fake.EditIssue(repo, n, func(i *testutil.GitHubIssue) { in.apply(i) })
		write(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /_fake/users", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Login, Email string }
		json.NewDecoder(r.Body).Decode(&in)
		fake.AddUser(in.Login, in.Email)
		write(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /_fake/push", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Push bool }
		json.NewDecoder(r.Body).Decode(&in)
		fake.SetPush(repo, in.Push)
		write(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /_fake/admin", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Admin bool }
		json.NewDecoder(r.Body).Decode(&in)
		fake.SetAdmin(repo, in.Admin)
		write(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /_fake/requests", func(w http.ResponseWriter, r *http.Request) { write(w, fake.Requests()) })
	mux.Handle("/", fake)

	log.Printf("fake GitHub on %s (repository %s, OAuth app %s)", *addr, repo, testutil.GitHubClientID)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

// patch é o que o painel aceita numa issue; o que não vem fica como está.
type patch struct {
	Title     *string   `json:"title"`
	Body      *string   `json:"body"`
	State     *string   `json:"state"`
	Labels    *[]string `json:"labels"`
	Assignees *[]string `json:"assignees"`
}

func (p patch) apply(i *testutil.GitHubIssue) {
	if p.Title != nil {
		i.Title = *p.Title
	}
	if p.Body != nil {
		i.Body = *p.Body
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

func write(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// seed deixa o repositório com issues de vários jeitos: com e sem responsável, com Markdown, com
// responsável que não publica e-mail, fechada e um pull request. Os usuários são as pessoas do seed do
// sistema (cmd/seed), com o e-mail público.
func seed(fake *testutil.GitHub) {
	fake.AddRepo(repo)
	for login, email := range map[string]string{
		"ana-souza": "ana@example.com", "bruno-lima": "bruno@example.com", "carla-mendes": "carla@example.com",
		"helena-costa": "helena@example.com", "sem-email": "", "visitante": "visitante@fora.com",
	} {
		fake.AddUser(login, email)
	}
	add := func(i testutil.GitHubIssue) { fake.AddIssue(repo, i) }
	add(testutil.GitHubIssue{Title: "Corrigir o login com Google", Body: "## O que acontece\r\n\r\nO botão **Entrar com Google** volta para a tela inicial.\r\n\r\n- reproduzir no Chrome\r\n- conferir o redirect", Labels: []string{"bug", "auth"}, Assignees: []string{"ana-souza"}})
	add(testutil.GitHubIssue{Title: "Relatório mensal em PDF", Body: "Exportar o relatório do mês em PDF.", Labels: []string{"feature"}})
	add(testutil.GitHubIssue{Title: "Revisar textos do onboarding", Labels: []string{"docs", "ux"}, Assignees: []string{"bruno-lima"}})
	add(testutil.GitHubIssue{Title: "Cache da lista de projetos", Body: "A lista demora com muitos projetos.", Labels: []string{"performance"}, Assignees: []string{"sem-email"}})
	add(testutil.GitHubIssue{Title: "Convite de pessoa de fora", Assignees: []string{"visitante"}})
	add(testutil.GitHubIssue{Title: "Migrar para o novo provedor de e-mail", State: "closed", Labels: []string{"infra"}})
	add(testutil.GitHubIssue{Title: "Atualizar dependências", PullRequest: true, Labels: []string{"deps"}})
}
