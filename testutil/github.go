package testutil

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// GitHubIssue é uma issue (ou um pull request) do GitHub fake. Os campos são os que a sincronização
// lê e escreve; o resto da issue de verdade não existe aqui.
type GitHubIssue struct {
	Number      int
	Title       string
	Body        string
	State       string // "open" ou "closed"
	StateReason string // "completed", "not_planned", "reopened" ou vazio
	Labels      []string
	Assignees   []string
	// PullRequest marca um pull request: a lista de issues do GitHub o traz junto com as issues.
	PullRequest bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	// Deleted só vem preenchido por Issue: a issue foi apagada (pelo GraphQL ou por DeleteIssue), e o GitHub
	// responde 410 por ela.
	Deleted bool
}

// GitHubRequest é uma requisição que o GitHub fake recebeu, para o teste contar o que o sistema
// pediu (a prova de que uma rodada sem diferença não escreve nada).
type GitHubRequest struct {
	Method string
	Path   string
	Query  string
	Body   string
}

// GitHub é um GitHub falso com estado: repositórios com issues, etiquetas e permissões, usuários com
// e-mail público ou não, a busca de usuário por e-mail e a troca de código por token do OAuth. Fala
// o mesmo que a API de verdade nos pontos que o sistema usa e tem chaves para os defeitos que a
// sincronização precisa aguentar: PATCH que descarta campos sem avisar, falta de permissão, issue
// apagada ou transferida, limite de requisições. Só atende em loopback.
//
// Conhece os repositórios owner/repo (com a issue 42) e owner/other, e o login octocat.
type GitHub struct {
	mu       sync.Mutex
	repos    map[string]*ghRepo
	users    map[string]string // login → e-mail público ("" é sem e-mail público)
	nologin  map[string]bool   // logins que existem mas o GitHub não aceita como responsável
	requests []GitHubRequest
	faults   []*ghFault
	handler  *http.ServeMux
	last     time.Time // o último updated_at dado, para ele só crescer
	limited  time.Time // até quando todas as requisições são recusadas por limite
	remains  int       // o X-RateLimit-Remaining informado; negativo é o padrão
}

type ghRepo struct {
	name       string
	push       bool
	admin      bool // a conta é admin do repositório: só ela apaga uma issue (GraphQL deleteIssue)
	archived   bool
	issues     []*GitHubIssue
	next       int
	labels     map[string]string // minúsculas → nome como foi criado
	discard    map[string]bool   // campos que o PATCH descarta sem avisar: labels, assignees, title, body, state
	moved      map[int]bool      // issues transferidas: 301
	gone       map[int]bool      // issues apagadas: 410
	labelError int               // status que a criação de etiqueta devolve, se não for zero
	noIssues   bool              // o repositório desligou as issues: criar uma devolve 410
}

type ghFault struct {
	method, prefix string
	status         int
	times          int
}

// NewGitHub devolve o GitHub fake com os repositórios e a issue 42 de sempre.
func NewGitHub() *GitHub {
	g := &GitHub{
		repos:   map[string]*ghRepo{},
		users:   map[string]string{GitHubLogin: ""},
		nologin: map[string]bool{},
		remains: -1,
	}
	g.handler = g.routes()
	g.AddRepo("owner/repo")
	g.AddRepo("owner/other")
	g.AddIssue("owner/repo", GitHubIssue{Number: 42, Title: "Corrigir login", State: "open"})
	return g
}

// FakeGitHub é o GitHub fake como http.Handler, para quem só precisa do servidor.
func FakeGitHub() http.Handler { return NewGitHub() }

// bump devolve um instante maior que todos os anteriores: assim o updated_at de duas escritas
// seguidas nunca empata, e a ordem por updated_at é a ordem das escritas.
func (g *GitHub) bump() time.Time {
	now := time.Now().UTC().Truncate(time.Second)
	if !now.After(g.last) {
		now = g.last.Add(time.Second)
	}
	g.last = now
	return now
}

// AddRepo cria um repositório onde o token tem permissão de escrita.
func (g *GitHub) AddRepo(name string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.repos[name] = &ghRepo{
		name: name, push: true, admin: true, next: 1,
		labels:  map[string]string{},
		discard: map[string]bool{},
		moved:   map[int]bool{},
		gone:    map[int]bool{},
	}
}

