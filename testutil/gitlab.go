package testutil

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Os níveis de acesso do GitLab que o fake usa (GitLabAccess).
const (
	GitLabGuest     = 10
	GitLabPlanner   = 15
	GitLabReporter  = 20
	GitLabDeveloper = 30
	GitLabOwner     = 50
)

// GitLabIssue é uma issue do GitLab fake. Os campos são os que a sincronização lê e escreve; o resto da issue
// de verdade não existe aqui.
type GitLabIssue struct {
	IID         int
	Title       string
	Description string
	State       string // "opened" ou "closed"
	// Type é o tipo do item ("issue", o padrão, "incident", "task"): a lista do GitLab traz todos juntos.
	Type      string
	Labels    []string
	Assignees []string // usernames
	CreatedAt time.Time
	UpdatedAt time.Time
	// MovedTo é a issue para onde ela foi transferida: ela fica no projeto, fechada, com moved_to_id.
	MovedTo int
	// Deleted só vem preenchido por Issue: a issue foi apagada, e o GitLab responde 404 por ela.
	Deleted bool
}

// GitLabRequest é uma requisição que o GitLab fake recebeu, para o teste contar o que o sistema pediu.
type GitLabRequest struct {
	Method string
	Path   string // o caminho escapado, como chegou ("/api/v4/projects/group%2Fproject/issues")
	Query  string
	Body   string
}

// GitLab é um GitLab falso com estado, no molde do GitHub fake: projetos com issues, etiquetas e o nível de
// acesso da conta, usuários com e-mail público ou não, a busca de usuário e o OAuth com o token que vence e
// a renovação que gira o par. A API fica em /api/v4 e o OAuth na raiz, como no gitlab.com. Tem chaves para
// os defeitos que a sincronização precisa aguentar: falta de permissão, issue apagada ou transferida, issues
// desligadas, limite de requisições, token vencido ou revogado. Só atende em loopback.
//
// Conhece os projetos group/project (com a issue 7) e group/other, e o usuário octolab.
type GitLab struct {
	mu       sync.Mutex
	projects map[string]*glProject
	users    map[string]*glUser // username → usuário
	nologin  map[string]bool    // usernames que existem mas o GitLab descarta como responsável
	requests []GitLabRequest
	faults   []*ghFault
	handler  *http.ServeMux
	last     time.Time
	limited  time.Time
	remains  int
	nextID   int

	// O OAuth: os tokens de acesso que o fake emitiu e quando vencem, o par de cada token de renovação, e os
	// tokens que deixaram de valer (girados ou revogados).
	lifetime  time.Duration
	issued    map[string]time.Time
	refresh   map[string]string // token de renovação → token de acesso do par
	dead      map[string]bool
	refreshes int
	seq       int
}

type glUser struct {
	id       int
	username string
	email    string // o e-mail público; vazio é sem
}

type glProject struct {
	id         int
	path       string
	access     int // o nível de acesso da conta; zero é não ser membro
	archived   bool
	noIssues   bool
	private    bool
	issues     []*GitLabIssue
	next       int
	labels     map[string]string // minúsculas → nome como foi criado
	discard    map[string]bool   // campos que o PUT descarta sem avisar: labels, assignee_ids, title, description, state_event
	gone       map[int]bool      // issues apagadas: 404
	labelError int
}

// NewGitLab devolve o GitLab fake com os projetos e a issue 7 de sempre.
func NewGitLab() *GitLab {
	g := &GitLab{
		projects: map[string]*glProject{},
		users:    map[string]*glUser{},
		nologin:  map[string]bool{},
		remains:  -1,
		nextID:   100,
		lifetime: 2 * time.Hour,
		issued:   map[string]time.Time{},
		refresh:  map[string]string{},
		dead:     map[string]bool{},
	}
	g.users[GitLabLogin] = &glUser{id: GitLabUserID, username: GitLabLogin}
	g.handler = g.routes()
	g.AddProject("group/project")
	g.AddProject("group/other")
	g.AddIssue("group/project", GitLabIssue{IID: 7, Title: "Ajustar relatório", State: "opened"})
	return g
}

