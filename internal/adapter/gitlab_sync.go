package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// A leitura e a escrita das issues do GitLab, para a sincronização com as tarefas (IssueSyncer). É o
// mesmo desenho do GitHub (github_sync.go); o que muda é o que a API do GitLab pede:
//   - o estado aberto se chama "opened", e fechar ou reabrir é state_event;
//   - o responsável se escreve pelo id do usuário (assignee_ids), não pelo username;
//   - a permissão de escrita é o nível de acesso da conta no projeto, herdado dos grupos;
//   - a issue transferida continua no projeto antigo, fechada e com moved_to_id;
//   - só a conta Owner apaga uma issue.

const (
	// gitlabMaxPages é o teto de páginas de uma listagem (100 itens por página).
	gitlabMaxPages = 100
	// gitlabRateReserve é o que sobra do limite de requisições quando a listagem para, como no GitHub.
	gitlabRateReserve = 25
	// gitlabBodyLimit é quanto de uma resposta se lê.
	gitlabBodyLimit = 16 << 20
	// gitlabCanWrite é o nível de acesso (Planner) a partir do qual a conta edita as issues dos outros,
	// as etiquetas e os responsáveis. Abaixo dele (Guest) ela só lê e abre issues.
	gitlabCanWrite = 15
	// gitlabEmailCandidates é quantos usuários da busca por e-mail são conferidos um a um.
	gitlabEmailCandidates = 5
)

var (
	_ IssueSyncer      = (*GitLabIntegration)(nil)
	_ ItemRemover      = (*GitLabIntegration)(nil)
	_ AccountLookup    = (*GitLabIntegration)(nil)
	_ RepositoryLister = (*GitLabIntegration)(nil)
)

// syncClient é o cliente da sincronização: não segue redirecionamento, pelo mesmo motivo do GitHub (o
// net/http trocaria o PUT por um GET ao seguir um e devolveria "200" sem ter gravado nada).
func (g *GitLabIntegration) syncClient() *http.Client {
	c := *g.client()
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &c
}

// call faz uma requisição. target é um caminho da API ("/projects/...") ou, para seguir a paginação, o
// endereço completo que o GitLab mandou, que precisa ser do mesmo servidor.
func (g *GitLabIntegration) call(ctx context.Context, conn Connection, method, target string, body any) (*http.Response, error) {
	token, err := gitlabDescriptor.token(conn)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(target, "http") {
		if !g.sameServer(target) {
			return nil, ErrUnexpectedResponse.With("provider", "GitLab")
		}
	} else {
		target = g.baseURL() + target
	}
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, payload)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "working-time-tracker")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := g.syncClient().Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrProviderUnreachable.With("provider", "GitLab").Wrap(err)
	}
	return resp, nil
}

// sameServer diz se o endereço é do servidor da API: o token só viaja para ele.
func (g *GitLabIntegration) sameServer(target string) bool {
	base, err := url.Parse(g.baseURL())
	if err != nil {
		return false
	}
	u, err := url.Parse(target)
	return err == nil && u.Scheme == base.Scheme && u.Host == base.Host
}

// gitlabCheck devolve o erro de uma resposta que não é de sucesso e fecha o corpo; nil deixa a resposta
// aberta para quem chamou ler. missing é o erro de um 404 (o que não existe depende do que se pediu).
func gitlabCheck(resp *http.Response, missing error) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	defer closeBody(resp)
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return ErrInvalidToken.With("provider", "GitLab")
	case http.StatusTooManyRequests:
		return gitlabRateLimited(resp)
	case http.StatusForbidden:
		if resp.Header.Get("RateLimit-Remaining") == "0" {
			return gitlabRateLimited(resp)
		}
		return ErrForbidden.With("provider", "GitLab")
	case http.StatusNotFound:
		if missing != nil {
			return missing
		}
	}
	return ErrProviderStatus.With("provider", "GitLab", "status", resp.StatusCode)
}

// gitlabRateLimited monta o erro do limite com o instante em que ele acaba, se o GitLab o disse.
func gitlabRateLimited(resp *http.Response) error {
	until := time.Now().Add(time.Minute)
	if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 {
		until = time.Now().Add(time.Duration(s) * time.Second)
	} else if reset, err := strconv.ParseInt(resp.Header.Get("RateLimit-Reset"), 10, 64); err == nil && reset > 0 {
		until = time.Unix(reset, 0)
	}
	return ErrRateLimited.With("provider", "GitLab", "until", until.Unix())
}