func (g *GitHub) repo(name string) *ghRepo {
	r := g.repos[name]
	if r == nil {
		panic("testutil: unknown fake GitHub repository " + name)
	}
	return r
}

// AddUser cadastra um usuário. email é o e-mail público do perfil; vazio é o usuário que não
// publica e-mail (o caso comum), que o sistema não consegue ligar a uma pessoa.
func (g *GitHub) AddUser(login, email string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.users[login] = email
}

// Unassignable faz o GitHub descartar o login quando ele é posto como responsável de uma issue,
// como acontece com quem não tem acesso ao repositório.
func (g *GitHub) Unassignable(login string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nologin[login] = true
}

// DisableIssues desliga as issues do repositório: criar uma passa a devolver 410, como o GitHub.
func (g *GitHub) DisableIssues(repo string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.repo(repo).noIssues = true
}

// AddIssue cria uma issue. Number zero pega o próximo número do repositório; o estado vazio é
// aberta. Devolve o número.
func (g *GitHub) AddIssue(repo string, in GitHubIssue) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	r := g.repo(repo)
	if in.Number == 0 {
		in.Number = r.next
	}
	if in.Number >= r.next {
		r.next = in.Number + 1
	}
	if in.State == "" {
		in.State = "open"
	}
	in.CreatedAt = g.bump()
	in.UpdatedAt = in.CreatedAt
	in.Labels = append([]string(nil), in.Labels...)
	in.Assignees = append([]string(nil), in.Assignees...)
	for _, l := range in.Labels {
		r.labels[strings.ToLower(l)] = l
	}
	r.issues = append(r.issues, &in)
	return in.Number
}

// Issue devolve uma cópia da issue como está no fake.
func (g *GitHub) Issue(repo string, number int) (GitHubIssue, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	r := g.repo(repo)
	if i := g.find(r, number); i != nil {
		out := *i
		out.Labels = append([]string(nil), i.Labels...)
		out.Assignees = append([]string(nil), i.Assignees...)
		out.Deleted = r.gone[number]
		return out, true
	}
	return GitHubIssue{}, false
}

func (g *GitHub) find(r *ghRepo, number int) *GitHubIssue {
	for _, i := range r.issues {
		if i.Number == number {
			return i
		}
	}
	return nil
}

// EditIssue é alguém mexendo na issue pelo site do GitHub: aplica a função e atualiza o updated_at.
func (g *GitHub) EditIssue(repo string, number int, edit func(*GitHubIssue)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	r := g.repo(repo)
	i := g.find(r, number)
	if i == nil {
		panic(fmt.Sprintf("testutil: fake GitHub %s has no issue %d", repo, number))
	}
	edit(i)
	for _, l := range i.Labels {
		r.labels[strings.ToLower(l)] = l
	}
	i.UpdatedAt = g.bump()
}

// DeleteIssue apaga a issue: o GitHub passa a responder 410.
func (g *GitHub) DeleteIssue(repo string, number int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.repo(repo).gone[number] = true
}

// MoveIssue transfere a issue para outro repositório: o endereço antigo passa a responder 301.
func (g *GitHub) MoveIssue(repo string, number int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.repo(repo).moved[number] = true
}

// SetPush diz se o token escreve no repositório (permissions.push). Sem escrita, o PATCH é recusado.
func (g *GitHub) SetPush(repo string, push bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.repo(repo).push = push
}

// SetAdmin diz se a conta é admin do repositório. Sem isso o GraphQL recusa deleteIssue (FORBIDDEN), como o GitHub
// faz, e a issue só pode ser fechada. O padrão de um repositório novo é ser admin.
func (g *GitHub) SetAdmin(repo string, admin bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.repo(repo).admin = admin
}

// SetArchived arquiva o repositório: ele continua legível e deixa de aceitar escrita.
func (g *GitHub) SetArchived(repo string, archived bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.repo(repo).archived = archived
}

// Discard faz o PATCH descartar estes campos sem avisar (labels, assignees, title, body, state):
// responde 200 com a issue como estava, que é o que o GitHub faz com quem não tem permissão.
func (g *GitHub) Discard(repo string, fields ...string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	r := g.repo(repo)
	r.discard = map[string]bool{}
	for _, f := range fields {
		r.discard[f] = true
	}
}

// LabelCreateStatus faz a criação de etiqueta devolver este status (403, por exemplo). Zero volta ao normal.
func (g *GitHub) LabelCreateStatus(repo string, status int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.repo(repo).labelError = status
}