// FakeGitLab é o GitLab fake como http.Handler, para quem só precisa do servidor. A API fica em /api/v4.
func FakeGitLab() http.Handler { return NewGitLab() }

func (g *GitLab) bump() time.Time {
	now := time.Now().UTC().Truncate(time.Second)
	if !now.After(g.last) {
		now = g.last.Add(time.Second)
	}
	g.last = now
	return now
}

// AddProject cria um projeto em que a conta é Owner.
func (g *GitLab) AddProject(path string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nextID++
	g.projects[path] = &glProject{
		id: g.nextID, path: path, access: GitLabOwner, next: 1, private: true,
		labels:  map[string]string{},
		discard: map[string]bool{},
		gone:    map[int]bool{},
	}
}

func (g *GitLab) project(path string) *glProject {
	p := g.projects[path]
	if p == nil {
		panic("testutil: unknown fake GitLab project " + path)
	}
	return p
}

// AddUser cadastra um usuário. email é o e-mail público do perfil; vazio é o usuário que não publica e-mail.
func (g *GitLab) AddUser(username, email string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if u := g.users[username]; u != nil {
		u.email = email
		return
	}
	g.nextID++
	g.users[username] = &glUser{id: g.nextID, username: username, email: email}
}

// Unassignable faz o GitLab descartar o usuário quando ele é posto como responsável de uma issue, como
// acontece com quem não é membro do projeto.
func (g *GitLab) Unassignable(username string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nologin[username] = true
}

// SetAccess muda o nível de acesso da conta no projeto (GitLabGuest, GitLabPlanner, ..., GitLabOwner; zero é
// não ser membro).
func (g *GitLab) SetAccess(path string, level int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.project(path).access = level
}

// SetArchived arquiva o projeto: ele continua legível e deixa de aceitar escrita.
func (g *GitLab) SetArchived(path string, archived bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.project(path).archived = archived
}

// DisableIssues desliga as issues do projeto: criar uma passa a devolver 403, como o GitLab.
func (g *GitLab) DisableIssues(path string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.project(path).noIssues = true
}

// AddIssue cria uma issue. IID zero pega o próximo número do projeto; o estado vazio é aberta e o tipo vazio
// é issue. Devolve o iid.
func (g *GitLab) AddIssue(path string, in GitLabIssue) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	p := g.project(path)
	if in.IID == 0 {
		in.IID = p.next
	}
	if in.IID >= p.next {
		p.next = in.IID + 1
	}
	if in.State == "" {
		in.State = "opened"
	}
	if in.Type == "" {
		in.Type = "issue"
	}
	in.CreatedAt = g.bump()
	in.UpdatedAt = in.CreatedAt
	in.Labels = append([]string(nil), in.Labels...)
	in.Assignees = append([]string(nil), in.Assignees...)
	for _, l := range in.Labels {
		p.labels[strings.ToLower(l)] = l
	}
	p.issues = append(p.issues, &in)
	return in.IID
}

// Issue devolve uma cópia da issue como está no fake.
func (g *GitLab) Issue(path string, iid int) (GitLabIssue, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	p := g.project(path)
	if i := g.find(p, iid); i != nil {
		out := *i
		out.Labels = append([]string(nil), i.Labels...)
		out.Assignees = append([]string(nil), i.Assignees...)
		out.Deleted = p.gone[iid]
		return out, true
	}
	return GitLabIssue{}, false
}

func (g *GitLab) find(p *glProject, iid int) *GitLabIssue {
	for _, i := range p.issues {
		if i.IID == iid {
			return i
		}
	}
	return nil
}

// EditIssue é alguém mexendo na issue pelo site do GitLab: aplica a função e atualiza o updated_at.
func (g *GitLab) EditIssue(path string, iid int, edit func(*GitLabIssue)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	p := g.project(path)
	i := g.find(p, iid)
	if i == nil {
		panic(fmt.Sprintf("testutil: fake GitLab %s has no issue %d", path, iid))
	}
	edit(i)
	for _, l := range i.Labels {
		p.labels[strings.ToLower(l)] = l
	}
	i.UpdatedAt = g.bump()
}