// gitlabRemaining lê quantas requisições ainda cabem no limite; -1 se o GitLab não disse.
func gitlabRemaining(resp *http.Response) int {
	n, err := strconv.Atoi(resp.Header.Get("RateLimit-Remaining"))
	if err != nil {
		return -1
	}
	return n
}

// gitlabDecode lê o corpo JSON e o fecha.
func gitlabDecode(resp *http.Response, into any) error {
	defer closeBody(resp)
	if err := json.NewDecoder(io.LimitReader(resp.Body, gitlabBodyLimit)).Decode(into); err != nil {
		return ErrUnexpectedResponse.With("provider", "GitLab").Wrap(err)
	}
	return nil
}

// gitlabState traduz o estado do GitLab para o da sincronização: "opened" é "open".
func gitlabState(state string) string {
	if state == "opened" {
		return "open"
	}
	return state
}

// gitlabUser é o usuário como a API o devolve nas issues e nas buscas.
type gitlabUser struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

// gitlabIssue é a issue como a API a devolve.
type gitlabIssue struct {
	IID         int          `json:"iid"`
	Title       string       `json:"title"`
	Description *string      `json:"description"`
	State       string       `json:"state"`
	WebURL      string       `json:"web_url"`
	UpdatedAt   string       `json:"updated_at"`
	Labels      []string     `json:"labels"`
	Assignees   []gitlabUser `json:"assignees"`
	// MovedToID é a issue que foi transferida para outro projeto: ela fica no projeto antigo, fechada.
	MovedToID *int `json:"moved_to_id"`
}

func (i gitlabIssue) toIssue() Issue {
	out := Issue{
		ID:        strconv.Itoa(i.IID),
		Number:    i.IID,
		Title:     i.Title,
		State:     gitlabState(i.State),
		URL:       i.WebURL,
		Labels:    []string{},
		Assignees: []string{},
	}
	if i.Description != nil {
		out.Body = *i.Description
	}
	if t, err := time.Parse(time.RFC3339, i.UpdatedAt); err == nil {
		out.UpdatedAt = t
	}
	out.Labels = append(out.Labels, i.Labels...)
	for _, a := range i.Assignees {
		out.Assignees = append(out.Assignees, a.Username)
	}
	return out
}

func (g *GitLabIntegration) projectPath(conn Connection) (string, error) {
	meta, err := parseGitlabMetadata(conn.Metadata)
	if err != nil {
		return "", err
	}
	return "/projects/" + url.PathEscape(meta.ProjectURL), nil
}

// gitlabProjectBody é o projeto como a API o devolve.
type gitlabProjectBody struct {
	PathWithNamespace string `json:"path_with_namespace"`
	Archived          bool   `json:"archived"`
	IssuesEnabled     *bool  `json:"issues_enabled"`
}