// LabelNames lista as etiquetas que o repositório tem, em ordem.
func (g *GitHub) LabelNames(repo string) []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	var out []string
	for _, n := range g.repo(repo).labels {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Fail faz as próximas `times` requisições que casam com o método e o prefixo do caminho
// responderem com este status, sem tocar no estado.
func (g *GitHub) Fail(method, pathPrefix string, status, times int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.faults = append(g.faults, &ghFault{method: method, prefix: pathPrefix, status: status, times: times})
}

// RateLimitedFor recusa toda requisição, por limite, durante este tempo.
func (g *GitHub) RateLimitedFor(d time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.limited = time.Now().Add(d)
}

// Remaining muda o X-RateLimit-Remaining que as respostas informam. Negativo volta ao padrão.
func (g *GitHub) Remaining(n int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.remains = n
}

// Requests devolve o que o fake recebeu, na ordem. Reset zera o registro.
func (g *GitHub) Requests() []GitHubRequest {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]GitHubRequest(nil), g.requests...)
}

// Count conta as requisições do método cujo caminho começa com o prefixo.
func (g *GitHub) Count(method, pathPrefix string) int {
	n := 0
	for _, r := range g.Requests() {
		if r.Method == method && strings.HasPrefix(r.Path, pathPrefix) {
			n++
		}
	}
	return n
}

// Writes conta as requisições que escrevem (tudo que não é GET).
func (g *GitHub) Writes() int {
	n := 0
	for _, r := range g.Requests() {
		if r.Method != http.MethodGet {
			n++
		}
	}
	return n
}

// Reset apaga o registro de requisições.
func (g *GitHub) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.requests = nil
}

// ServeHTTP atende a API e o site (o OAuth) no mesmo endereço.
func (g *GitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	r.Body = io.NopCloser(strings.NewReader(string(body)))

	g.mu.Lock()
	g.requests = append(g.requests, GitHubRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Body: string(body)})
	remains := g.remains
	limited := g.limited
	var fault *ghFault
	for _, f := range g.faults {
		if f.times > 0 && f.method == r.Method && strings.HasPrefix(r.URL.Path, f.prefix) {
			f.times--
			fault = f
			break
		}
	}
	g.mu.Unlock()

	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	if remains >= 0 {
		h.Set("X-RateLimit-Remaining", strconv.Itoa(remains))
		h.Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10))
	}
	if r.URL.Path != "/login/oauth/access_token" && r.URL.Path != "/login/oauth/authorize" && time.Now().Before(limited) {
		h.Set("X-RateLimit-Remaining", "0")
		h.Set("X-RateLimit-Reset", strconv.FormatInt(limited.Unix(), 10))
		http.Error(w, `{"message":"API rate limit exceeded"}`, http.StatusForbidden)
		return
	}
	if fault != nil {
		http.Error(w, `{"message":"injected failure"}`, fault.status)
		return
	}
	g.handler.ServeHTTP(w, r)
}

func (g *GitHub) routes() *http.ServeMux {
	mux := http.NewServeMux()
	auth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") == "Bearer "+InvalidToken {
				http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
				return
			}
			next(w, r)
		}
	}

	mux.HandleFunc("GET /repos/{owner}/{repo}", auth(g.getRepo))
	mux.HandleFunc("GET /repos/{owner}/{repo}/issues", auth(g.listIssues))
	mux.HandleFunc("GET /repos/{owner}/{repo}/issues/{number}", auth(g.getIssue))
	mux.HandleFunc("POST /repos/{owner}/{repo}/issues", auth(g.createIssue))
	mux.HandleFunc("PATCH /repos/{owner}/{repo}/issues/{number}", auth(g.patchIssue))
	mux.HandleFunc("POST /graphql", auth(g.graphql))
	mux.HandleFunc("GET /repos/{owner}/{repo}/labels", auth(g.listLabels))
	mux.HandleFunc("POST /repos/{owner}/{repo}/labels", auth(g.createLabel))
	mux.HandleFunc("GET /users/{login}", auth(g.getUser))
	mux.HandleFunc("GET /search/users", auth(g.searchUsers))

	// O OAuth: o GitHub responde 200 também quando recusa, com o motivo em error.
	mux.HandleFunc("GET /login/oauth/authorize", g.authorize)
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
			Code         string `json:"code"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		switch {
		case body.ClientID != GitHubClientID || body.ClientSecret != GitHubClientSecret:
			w.Write([]byte(`{"error":"incorrect_client_credentials"}`))
		case body.Code == GitHubOAuthCode:
			fmt.Fprintf(w, `{"access_token":%q,"token_type":"bearer","scope":"repo"}`, GitHubOAuthToken)
		case body.Code == GitHubSecondOAuthCode:
			fmt.Fprintf(w, `{"access_token":%q,"token_type":"bearer","scope":"repo"}`, GitHubSecondOAuthToken)
		default:
			w.Write([]byte(`{"error":"bad_verification_code"}`))
		}
	})
	mux.HandleFunc("GET /user", auth(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"login":%q}`, GitHubLogin)
	}))
	mux.HandleFunc("GET /user/repos", auth(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer "+GitHubSecondOAuthToken {
			w.Write([]byte(`[{"full_name":"owner/second","private":false}]`))
			return
		}
		w.Write([]byte(`[{"full_name":"owner/repo","private":true},{"full_name":"owner/other","private":false}]`))
	}))
	return mux
}