// DeleteIssue apaga a issue: o GitLab passa a responder 404.
func (g *GitLab) DeleteIssue(path string, iid int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.project(path).gone[iid] = true
}

// MoveIssue transfere a issue para outro projeto: ela continua aqui, fechada, com moved_to_id.
func (g *GitLab) MoveIssue(path string, iid int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	p := g.project(path)
	i := g.find(p, iid)
	if i == nil {
		panic(fmt.Sprintf("testutil: fake GitLab %s has no issue %d", path, iid))
	}
	g.nextID++
	i.MovedTo = g.nextID
	i.State = "closed"
	i.UpdatedAt = g.bump()
}

// Discard faz o PUT descartar estes campos sem avisar (labels, assignee_ids, title, description, state_event):
// responde 200 com a issue como estava.
func (g *GitLab) Discard(path string, fields ...string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	p := g.project(path)
	p.discard = map[string]bool{}
	for _, f := range fields {
		p.discard[f] = true
	}
}

// LabelCreateStatus faz a criação de etiqueta devolver este status (403, por exemplo). Zero volta ao normal.
func (g *GitLab) LabelCreateStatus(path string, status int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.project(path).labelError = status
}

// LabelNames lista as etiquetas que o projeto tem, em ordem.
func (g *GitLab) LabelNames(path string) []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	var out []string
	for _, n := range g.project(path).labels {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Fail faz as próximas `times` requisições que casam com o método e o prefixo do caminho (escapado, com o
// /api/v4) responderem com este status, sem tocar no estado.
func (g *GitLab) Fail(method, pathPrefix string, status, times int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.faults = append(g.faults, &ghFault{method: method, prefix: pathPrefix, status: status, times: times})
}

// RateLimitedFor recusa toda requisição da API, por limite, durante este tempo.
func (g *GitLab) RateLimitedFor(d time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.limited = time.Now().Add(d)
}

// Remaining muda o RateLimit-Remaining que as respostas informam. Negativo volta ao padrão.
func (g *GitLab) Remaining(n int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.remains = n
}

// TokenLifetime muda quanto vale o token de acesso que o OAuth emite daqui em diante (o padrão é 2 horas,
// como no gitlab.com). Um tempo curto exercita a renovação.
func (g *GitLab) TokenLifetime(d time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.lifetime = d
}

// RevokeTokens revoga todos os tokens que o OAuth emitiu, como quem tira a autorização do app no GitLab: a API
// responde 401 por eles e a renovação recusa (invalid_grant).
func (g *GitLab) RevokeTokens() {
	g.mu.Lock()
	defer g.mu.Unlock()
	for token := range g.issued {
		g.dead[token] = true
	}
	g.refresh = map[string]string{}
}

// Refreshes conta as renovações de token que deram certo.
func (g *GitLab) Refreshes() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.refreshes
}

// Requests devolve o que o fake recebeu, na ordem.
func (g *GitLab) Requests() []GitLabRequest {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]GitLabRequest(nil), g.requests...)
}

// Count conta as requisições do método cujo caminho (escapado) começa com o prefixo.
func (g *GitLab) Count(method, pathPrefix string) int {
	n := 0
	for _, r := range g.Requests() {
		if r.Method == method && strings.HasPrefix(r.Path, pathPrefix) {
			n++
		}
	}
	return n
}

// Writes conta as requisições da API que escrevem (tudo que não é GET, fora o OAuth).
func (g *GitLab) Writes() int {
	n := 0
	for _, r := range g.Requests() {
		if r.Method != http.MethodGet && strings.HasPrefix(r.Path, "/api/") {
			n++
		}
	}
	return n
}

// Reset apaga o registro de requisições.
func (g *GitLab) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.requests = nil
}