func (g *GitLabIntegration) project(ctx context.Context, conn Connection) (*gitlabProjectBody, error) {
	path, err := g.projectPath(conn)
	if err != nil {
		return nil, err
	}
	resp, err := g.call(ctx, conn, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	if err := gitlabCheck(resp, ErrGitLabProjectMissing); err != nil {
		return nil, err
	}
	var body gitlabProjectBody
	if err := gitlabDecode(resp, &body); err != nil {
		return nil, err
	}
	return &body, nil
}

// Repo lê o projeto e o nível de acesso da conta nele. O nível vem de members/all, que soma o que a conta
// herda dos grupos acima do projeto: o "permissions" do próprio projeto só mostra o grupo direto.
func (g *GitLabIntegration) Repo(ctx context.Context, conn Connection) (*IssueRepo, error) {
	project, err := g.project(ctx, conn)
	if err != nil {
		return nil, err
	}
	level, err := g.accessLevel(ctx, conn)
	if err != nil {
		return nil, err
	}
	return &IssueRepo{
		FullName: project.PathWithNamespace,
		Archived: project.Archived,
		CanPush:  level >= gitlabCanWrite,
	}, nil
}

// accessLevel é o nível de acesso efetivo da conta no projeto; zero é quem não é membro (e só lê um
// projeto público).
func (g *GitLabIntegration) accessLevel(ctx context.Context, conn Connection) (int, error) {
	path, err := g.projectPath(conn)
	if err != nil {
		return 0, err
	}
	resp, err := g.call(ctx, conn, "GET", "/user", nil)
	if err != nil {
		return 0, err
	}
	if err := gitlabCheck(resp, nil); err != nil {
		return 0, err
	}
	var me gitlabUser
	if err := gitlabDecode(resp, &me); err != nil {
		return 0, err
	}
	resp, err = g.call(ctx, conn, "GET", path+"/members/all/"+strconv.Itoa(me.ID), nil)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode == http.StatusNotFound {
		closeBody(resp)
		return 0, nil
	}
	if err := gitlabCheck(resp, nil); err != nil {
		return 0, err
	}
	var member struct {
		AccessLevel int `json:"access_level"`
	}
	if err := gitlabDecode(resp, &member); err != nil {
		return 0, err
	}
	return member.AccessLevel, nil
}

func (g *GitLabIntegration) ListIssues(ctx context.Context, conn Connection, opts ListIssuesOptions) (*IssueList, error) {
	path, err := g.projectPath(conn)
	if err != nil {
		return nil, err
	}
	state := opts.State
	switch state {
	case "", "open":
		state = "opened"
	}
	// Só issues: os incidentes, os casos de teste e as tarefas filhas também moram nesta lista, como os
	// pull requests na do GitHub.
	q := url.Values{
		"state": {state}, "per_page": {"100"}, "sort": {"asc"}, "order_by": {"created_at"}, "issue_type": {"issue"},
	}
	if opts.ByUpdated {
		q.Set("order_by", "updated_at")
	}
	if !opts.Since.IsZero() {
		q.Set("updated_after", opts.Since.UTC().Format(time.RFC3339))
	}

	list := &IssueList{ServerTime: time.Now()}
	target := path + "/issues?" + q.Encode()
	for page := 0; target != ""; page++ {
		if page >= gitlabMaxPages {
			return list, ErrListTooLong.With("provider", "GitLab")
		}
		resp, err := g.call(ctx, conn, "GET", target, nil)
		if err != nil {
			return list, err
		}
		if err := gitlabCheck(resp, ErrGitLabProjectMissing); err != nil {
			return list, err
		}
		if page == 0 {
			if t, err := http.ParseTime(resp.Header.Get("Date")); err == nil {
				list.ServerTime = t
			}
		}
		var rows []gitlabIssue
		left := gitlabRemaining(resp)
		next := ""
		if m := linkNext.FindStringSubmatch(resp.Header.Get("Link")); m != nil {
			next = m[1]
		}
		if err := gitlabDecode(resp, &rows); err != nil {
			return list, err
		}
		for _, row := range rows {
			// A transferida não é mais deste projeto: a rodada completa a acha sumida pelo GetIssue.
			if row.MovedToID == nil {
				list.Issues = append(list.Issues, row.toIssue())
			}
		}
		target = next
		if target != "" && left >= 0 && left < gitlabRateReserve {
			reset := time.Now().Add(time.Minute)
			return list, ErrRateLimited.With("provider", "GitLab", "until", reset.Unix())
		}
	}
	return list, nil
}

// readIssue lê a issue pelo iid. A transferida para outro projeto conta como sumida.
func (g *GitLabIntegration) readIssue(ctx context.Context, conn Connection, path, n string) (*gitlabIssue, error) {
	resp, err := g.call(ctx, conn, "GET", path+"/issues/"+n, nil)
	if err != nil {
		return nil, err
	}
	if err := gitlabCheck(resp, ErrIssueGone.With("item", n)); err != nil {
		return nil, err
	}
	var row gitlabIssue
	if err := gitlabDecode(resp, &row); err != nil {
		return nil, err
	}
	if row.MovedToID != nil {
		return nil, ErrIssueGone.With("item", n)
	}
	return &row, nil
}

func (g *GitLabIntegration) GetIssue(ctx context.Context, conn Connection, id string) (*Issue, error) {
	path, err := g.projectPath(conn)
	if err != nil {
		return nil, err
	}
	n, err := issueNumber(id)
	if err != nil {
		return nil, err
	}
	row, err := g.readIssue(ctx, conn, path, n)
	if err != nil {
		return nil, err
	}
	issue := row.toIssue()
	return &issue, nil
}

// userIDs troca os usernames pelos ids que o GitLab pede para pôr um responsável. O username que o GitLab
// não tem fica de fora, como o GitHub descarta o login que não pode ser designado.
func (g *GitLabIntegration) userIDs(ctx context.Context, conn Connection, usernames []string) ([]int, error) {
	ids := []int{}
	for _, username := range usernames {
		user, err := g.userByUsername(ctx, conn, username)
		if err != nil {
			return nil, err
		}
		if user != nil {
			ids = append(ids, user.ID)
		}
	}
	return ids, nil
}

func (g *GitLabIntegration) userByUsername(ctx context.Context, conn Connection, username string) (*gitlabUser, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, nil
	}
	resp, err := g.call(ctx, conn, "GET", "/users?"+url.Values{"username": {username}}.Encode(), nil)
	if err != nil {
		return nil, err
	}
	if err := gitlabCheck(resp, nil); err != nil {
		return nil, err
	}
	var rows []gitlabUser
	if err := gitlabDecode(resp, &rows); err != nil {
		return nil, err
	}
	for _, u := range rows {
		if strings.EqualFold(u.Username, username) {
			return &u, nil
		}
	}
	return nil, nil
}