// authorize é a tela de autorização do GitHub com o "Autorizar" já clicado: devolve a pessoa ao
// endereço de retorno com o código. O navegador de verdade passa por aqui; os testes só olham a URL.
func (g *GitHub) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("client_id") != GitHubClientID {
		http.Error(w, "unknown client_id", http.StatusNotFound)
		return
	}
	back, err := url.Parse(q.Get("redirect_uri"))
	if err != nil || back.Scheme == "" {
		http.Error(w, "invalid redirect_uri", http.StatusBadRequest)
		return
	}
	v := back.Query()
	v.Set("code", GitHubOAuthCode)
	v.Set("state", q.Get("state"))
	back.RawQuery = v.Encode()
	http.Redirect(w, r, back.String(), http.StatusFound)
}

func (g *GitHub) lookup(w http.ResponseWriter, r *http.Request) *ghRepo {
	name := r.PathValue("owner") + "/" + r.PathValue("repo")
	g.mu.Lock()
	repo := g.repos[name]
	g.mu.Unlock()
	if repo == nil {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	}
	return repo
}

func (g *GitHub) getRepo(w http.ResponseWriter, r *http.Request) {
	repo := g.lookup(w, r)
	if repo == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"full_name": repo.name,
		"archived":  repo.archived,
		"permissions": map[string]any{
			"admin": repo.admin && repo.push, "push": repo.push, "pull": true,
		},
	})
}

// nodeID é o id da issue no GraphQL. No fake ele leva o repositório e o número, para o deleteIssue achar a issue.
func nodeID(repo string, number int) string { return "I_" + repo + "#" + strconv.Itoa(number) }

// issueJSON é a issue como a API a devolve.
func issueJSON(repo string, i *GitHubIssue) map[string]any {
	labels := make([]map[string]any, len(i.Labels))
	for n, l := range i.Labels {
		labels[n] = map[string]any{"name": l, "color": "ededed"}
	}
	assignees := make([]map[string]any, len(i.Assignees))
	for n, a := range i.Assignees {
		assignees[n] = map[string]any{"login": a}
	}
	out := map[string]any{
		"node_id":    nodeID(repo, i.Number),
		"number":     i.Number,
		"title":      i.Title,
		"body":       nil,
		"state":      i.State,
		"html_url":   "https://github.com/" + repo + "/issues/" + strconv.Itoa(i.Number),
		"labels":     labels,
		"assignees":  assignees,
		"created_at": i.CreatedAt.Format(time.RFC3339),
		"updated_at": i.UpdatedAt.Format(time.RFC3339),
	}
	if i.Body != "" {
		out["body"] = i.Body
	}
	if i.StateReason != "" {
		out["state_reason"] = i.StateReason
	}
	if len(i.Assignees) > 0 {
		out["assignee"] = map[string]any{"login": i.Assignees[0]}
	}
	if i.PullRequest {
		out["pull_request"] = map[string]any{}
		out["html_url"] = "https://github.com/" + repo + "/pull/" + strconv.Itoa(i.Number)
	}
	return out
}