// ServeHTTP atende a API e o OAuth no mesmo endereço.
func (g *GitLab) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	r.Body = io.NopCloser(strings.NewReader(string(body)))

	g.mu.Lock()
	g.requests = append(g.requests, GitLabRequest{Method: r.Method, Path: r.URL.EscapedPath(), Query: r.URL.RawQuery, Body: string(body)})
	remains := g.remains
	limited := g.limited
	var fault *ghFault
	for _, f := range g.faults {
		if f.times > 0 && f.method == r.Method && strings.HasPrefix(r.URL.EscapedPath(), f.prefix) {
			f.times--
			fault = f
			break
		}
	}
	g.mu.Unlock()

	h := w.Header()
	h.Set("Content-Type", "application/json")
	h.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	api := strings.HasPrefix(r.URL.Path, "/api/")
	if remains >= 0 {
		h.Set("RateLimit-Remaining", strconv.Itoa(remains))
		h.Set("RateLimit-Reset", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10))
	}
	if api && time.Now().Before(limited) {
		h.Set("RateLimit-Remaining", "0")
		h.Set("RateLimit-Reset", strconv.FormatInt(limited.Unix(), 10))
		h.Set("Retry-After", strconv.Itoa(max(1, int(time.Until(limited).Seconds()))))
		http.Error(w, `{"message":"Retry later"}`, http.StatusTooManyRequests)
		return
	}
	if fault != nil {
		http.Error(w, `{"message":"injected failure"}`, fault.status)
		return
	}
	g.handler.ServeHTTP(w, r)
}

// authorized diz se o token da requisição vale: o InvalidToken nunca vale, e um token que o OAuth emitiu
// deixa de valer quando vence, gira ou é revogado. Qualquer outro é um token pessoal válido.
func (g *GitLab) authorized(r *http.Request) bool {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		token = r.Header.Get("PRIVATE-TOKEN")
	}
	if token == "" || token == InvalidToken {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.dead[token] {
		return false
	}
	if expires, issued := g.issued[token]; issued && time.Now().After(expires) {
		return false
	}
	return true
}

func (g *GitLab) routes() *http.ServeMux {
	mux := http.NewServeMux()
	auth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !g.authorized(r) {
				http.Error(w, `{"message":"401 Unauthorized"}`, http.StatusUnauthorized)
				return
			}
			next(w, r)
		}
	}

	mux.HandleFunc("GET /api/v4/user", auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"id": GitLabUserID, "username": GitLabLogin})
	}))
	mux.HandleFunc("GET /api/v4/projects", auth(g.listProjects))
	mux.HandleFunc("GET /api/v4/projects/{id}", auth(g.getProject))
	mux.HandleFunc("GET /api/v4/projects/{id}/members/all/{user}", auth(g.getMember))
	mux.HandleFunc("GET /api/v4/projects/{id}/issues", auth(g.listIssues))
	mux.HandleFunc("POST /api/v4/projects/{id}/issues", auth(g.createIssue))
	mux.HandleFunc("GET /api/v4/projects/{id}/issues/{iid}", auth(g.getIssue))
	mux.HandleFunc("PUT /api/v4/projects/{id}/issues/{iid}", auth(g.putIssue))
	mux.HandleFunc("DELETE /api/v4/projects/{id}/issues/{iid}", auth(g.deleteIssue))
	mux.HandleFunc("GET /api/v4/projects/{id}/labels", auth(g.listLabels))
	mux.HandleFunc("POST /api/v4/projects/{id}/labels", auth(g.createLabel))
	mux.HandleFunc("GET /api/v4/users", auth(g.listUsers))
	mux.HandleFunc("GET /api/v4/users/{user}", auth(g.getUser))

	mux.HandleFunc("GET /oauth/authorize", g.authorize)
	mux.HandleFunc("POST /oauth/token", g.token)
	return mux
}

// authorize é a tela de autorização do GitLab com o "Autorizar" já clicado: devolve a pessoa ao endereço de
// retorno com o código.
func (g *GitLab) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("client_id") != GitLabClientID || q.Get("response_type") != "code" {
		http.Error(w, "unknown client_id", http.StatusNotFound)
		return
	}
	back, err := url.Parse(q.Get("redirect_uri"))
	if err != nil || back.Scheme == "" {
		http.Error(w, "invalid redirect_uri", http.StatusBadRequest)
		return
	}
	v := back.Query()
	v.Set("code", GitLabOAuthCode)
	v.Set("state", q.Get("state"))
	back.RawQuery = v.Encode()
	http.Redirect(w, r, back.String(), http.StatusFound)
}