func (g *GitLabIntegration) UpdateIssue(ctx context.Context, conn Connection, id string, patch IssuePatch) (*Issue, error) {
	path, err := g.projectPath(conn)
	if err != nil {
		return nil, err
	}
	n, err := issueNumber(id)
	if err != nil {
		return nil, err
	}
	body := map[string]any{}
	if patch.Title != nil {
		body["title"] = *patch.Title
	}
	if patch.Body != nil {
		body["description"] = *patch.Body
	}
	// O GitLab não tem o motivo do fechamento: StateReason fica de fora.
	if patch.State != nil {
		switch *patch.State {
		case "closed":
			body["state_event"] = "close"
		case "open":
			body["state_event"] = "reopen"
		}
	}
	if patch.Labels != nil {
		body["labels"] = strings.Join(*patch.Labels, ",")
	}
	if patch.Assignees != nil {
		ids, err := g.userIDs(ctx, conn, *patch.Assignees)
		if err != nil {
			return nil, err
		}
		body["assignee_ids"] = ids
	}
	resp, err := g.call(ctx, conn, "PUT", path+"/issues/"+n, body)
	if err != nil {
		return nil, err
	}
	if err := gitlabCheck(resp, ErrIssueGone.With("item", n)); err != nil {
		return nil, err
	}
	var row gitlabIssue
	if err := gitlabDecode(resp, &row); err != nil {
		return nil, err
	}
	issue := row.toIssue()
	return &issue, nil
}

// CreateIssue abre uma issue no projeto. Como no GitHub, quem não escreve no projeto (Guest) tem as etiquetas
// e os responsáveis descartados sem aviso, e quem chama vê o que sobrou na issue devolvida.
func (g *GitLabIntegration) CreateIssue(ctx context.Context, conn Connection, in NewIssue) (*Issue, error) {
	path, err := g.projectPath(conn)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"title": in.Title, "description": in.Body}
	if len(in.Labels) > 0 {
		body["labels"] = strings.Join(in.Labels, ",")
	}
	if len(in.Assignees) > 0 {
		ids, err := g.userIDs(ctx, conn, in.Assignees)
		if err != nil {
			return nil, err
		}
		if len(ids) > 0 {
			body["assignee_ids"] = ids
		}
	}
	resp, err := g.call(ctx, conn, "POST", path+"/issues", body)
	if err != nil {
		return nil, err
	}
	// O projeto com as issues desligadas responde 403 (ou 404); é o issues_enabled dele que diz qual é o caso.
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
		refused := gitlabCheck(resp, ErrGitLabProjectMissing)
		if errors.Is(refused, ErrForbidden) || errors.Is(refused, ErrGitLabProjectMissing) {
			if project, err := g.project(ctx, conn); err == nil && project.IssuesEnabled != nil && !*project.IssuesEnabled {
				return nil, ErrIssuesDisabled.With("provider", "GitLab")
			}
		}
		return nil, refused
	}
	if err := gitlabCheck(resp, ErrGitLabProjectMissing); err != nil {
		return nil, err
	}
	var row gitlabIssue
	if err := gitlabDecode(resp, &row); err != nil {
		return nil, err
	}
	issue := row.toIssue()
	return &issue, nil
}

// RemoveItem apaga a issue. No GitLab só a conta Owner do projeto apaga: com 403, a issue é fechada e o
// resultado é RemoveClosed, para quem chamou avisar que ela continua lá.
func (g *GitLabIntegration) RemoveItem(ctx context.Context, conn Connection, id string) (RemoveOutcome, error) {
	path, err := g.projectPath(conn)
	if err != nil {
		return "", err
	}
	n, err := issueNumber(id)
	if err != nil {
		return "", err
	}
	row, err := g.readIssue(ctx, conn, path, n)
	if err != nil {
		if errors.Is(err, ErrIssueGone) {
			return RemoveGone, nil
		}
		return "", err
	}

	resp, err := g.call(ctx, conn, "DELETE", path+"/issues/"+n, nil)
	if err != nil {
		return "", err
	}
	switch err := gitlabCheck(resp, ErrIssueGone.With("item", n)); {
	case err == nil:
		closeBody(resp)
		return RemoveDeleted, nil
	case errors.Is(err, ErrIssueGone):
		return RemoveGone, nil
	case !errors.Is(err, ErrForbidden):
		return "", err
	}
	// Sem permissão para apagar: o mais perto disso que o token faz é fechar a issue.
	if row.State != "closed" {
		closed := "closed"
		if _, err := g.UpdateIssue(ctx, conn, id, IssuePatch{State: &closed}); err != nil {
			if errors.Is(err, ErrIssueGone) {
				return RemoveGone, nil
			}
			return "", err
		}
	}
	return RemoveClosed, nil
}