func (g *GitHub) listIssues(w http.ResponseWriter, r *http.Request) {
	repo := g.lookup(w, r)
	if repo == nil {
		return
	}
	q := r.URL.Query()
	state := q.Get("state")
	if state == "" {
		state = "open"
	}
	var since time.Time
	if s := q.Get("since"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			http.Error(w, `{"message":"Validation Failed"}`, http.StatusUnprocessableEntity)
			return
		}
		since = t
	}

	g.mu.Lock()
	var rows []*GitHubIssue
	for _, i := range repo.issues {
		if repo.gone[i.Number] || repo.moved[i.Number] {
			continue
		}
		if state != "all" && i.State != state {
			continue
		}
		if !since.IsZero() && !i.UpdatedAt.After(since) {
			continue
		}
		rows = append(rows, i)
	}
	sortBy := q.Get("sort")
	desc := q.Get("direction") != "asc"
	sort.SliceStable(rows, func(a, b int) bool {
		x, y := rows[a], rows[b]
		var less bool
		switch sortBy {
		case "updated":
			less = x.UpdatedAt.Before(y.UpdatedAt) || (x.UpdatedAt.Equal(y.UpdatedAt) && x.Number < y.Number)
		default:
			less = x.CreatedAt.Before(y.CreatedAt) || (x.CreatedAt.Equal(y.CreatedAt) && x.Number < y.Number)
		}
		if desc {
			return !less
		}
		return less
	})

	perPage, page := intParam(q, "per_page", 30), intParam(q, "page", 1)
	perPage = min(max(perPage, 1), 100)
	from := min((page-1)*perPage, len(rows))
	to := min(from+perPage, len(rows))
	out := make([]map[string]any, 0, to-from)
	for _, i := range rows[from:to] {
		out = append(out, issueJSON(repo.name, i))
	}
	g.mu.Unlock()

	if to < len(rows) {
		next := *r.URL
		nq := next.Query()
		nq.Set("page", strconv.Itoa(page+1))
		nq.Set("per_page", strconv.Itoa(perPage))
		next.RawQuery = nq.Encode()
		w.Header().Set("Link", fmt.Sprintf(`<http://%s%s>; rel="next"`, r.Host, next.RequestURI()))
	}
	writeJSON(w, http.StatusOK, out)
}

func intParam(q url.Values, key string, def int) int {
	if n, err := strconv.Atoi(q.Get(key)); err == nil && n > 0 {
		return n
	}
	return def
}

// issueOr404 acha a issue do caminho, ou já responde o que o GitHub responde por ela: 301 para a
// transferida, 410 para a apagada, 404 para a que nunca existiu.
func (g *GitHub) issueOr404(w http.ResponseWriter, r *http.Request) (*ghRepo, *GitHubIssue) {
	repo := g.lookup(w, r)
	if repo == nil {
		return nil, nil
	}
	number, err := strconv.Atoi(r.PathValue("number"))
	g.mu.Lock()
	defer g.mu.Unlock()
	switch {
	case err != nil:
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	case repo.moved[number]:
		w.Header().Set("Location", "https://api.github.com/repositories/1/issues/"+strconv.Itoa(number))
		http.Error(w, `{"message":"Moved Permanently"}`, http.StatusMovedPermanently)
	case repo.gone[number]:
		http.Error(w, `{"message":"This issue was deleted"}`, http.StatusGone)
	default:
		if i := g.find(repo, number); i != nil {
			return repo, i
		}
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	}
	return nil, nil
}

func (g *GitHub) getIssue(w http.ResponseWriter, r *http.Request) {
	repo, issue := g.issueOr404(w, r)
	if issue == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	writeJSON(w, http.StatusOK, issueJSON(repo.name, issue))
}