// token troca o código (authorization_code) ou o token de renovação (refresh_token) por um par novo. A
// renovação gira o par: o token de acesso e o de renovação antigos deixam de valer.
func (g *GitLab) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	form := r.PostForm
	if form.Get("client_id") != GitLabClientID || form.Get("client_secret") != GitLabClientSecret {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_client"})
		return
	}
	if form.Get("redirect_uri") == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	var access, refresh string
	switch form.Get("grant_type") {
	case "authorization_code":
		switch form.Get("code") {
		case GitLabOAuthCode:
			access, refresh = GitLabOAuthToken, GitLabRefreshToken
		case GitLabSecondOAuthCode:
			access, refresh = GitLabSecondOAuthToken, GitLabSecondRefreshToken
		default:
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_grant"})
			return
		}
	case "refresh_token":
		old := form.Get("refresh_token")
		pair, ok := g.refresh[old]
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_grant"})
			return
		}
		delete(g.refresh, old)
		g.dead[pair] = true
		g.seq++
		g.refreshes++
		// O token novo guarda de qual conta veio: o segundo app lista outros projetos.
		prefix := "gloas-"
		if strings.HasPrefix(pair, GitLabSecondOAuthToken) {
			prefix = GitLabSecondOAuthToken + "-"
		}
		access, refresh = prefix+strconv.Itoa(g.seq), "glrt-"+strconv.Itoa(g.seq)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unsupported_grant_type"})
		return
	}
	delete(g.dead, access)
	g.issued[access] = time.Now().Add(g.lifetime)
	g.refresh[refresh] = access
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  access,
		"token_type":    "Bearer",
		"expires_in":    int(g.lifetime.Seconds()),
		"refresh_token": refresh,
		"scope":         "api",
		"created_at":    time.Now().Unix(),
	})
}