func (g *GitLabIntegration) ListLabels(ctx context.Context, conn Connection) ([]string, error) {
	path, err := g.projectPath(conn)
	if err != nil {
		return nil, err
	}
	var names []string
	target := path + "/labels?per_page=100"
	for page := 0; target != ""; page++ {
		if page >= gitlabMaxPages {
			return names, ErrListTooLong.With("provider", "GitLab")
		}
		resp, err := g.call(ctx, conn, "GET", target, nil)
		if err != nil {
			return names, err
		}
		if err := gitlabCheck(resp, ErrGitLabProjectMissing); err != nil {
			return names, err
		}
		next := ""
		if m := linkNext.FindStringSubmatch(resp.Header.Get("Link")); m != nil {
			next = m[1]
		}
		var rows []struct {
			Name string `json:"name"`
		}
		if err := gitlabDecode(resp, &rows); err != nil {
			return names, err
		}
		for _, r := range rows {
			names = append(names, r.Name)
		}
		target = next
	}
	return names, nil
}

func (g *GitLabIntegration) CreateLabel(ctx context.Context, conn Connection, name string) error {
	path, err := g.projectPath(conn)
	if err != nil {
		return err
	}
	resp, err := g.call(ctx, conn, "POST", path+"/labels", map[string]string{"name": name, "color": "#ededed"})
	if err != nil {
		return err
	}
	// 409 é a etiqueta que já existe (criada por outra rodada, ou com outra caixa): serve.
	if resp.StatusCode == http.StatusConflict {
		closeBody(resp)
		return nil
	}
	if err := gitlabCheck(resp, ErrGitLabProjectMissing); err != nil {
		return err
	}
	closeBody(resp)
	return nil
}

// publicEmail lê o e-mail público do usuário pelo id; vazio se ele não publica um.
func (g *GitLabIntegration) publicEmail(ctx context.Context, conn Connection, id int) (string, error) {
	resp, err := g.call(ctx, conn, "GET", "/users/"+strconv.Itoa(id), nil)
	if err != nil {
		return "", err
	}
	if resp.StatusCode == http.StatusNotFound {
		closeBody(resp)
		return "", nil
	}
	if err := gitlabCheck(resp, nil); err != nil {
		return "", err
	}
	var body struct {
		PublicEmail *string `json:"public_email"`
	}
	if err := gitlabDecode(resp, &body); err != nil {
		return "", err
	}
	if body.PublicEmail == nil {
		return "", nil
	}
	return strings.TrimSpace(*body.PublicEmail), nil
}

func (g *GitLabIntegration) UserEmail(ctx context.Context, conn Connection, login string) (string, error) {
	user, err := g.userByUsername(ctx, conn, login)
	if err != nil || user == nil {
		return "", err
	}
	return g.publicEmail(ctx, conn, user.ID)
}

// FindLoginByEmail busca o e-mail entre os usuários. A busca do GitLab também casa o nome e o username, então
// cada candidato tem o e-mail público conferido; só vale se exatamente um o publica.
func (g *GitLabIntegration) FindLoginByEmail(ctx context.Context, conn Connection, email string) (string, error) {
	email = strings.TrimSpace(email)
	if email == "" || strings.ContainsAny(email, " \t\r\n") {
		return "", nil
	}
	q := url.Values{"search": {email}, "per_page": {strconv.Itoa(gitlabEmailCandidates)}}
	resp, err := g.call(ctx, conn, "GET", "/users?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	if err := gitlabCheck(resp, nil); err != nil {
		return "", err
	}
	var rows []gitlabUser
	if err := gitlabDecode(resp, &rows); err != nil {
		return "", err
	}
	found := ""
	for _, u := range rows {
		public, err := g.publicEmail(ctx, conn, u.ID)
		if err != nil {
			return "", err
		}
		if !strings.EqualFold(public, email) {
			continue
		}
		// Com dois, escolher um seria adivinhar.
		if found != "" {
			return "", nil
		}
		found = u.Username
	}
	return found, nil
}