// createIssue abre uma issue. Como no GitHub, quem não tem permissão de escrita tem as etiquetas e os
// responsáveis descartados sem aviso, e uma etiqueta que o repositório não tem também é descartada.
func (g *GitHub) createIssue(w http.ResponseWriter, r *http.Request) {
	repo := g.lookup(w, r)
	if repo == nil {
		return
	}
	var in struct {
		Title     string   `json:"title"`
		Body      *string  `json:"body"`
		Labels    []string `json:"labels"`
		Assignees []string `json:"assignees"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, `{"message":"Problems parsing JSON"}`, http.StatusBadRequest)
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if repo.noIssues {
		http.Error(w, `{"message":"Issues are disabled for this repo"}`, http.StatusGone)
		return
	}
	if strings.TrimSpace(in.Title) == "" {
		http.Error(w, `{"message":"Validation Failed"}`, http.StatusUnprocessableEntity)
		return
	}
	issue := &GitHubIssue{Number: repo.next, Title: in.Title, State: "open"}
	if in.Body != nil {
		issue.Body = *in.Body
	}
	if repo.push && !repo.archived {
		for _, n := range in.Labels {
			if known, ok := repo.labels[strings.ToLower(n)]; ok {
				issue.Labels = append(issue.Labels, known)
			}
		}
		for _, l := range in.Assignees {
			if _, ok := g.users[l]; ok && !g.nologin[l] {
				issue.Assignees = append(issue.Assignees, l)
			}
		}
	}
	repo.next++
	issue.CreatedAt = g.bump()
	issue.UpdatedAt = issue.CreatedAt
	repo.issues = append(repo.issues, issue)
	writeJSON(w, http.StatusCreated, issueJSON(repo.name, issue))
}

// graphql atende a única mutation que o sistema usa, deleteIssue. Como o GitHub, responde 200 com o motivo em
// errors: FORBIDDEN para quem não é admin do repositório, NOT_FOUND para o id que não existe.
func (g *GitHub) graphql(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Query     string `json:"query"`
		Variables struct {
			ID string `json:"id"`
		} `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || !strings.Contains(in.Query, "deleteIssue") {
		http.Error(w, `{"message":"Problems parsing JSON"}`, http.StatusBadRequest)
		return
	}
	fail := func(kind, message string) {
		writeJSON(w, http.StatusOK, map[string]any{
			"data":   map[string]any{"deleteIssue": nil},
			"errors": []map[string]any{{"type": kind, "path": []string{"deleteIssue"}, "message": message}},
		})
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	var repo *ghRepo
	var issue *GitHubIssue
	if name, number, ok := strings.Cut(strings.TrimPrefix(in.Variables.ID, "I_"), "#"); ok && strings.HasPrefix(in.Variables.ID, "I_") {
		if n, err := strconv.Atoi(number); err == nil {
			if repo = g.repos[name]; repo != nil && !repo.gone[n] {
				issue = g.find(repo, n)
			}
		}
	}
	switch {
	case issue == nil:
		fail("NOT_FOUND", "Could not resolve to a node with the global id of '"+in.Variables.ID+"'")
	case !repo.admin || !repo.push || repo.archived:
		fail("FORBIDDEN", "Resource not accessible by personal access token: you do not have the correct permissions to execute `DeleteIssue`")
	default:
		repo.gone[issue.Number] = true
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleteIssue": map[string]any{"clientMutationId": nil}}})
	}
}

func (g *GitHub) patchIssue(w http.ResponseWriter, r *http.Request) {
	repo, issue := g.issueOr404(w, r)
	if issue == nil {
		return
	}
	var in map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, `{"message":"Problems parsing JSON"}`, http.StatusBadRequest)
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if !repo.push || repo.archived {
		http.Error(w, `{"message":"Must have push access to edit issues"}`, http.StatusForbidden)
		return
	}
	// Os campos entram numa ordem fixa: o estado antes do motivo, que o fecha com "completed" se faltar.
	for _, field := range []string{"title", "body", "state", "state_reason", "labels", "assignees"} {
		raw, sent := in[field]
		if !sent || repo.discard[field] {
			continue
		}
		switch field {
		case "title":
			json.Unmarshal(raw, &issue.Title)
		case "body":
			var s *string
			json.Unmarshal(raw, &s)
			issue.Body = ""
			if s != nil {
				issue.Body = *s
			}
		case "state":
			var state string
			json.Unmarshal(raw, &state)
			if state != issue.State {
				issue.State = state
				issue.StateReason = "completed"
				if state == "open" {
					issue.StateReason = "reopened"
				}
			}
		case "state_reason":
			json.Unmarshal(raw, &issue.StateReason)
		case "labels":
			var names []string
			json.Unmarshal(raw, &names)
			// Uma etiqueta que o repositório não tem é descartada: a sincronização cria antes de pedir.
			issue.Labels = nil
			for _, n := range names {
				if known, ok := repo.labels[strings.ToLower(n)]; ok {
					issue.Labels = append(issue.Labels, known)
				}
			}
		case "assignees":
			var logins []string
			json.Unmarshal(raw, &logins)
			issue.Assignees = nil
			for _, l := range logins {
				if _, ok := g.users[l]; ok && !g.nologin[l] {
					issue.Assignees = append(issue.Assignees, l)
				}
			}
		}
	}
	issue.UpdatedAt = g.bump()
	writeJSON(w, http.StatusOK, issueJSON(repo.name, issue))
}

func (g *GitHub) listLabels(w http.ResponseWriter, r *http.Request) {
	repo := g.lookup(w, r)
	if repo == nil {
		return
	}
	g.mu.Lock()
	var names []string
	for _, n := range repo.labels {
		names = append(names, n)
	}
	g.mu.Unlock()
	sort.Strings(names)

	q := r.URL.Query()
	perPage, page := min(intParam(q, "per_page", 30), 100), intParam(q, "page", 1)
	from := min((page-1)*perPage, len(names))
	to := min(from+perPage, len(names))
	out := make([]map[string]any, 0, to-from)
	for _, n := range names[from:to] {
		out = append(out, map[string]any{"name": n, "color": "ededed"})
	}
	if to < len(names) {
		next := *r.URL
		nq := next.Query()
		nq.Set("page", strconv.Itoa(page+1))
		nq.Set("per_page", strconv.Itoa(perPage))
		next.RawQuery = nq.Encode()
		w.Header().Set("Link", fmt.Sprintf(`<http://%s%s>; rel="next"`, r.Host, next.RequestURI()))
	}
	writeJSON(w, http.StatusOK, out)
}

func (g *GitHub) createLabel(w http.ResponseWriter, r *http.Request) {
	repo := g.lookup(w, r)
	if repo == nil {
		return
	}
	var in struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Name) == "" {
		http.Error(w, `{"message":"Validation Failed"}`, http.StatusUnprocessableEntity)
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if repo.labelError != 0 {
		http.Error(w, `{"message":"label creation refused"}`, repo.labelError)
		return
	}
	if !repo.push {
		http.Error(w, `{"message":"Must have push access to create labels"}`, http.StatusForbidden)
		return
	}
	if _, dup := repo.labels[strings.ToLower(in.Name)]; dup {
		http.Error(w, `{"message":"Validation Failed","errors":[{"code":"already_exists"}]}`, http.StatusUnprocessableEntity)
		return
	}
	repo.labels[strings.ToLower(in.Name)] = in.Name
	writeJSON(w, http.StatusCreated, map[string]any{"name": in.Name, "color": in.Color})
}

func (g *GitHub) getUser(w http.ResponseWriter, r *http.Request) {
	login := r.PathValue("login")
	g.mu.Lock()
	email, ok := g.users[login]
	g.mu.Unlock()
	if !ok {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		return
	}
	out := map[string]any{"login": login, "email": nil}
	if email != "" {
		out["email"] = email
	}
	writeJSON(w, http.StatusOK, out)
}

// searchUsers atende "<e-mail> in:email": acha os usuários cujo e-mail público é esse.
func (g *GitHub) searchUsers(w http.ResponseWriter, r *http.Request) {
	var email string
	for _, term := range strings.Fields(r.URL.Query().Get("q")) {
		if !strings.Contains(term, ":") {
			email = term
		}
	}
	g.mu.Lock()
	var items []map[string]any
	if email != "" {
		var logins []string
		for login, e := range g.users {
			if e != "" && strings.EqualFold(e, email) {
				logins = append(logins, login)
			}
		}
		sort.Strings(logins)
		for _, l := range logins {
			items = append(items, map[string]any{"login": l})
		}
	}
	g.mu.Unlock()
	if items == nil {
		items = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"total_count": len(items), "items": items})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// LoopbackOnly devolve um transporte que recusa qualquer endereço que não seja da própria máquina.
// Os testes o põem no lugar do transporte padrão: um teste que apontasse para o GitHub de verdade
// falha em vez de escrever nas issues de alguém.
func LoopbackOnly(next http.RoundTripper) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}
	return loopbackOnly{next}
}

type loopbackOnly struct{ next http.RoundTripper }

func (t loopbackOnly) RoundTrip(req *http.Request) (*http.Response, error) {
	host := req.URL.Hostname()
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil, fmt.Errorf("testutil: request to %s refused: tests only talk to loopback", req.URL.Host)
	}
	return t.next.RoundTrip(req)
}

// BlockExternalHTTP troca o transporte padrão por LoopbackOnly. É idempotente.
func BlockExternalHTTP() {
	if _, ok := http.DefaultTransport.(loopbackOnly); !ok {
		http.DefaultTransport = LoopbackOnly(http.DefaultTransport)
	}
}