func (g *GitLab) listProjects(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer "+GitLabSecondOAuthToken) {
		writeJSON(w, http.StatusOK, []map[string]any{{"path_with_namespace": "group/second", "visibility": "public"}})
		return
	}
	g.mu.Lock()
	var paths []string
	for path, p := range g.projects {
		if p.access > 0 {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	out := make([]map[string]any, 0, len(paths))
	for _, path := range paths {
		visibility := "private"
		if !g.projects[path].private {
			visibility = "public"
		}
		out = append(out, map[string]any{"path_with_namespace": path, "visibility": visibility})
	}
	g.mu.Unlock()
	writeJSON(w, http.StatusOK, out)
}

// lookup acha o projeto do caminho, pelo caminho escapado ("group%2Fproject") ou pelo id.
func (g *GitLab) lookup(w http.ResponseWriter, r *http.Request) *glProject {
	id := r.PathValue("id")
	g.mu.Lock()
	p := g.projects[id]
	if p == nil {
		if n, err := strconv.Atoi(id); err == nil {
			for _, candidate := range g.projects {
				if candidate.id == n {
					p = candidate
				}
			}
		}
	}
	g.mu.Unlock()
	if p == nil {
		http.Error(w, `{"message":"404 Project Not Found"}`, http.StatusNotFound)
	}
	return p
}

func (g *GitLab) projectJSON(p *glProject) map[string]any {
	return map[string]any{
		"id":                  p.id,
		"path_with_namespace": p.path,
		"archived":            p.archived,
		"issues_enabled":      !p.noIssues,
		"web_url":             "https://gitlab.com/" + p.path,
	}
}

func (g *GitLab) getProject(w http.ResponseWriter, r *http.Request) {
	p := g.lookup(w, r)
	if p == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	writeJSON(w, http.StatusOK, g.projectJSON(p))
}

func (g *GitLab) getMember(w http.ResponseWriter, r *http.Request) {
	p := g.lookup(w, r)
	if p == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if r.PathValue("user") != strconv.Itoa(GitLabUserID) || p.access == 0 {
		http.Error(w, `{"message":"404 Not found"}`, http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": GitLabUserID, "username": GitLabLogin, "access_level": p.access})
}

func (g *GitLab) issueJSON(p *glProject, i *GitLabIssue) map[string]any {
	assignees := make([]map[string]any, 0, len(i.Assignees))
	for _, a := range i.Assignees {
		if u := g.users[a]; u != nil {
			assignees = append(assignees, map[string]any{"id": u.id, "username": u.username})
		}
	}
	labels := append([]string{}, i.Labels...)
	out := map[string]any{
		"id":          p.id*1000 + i.IID,
		"iid":         i.IID,
		"project_id":  p.id,
		"title":       i.Title,
		"description": nil,
		"state":       i.State,
		"issue_type":  i.Type,
		"labels":      labels,
		"assignees":   assignees,
		"web_url":     "https://gitlab.com/" + p.path + "/-/issues/" + strconv.Itoa(i.IID),
		"created_at":  i.CreatedAt.Format("2006-01-02T15:04:05.000Z"),
		"updated_at":  i.UpdatedAt.Format("2006-01-02T15:04:05.000Z"),
		"moved_to_id": nil,
	}
	if i.Description != "" {
		out["description"] = i.Description
	}
	if i.MovedTo != 0 {
		out["moved_to_id"] = i.MovedTo
	}
	return out
}

func (g *GitLab) listIssues(w http.ResponseWriter, r *http.Request) {
	p := g.lookup(w, r)
	if p == nil {
		return
	}
	q := r.URL.Query()
	// Sem state, o GitLab traz todas.
	state := q.Get("state")
	var after time.Time
	if s := q.Get("updated_after"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			http.Error(w, `{"error":"updated_after is invalid"}`, http.StatusBadRequest)
			return
		}
		after = t
	}

	g.mu.Lock()
	var rows []*GitLabIssue
	for _, i := range p.issues {
		if p.gone[i.IID] {
			continue
		}
		if state != "" && state != "all" && i.State != state {
			continue
		}
		if t := q.Get("issue_type"); t != "" && i.Type != t {
			continue
		}
		if !after.IsZero() && i.UpdatedAt.Before(after) {
			continue
		}
		rows = append(rows, i)
	}
	byUpdated := q.Get("order_by") == "updated_at"
	desc := q.Get("sort") != "asc"
	sort.SliceStable(rows, func(a, b int) bool {
		x, y := rows[a], rows[b]
		var less bool
		if byUpdated {
			less = x.UpdatedAt.Before(y.UpdatedAt) || (x.UpdatedAt.Equal(y.UpdatedAt) && x.IID < y.IID)
		} else {
			less = x.CreatedAt.Before(y.CreatedAt) || (x.CreatedAt.Equal(y.CreatedAt) && x.IID < y.IID)
		}
		if desc {
			return !less
		}
		return less
	})

	perPage, page := intParam(q, "per_page", 20), intParam(q, "page", 1)
	perPage = min(max(perPage, 1), 100)
	from := min((page-1)*perPage, len(rows))
	to := min(from+perPage, len(rows))
	out := make([]map[string]any, 0, to-from)
	for _, i := range rows[from:to] {
		out = append(out, g.issueJSON(p, i))
	}
	g.mu.Unlock()

	if to < len(rows) {
		g.linkNext(w, r, page, perPage)
	}
	writeJSON(w, http.StatusOK, out)
}

func (g *GitLab) linkNext(w http.ResponseWriter, r *http.Request, page, perPage int) {
	next := *r.URL
	nq := next.Query()
	nq.Set("page", strconv.Itoa(page+1))
	nq.Set("per_page", strconv.Itoa(perPage))
	next.RawQuery = nq.Encode()
	w.Header().Set("Link", fmt.Sprintf(`<http://%s%s?%s>; rel="next"`, r.Host, r.URL.EscapedPath(), next.RawQuery))
}

// issueOr404 acha a issue do caminho, ou já responde 404 pela apagada e pela que nunca existiu.
func (g *GitLab) issueOr404(w http.ResponseWriter, r *http.Request) (*glProject, *GitLabIssue) {
	p := g.lookup(w, r)
	if p == nil {
		return nil, nil
	}
	iid, err := strconv.Atoi(r.PathValue("iid"))
	g.mu.Lock()
	defer g.mu.Unlock()
	if err == nil && !p.gone[iid] {
		if i := g.find(p, iid); i != nil {
			return p, i
		}
	}
	http.Error(w, `{"message":"404 Not found"}`, http.StatusNotFound)
	return nil, nil
}

func (g *GitLab) getIssue(w http.ResponseWriter, r *http.Request) {
	p, issue := g.issueOr404(w, r)
	if issue == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	writeJSON(w, http.StatusOK, g.issueJSON(p, issue))
}

// splitLabels lê a lista de etiquetas separada por vírgulas, como a API do GitLab a recebe.
func splitLabels(raw string) []string {
	var out []string
	for _, l := range strings.Split(raw, ",") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// setLabels põe as etiquetas na issue. Como o GitLab, cria no projeto a que ainda não existe.
func (p *glProject) setLabels(i *GitLabIssue, names []string) {
	i.Labels = nil
	for _, n := range names {
		known, ok := p.labels[strings.ToLower(n)]
		if !ok {
			known = n
			p.labels[strings.ToLower(n)] = n
		}
		i.Labels = append(i.Labels, known)
	}
}

// assign põe os responsáveis pelos ids. O id que o GitLab não conhece (ou o zero) e o usuário que não pode
// ser responsável ficam de fora sem erro.
func (g *GitLab) assign(i *GitLabIssue, ids []int) {
	i.Assignees = nil
	for _, id := range ids {
		for _, u := range g.users {
			if u.id == id && !g.nologin[u.username] {
				i.Assignees = append(i.Assignees, u.username)
			}
		}
	}
}

// createIssue abre uma issue. Como no GitLab, quem só é Guest abre a issue mas tem as etiquetas e os
// responsáveis descartados sem aviso.
func (g *GitLab) createIssue(w http.ResponseWriter, r *http.Request) {
	p := g.lookup(w, r)
	if p == nil {
		return
	}
	var in struct {
		Title       string  `json:"title"`
		Description *string `json:"description"`
		Labels      string  `json:"labels"`
		AssigneeIDs []int   `json:"assignee_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if p.noIssues || p.archived || p.access < GitLabGuest {
		http.Error(w, `{"message":"403 Forbidden"}`, http.StatusForbidden)
		return
	}
	if strings.TrimSpace(in.Title) == "" {
		http.Error(w, `{"error":"title is missing"}`, http.StatusBadRequest)
		return
	}
	issue := &GitLabIssue{IID: p.next, Title: in.Title, State: "opened", Type: "issue"}
	if in.Description != nil {
		issue.Description = *in.Description
	}
	if p.access >= GitLabPlanner {
		p.setLabels(issue, splitLabels(in.Labels))
		g.assign(issue, in.AssigneeIDs)
	}
	p.next++
	issue.CreatedAt = g.bump()
	issue.UpdatedAt = issue.CreatedAt
	p.issues = append(p.issues, issue)
	writeJSON(w, http.StatusCreated, g.issueJSON(p, issue))
}

func (g *GitLab) putIssue(w http.ResponseWriter, r *http.Request) {
	p, issue := g.issueOr404(w, r)
	if issue == nil {
		return
	}
	var in map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if p.access < GitLabPlanner || p.archived {
		http.Error(w, `{"message":"403 Forbidden"}`, http.StatusForbidden)
		return
	}
	for _, field := range []string{"title", "description", "state_event", "labels", "assignee_ids"} {
		raw, sent := in[field]
		if !sent || p.discard[field] {
			continue
		}
		switch field {
		case "title":
			_ = json.Unmarshal(raw, &issue.Title)
		case "description":
			var s *string
			_ = json.Unmarshal(raw, &s)
			issue.Description = ""
			if s != nil {
				issue.Description = *s
			}
		case "state_event":
			var event string
			_ = json.Unmarshal(raw, &event)
			switch event {
			case "close":
				issue.State = "closed"
			case "reopen":
				issue.State = "opened"
			}
		case "labels":
			var names string
			_ = json.Unmarshal(raw, &names)
			p.setLabels(issue, splitLabels(names))
		case "assignee_ids":
			var ids []int
			_ = json.Unmarshal(raw, &ids)
			g.assign(issue, ids)
		}
	}
	issue.UpdatedAt = g.bump()
	writeJSON(w, http.StatusOK, g.issueJSON(p, issue))
}

// deleteIssue apaga a issue: só o Owner do projeto pode, como no GitLab.
func (g *GitLab) deleteIssue(w http.ResponseWriter, r *http.Request) {
	p, issue := g.issueOr404(w, r)
	if issue == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if p.access < GitLabOwner || p.archived {
		http.Error(w, `{"message":"403 Forbidden"}`, http.StatusForbidden)
		return
	}
	p.gone[issue.IID] = true
	w.WriteHeader(http.StatusNoContent)
}

func (g *GitLab) listLabels(w http.ResponseWriter, r *http.Request) {
	p := g.lookup(w, r)
	if p == nil {
		return
	}
	g.mu.Lock()
	var names []string
	for _, n := range p.labels {
		names = append(names, n)
	}
	g.mu.Unlock()
	sort.Strings(names)

	q := r.URL.Query()
	perPage, page := min(intParam(q, "per_page", 20), 100), intParam(q, "page", 1)
	from := min((page-1)*perPage, len(names))
	to := min(from+perPage, len(names))
	out := make([]map[string]any, 0, to-from)
	for _, n := range names[from:to] {
		out = append(out, map[string]any{"name": n, "color": "#ededed"})
	}
	if to < len(names) {
		g.linkNext(w, r, page, perPage)
	}
	writeJSON(w, http.StatusOK, out)
}

func (g *GitLab) createLabel(w http.ResponseWriter, r *http.Request) {
	p := g.lookup(w, r)
	if p == nil {
		return
	}
	var in struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Name) == "" || in.Color == "" {
		http.Error(w, `{"error":"name is missing"}`, http.StatusBadRequest)
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if p.labelError != 0 {
		http.Error(w, `{"message":"label creation refused"}`, p.labelError)
		return
	}
	if p.access < GitLabPlanner || p.archived {
		http.Error(w, `{"message":"403 Forbidden"}`, http.StatusForbidden)
		return
	}
	if _, dup := p.labels[strings.ToLower(in.Name)]; dup {
		http.Error(w, `{"message":"Label already exists"}`, http.StatusConflict)
		return
	}
	p.labels[strings.ToLower(in.Name)] = in.Name
	writeJSON(w, http.StatusCreated, map[string]any{"name": in.Name, "color": in.Color})
}

// listUsers atende username= (o usuário daquele username) e search= (os usuários cujo nome, username ou
// e-mail público casam). Como no GitLab para quem não é admin, a lista não traz o e-mail.
func (g *GitLab) listUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	g.mu.Lock()
	var found []*glUser
	for _, u := range g.users {
		switch {
		case q.Has("username"):
			if strings.EqualFold(u.username, q.Get("username")) {
				found = append(found, u)
			}
		case q.Has("search"):
			s := strings.ToLower(q.Get("search"))
			if s != "" && (strings.Contains(strings.ToLower(u.username), s) || (u.email != "" && strings.EqualFold(u.email, s))) {
				found = append(found, u)
			}
		}
	}
	g.mu.Unlock()
	sort.Slice(found, func(a, b int) bool { return found[a].id < found[b].id })
	out := make([]map[string]any, 0, len(found))
	for _, u := range found {
		out = append(out, map[string]any{"id": u.id, "username": u.username})
	}
	writeJSON(w, http.StatusOK, out)
}

func (g *GitLab) getUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("user"))
	g.mu.Lock()
	var user *glUser
	for _, u := range g.users {
		if err == nil && u.id == id {
			user = u
		}
	}
	g.mu.Unlock()
	if user == nil {
		http.Error(w, `{"message":"404 User Not Found"}`, http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": user.id, "username": user.username, "public_email": user.email})
}
