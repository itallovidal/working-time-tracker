package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"working-time-tracker/internal/apperr"
)

// A leitura e a escrita das issues, para a sincronização com as tarefas (IssueSyncer).

const (
	// githubAPIVersion fixa a versão da API: sem ela o GitHub usa a padrão, que pode mudar.
	githubAPIVersion = "2022-11-28"
	// githubMaxPages é o teto de páginas de uma listagem (100 issues por página).
	githubMaxPages = 100
	// githubRateReserve é o que sobra do limite de requisições quando a listagem para: o resto fica
	// para o que o usuário fizer pelo próprio GitHub com o mesmo token.
	githubRateReserve = 25
	// githubBodyLimit é quanto de uma resposta se lê.
	githubBodyLimit = 16 << 20
)

var (
	_ IssueSyncer = (*GitHubIntegration)(nil)
	_ ItemRemover = (*GitHubIntegration)(nil)
)

// syncClient é o cliente da sincronização: não segue redirecionamento. O net/http transformaria o
// PATCH em GET ao seguir um 301 e devolveria "200" sem ter gravado nada; e uma issue transferida
// responde 301 justamente para dizer que o endereço mudou.
func (g *GitHubIntegration) syncClient() *http.Client {
	c := *g.client()
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &c
}

// call faz uma requisição. target é um caminho da API ("/repos/...") ou, para seguir a paginação, o
// endereço completo que o GitHub mandou, que precisa ser do mesmo servidor.
func (g *GitHubIntegration) call(ctx context.Context, conn Connection, method, target string, body any) (*http.Response, error) {
	token, err := githubDescriptor.token(conn)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(target, "http") {
		if !g.sameServer(target) {
			return nil, ErrUnexpectedResponse.With("provider", "GitHub")
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
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
	req.Header.Set("User-Agent", "working-time-tracker")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := g.syncClient().Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrProviderUnreachable.With("provider", "GitHub").Wrap(err)
	}
	return resp, nil
}

// sameServer diz se o endereço é do servidor da API: o token só viaja para ele.
func (g *GitHubIntegration) sameServer(target string) bool {
	base, err := url.Parse(g.baseURL())
	if err != nil {
		return false
	}
	u, err := url.Parse(target)
	return err == nil && u.Scheme == base.Scheme && u.Host == base.Host
}

// check devolve o erro de uma resposta que não é de sucesso e fecha o corpo; nil deixa a resposta
// aberta para quem chamou ler. missing é o erro de um 404 (o que não existe depende do que se pediu).
func check(resp *http.Response, missing error) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	defer resp.Body.Close()
	message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return ErrInvalidToken.With("provider", "GitHub")
	case http.StatusTooManyRequests:
		return rateLimited(resp)
	case http.StatusForbidden:
		if resp.Header.Get("X-RateLimit-Remaining") == "0" || resp.Header.Get("Retry-After") != "" ||
			strings.Contains(strings.ToLower(string(message)), "rate limit") {
			return rateLimited(resp)
		}
		return ErrForbidden.With("provider", "GitHub")
	case http.StatusMovedPermanently, http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect,
		http.StatusNotFound, http.StatusGone:
		if missing != nil {
			return missing
		}
	}
	return ErrProviderStatus.With("provider", "GitHub", "status", resp.StatusCode)
}

// rateLimited monta o erro do limite com o instante em que ele acaba, se o GitHub o disse.
func rateLimited(resp *http.Response) error {
	until := time.Now().Add(time.Minute)
	if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 {
		until = time.Now().Add(time.Duration(s) * time.Second)
	} else if reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil && reset > 0 {
		until = time.Unix(reset, 0)
	}
	return ErrRateLimited.With("provider", "GitHub", "until", until.Unix())
}

// RateLimitedUntil diz até quando o erro pede para esperar, se ele é de limite de requisições.
func RateLimitedUntil(err error) (time.Time, bool) {
	var e *apperr.Error
	if !errors.As(err, &e) || e.Code != ErrRateLimited.Code {
		return time.Time{}, false
	}
	if until, ok := e.Params["until"].(int64); ok {
		return time.Unix(until, 0), true
	}
	return time.Now().Add(time.Minute), true
}

// remaining lê quantas requisições ainda cabem no limite; -1 se o GitHub não disse.
func remaining(resp *http.Response) int {
	n, err := strconv.Atoi(resp.Header.Get("X-RateLimit-Remaining"))
	if err != nil {
		return -1
	}
	return n
}

// decode lê o corpo JSON e o fecha.
func decode(resp *http.Response, into any) error {
	defer resp.Body.Close()
	if err := json.NewDecoder(io.LimitReader(resp.Body, githubBodyLimit)).Decode(into); err != nil {
		return ErrUnexpectedResponse.With("provider", "GitHub").Wrap(err)
	}
	return nil
}

// githubIssue é a issue como a API a devolve.
type githubIssue struct {
	// NodeID é o id da issue no GraphQL, o único jeito de apagá-la.
	NodeID      string  `json:"node_id"`
	Number      int     `json:"number"`
	Title       string  `json:"title"`
	Body        *string `json:"body"`
	State       string  `json:"state"`
	StateReason *string `json:"state_reason"`
	HTMLURL     string  `json:"html_url"`
	UpdatedAt   string  `json:"updated_at"`
	Labels      []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Assignees []struct {
		Login string `json:"login"`
	} `json:"assignees"`
	PullRequest *json.RawMessage `json:"pull_request"`
}

func (i githubIssue) toIssue() Issue {
	out := Issue{
		ID:          strconv.Itoa(i.Number),
		Number:      i.Number,
		Title:       i.Title,
		State:       i.State,
		URL:         i.HTMLURL,
		PullRequest: i.PullRequest != nil,
		Labels:      []string{},
		Assignees:   []string{},
	}
	if i.Body != nil {
		out.Body = *i.Body
	}
	if i.StateReason != nil {
		out.StateReason = *i.StateReason
	}
	if t, err := time.Parse(time.RFC3339, i.UpdatedAt); err == nil {
		out.UpdatedAt = t
	}
	for _, l := range i.Labels {
		out.Labels = append(out.Labels, l.Name)
	}
	for _, a := range i.Assignees {
		out.Assignees = append(out.Assignees, a.Login)
	}
	return out
}

func (g *GitHubIntegration) repoPath(conn Connection) (string, error) {
	meta, err := parseGithubMetadata(conn.Metadata)
	if err != nil {
		return "", err
	}
	return "/repos/" + meta.Repo, nil
}

func (g *GitHubIntegration) Repo(ctx context.Context, conn Connection) (*IssueRepo, error) {
	path, err := g.repoPath(conn)
	if err != nil {
		return nil, err
	}
	resp, err := g.call(ctx, conn, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	if err := check(resp, ErrGitHubRepoNotFound); err != nil {
		return nil, err
	}
	var body struct {
		FullName    string `json:"full_name"`
		Archived    bool   `json:"archived"`
		Permissions struct {
			Admin    bool `json:"admin"`
			Maintain bool `json:"maintain"`
			Push     bool `json:"push"`
		} `json:"permissions"`
	}
	if err := decode(resp, &body); err != nil {
		return nil, err
	}
	return &IssueRepo{
		FullName: body.FullName,
		Archived: body.Archived,
		CanPush:  body.Permissions.Push || body.Permissions.Maintain || body.Permissions.Admin,
	}, nil
}

var linkNext = regexp.MustCompile(`<([^>]+)>\s*;\s*rel="next"`)

func (g *GitHubIntegration) ListIssues(ctx context.Context, conn Connection, opts ListIssuesOptions) (*IssueList, error) {
	path, err := g.repoPath(conn)
	if err != nil {
		return nil, err
	}
	state := opts.State
	if state == "" {
		state = "open"
	}
	q := url.Values{"state": {state}, "per_page": {"100"}, "direction": {"asc"}, "sort": {"created"}}
	if opts.ByUpdated {
		q.Set("sort", "updated")
	}
	if !opts.Since.IsZero() {
		q.Set("since", opts.Since.UTC().Format(time.RFC3339))
	}

	list := &IssueList{ServerTime: time.Now()}
	target := path + "/issues?" + q.Encode()
	for page := 0; target != ""; page++ {
		if page >= githubMaxPages {
			return list, ErrListTooLong.With("provider", "GitHub")
		}
		resp, err := g.call(ctx, conn, "GET", target, nil)
		if err != nil {
			return list, err
		}
		if err := check(resp, ErrGitHubRepoNotFound); err != nil {
			return list, err
		}
		if page == 0 {
			if t, err := http.ParseTime(resp.Header.Get("Date")); err == nil {
				list.ServerTime = t
			}
		}
		var rows []githubIssue
		left := remaining(resp)
		next := ""
		if m := linkNext.FindStringSubmatch(resp.Header.Get("Link")); m != nil {
			next = m[1]
		}
		if err := decode(resp, &rows); err != nil {
			return list, err
		}
		for _, row := range rows {
			if row.PullRequest == nil {
				list.Issues = append(list.Issues, row.toIssue())
			}
		}
		target = next
		if target != "" && left >= 0 && left < githubRateReserve {
			reset := time.Now().Add(time.Minute)
			return list, ErrRateLimited.With("provider", "GitHub", "until", reset.Unix())
		}
	}
	return list, nil
}

func (g *GitHubIntegration) GetIssue(ctx context.Context, conn Connection, id string) (*Issue, error) {
	path, err := g.repoPath(conn)
	if err != nil {
		return nil, err
	}
	n, err := issueNumber(id)
	if err != nil {
		return nil, err
	}
	resp, err := g.call(ctx, conn, "GET", path+"/issues/"+n, nil)
	if err != nil {
		return nil, err
	}
	if err := check(resp, ErrIssueGone.With("item", n)); err != nil {
		return nil, err
	}
	var row githubIssue
	if err := decode(resp, &row); err != nil {
		return nil, err
	}
	issue := row.toIssue()
	return &issue, nil
}

func (g *GitHubIntegration) UpdateIssue(ctx context.Context, conn Connection, id string, patch IssuePatch) (*Issue, error) {
	path, err := g.repoPath(conn)
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
		body["body"] = *patch.Body
	}
	if patch.State != nil {
		body["state"] = *patch.State
	}
	if patch.StateReason != nil {
		body["state_reason"] = *patch.StateReason
	}
	if patch.Labels != nil {
		body["labels"] = nonNil(*patch.Labels)
	}
	if patch.Assignees != nil {
		body["assignees"] = nonNil(*patch.Assignees)
	}
	resp, err := g.call(ctx, conn, "PATCH", path+"/issues/"+n, body)
	if err != nil {
		return nil, err
	}
	if err := check(resp, ErrIssueGone.With("item", n)); err != nil {
		return nil, err
	}
	var row githubIssue
	if err := decode(resp, &row); err != nil {
		return nil, err
	}
	issue := row.toIssue()
	return &issue, nil
}

// CreateIssue abre uma issue no repositório. O GitHub só grava as etiquetas que o repositório tem e os
// responsáveis que podem ser designados (e só para quem tem permissão de escrita): o que ele descarta
// não é erro, e quem chama vê o que sobrou na issue devolvida.
func (g *GitHubIntegration) CreateIssue(ctx context.Context, conn Connection, in NewIssue) (*Issue, error) {
	path, err := g.repoPath(conn)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"title": in.Title, "body": in.Body}
	if len(in.Labels) > 0 {
		body["labels"] = in.Labels
	}
	if len(in.Assignees) > 0 {
		body["assignees"] = in.Assignees
	}
	resp, err := g.call(ctx, conn, "POST", path+"/issues", body)
	if err != nil {
		return nil, err
	}
	// 410 é o repositório com as issues desligadas; o check trataria como "não achei".
	if resp.StatusCode == http.StatusGone {
		resp.Body.Close()
		return nil, ErrIssuesDisabled.With("provider", "GitHub")
	}
	if err := check(resp, ErrGitHubRepoNotFound); err != nil {
		return nil, err
	}
	var row githubIssue
	if err := decode(resp, &row); err != nil {
		return nil, err
	}
	issue := row.toIssue()
	return &issue, nil
}

// RemoveItem apaga a issue. A API comum do GitHub não apaga issue: só o GraphQL (deleteIssue), e só para quem é
// admin do repositório. Se ele recusar por permissão, a issue é fechada como "não planejada" e o resultado é
// RemoveClosed, para quem chamou avisar que ela continua lá.
func (g *GitHubIntegration) RemoveItem(ctx context.Context, conn Connection, id string) (RemoveOutcome, error) {
	path, err := g.repoPath(conn)
	if err != nil {
		return "", err
	}
	n, err := issueNumber(id)
	if err != nil {
		return "", err
	}
	resp, err := g.call(ctx, conn, "GET", path+"/issues/"+n, nil)
	if err != nil {
		return "", err
	}
	if err := check(resp, ErrIssueGone.With("item", n)); err != nil {
		if errors.Is(err, ErrIssueGone) {
			return RemoveGone, nil
		}
		return "", err
	}
	var row githubIssue
	if err := decode(resp, &row); err != nil {
		return "", err
	}

	deleted, err := g.deleteIssue(ctx, conn, row.NodeID)
	if err != nil {
		return "", err
	}
	if deleted {
		return RemoveDeleted, nil
	}
	// Sem permissão para apagar: o mais perto disso que o token faz é fechar a issue.
	if row.State != "closed" {
		closed, reason := "closed", "not_planned"
		if _, err := g.UpdateIssue(ctx, conn, id, IssuePatch{State: &closed, StateReason: &reason}); err != nil {
			if errors.Is(err, ErrIssueGone) {
				return RemoveGone, nil
			}
			return "", err
		}
	}
	return RemoveClosed, nil
}

// graphqlURL é o endereço do GraphQL do mesmo servidor da API: api.github.com/graphql no GitHub, e
// <servidor>/api/graphql no Enterprise (onde a API fica em <servidor>/api/v3).
func (g *GitHubIntegration) graphqlURL() string {
	base := g.baseURL()
	if strings.HasSuffix(base, "/api/v3") {
		return strings.TrimSuffix(base, "/v3") + "/graphql"
	}
	return base + "/graphql"
}

// deleteIssue apaga a issue pelo GraphQL. Devolve false, sem erro, quando o GitHub não deixa a conta fazer
// isso (falta de permissão), para quem chamou cair no plano B.
func (g *GitHubIntegration) deleteIssue(ctx context.Context, conn Connection, nodeID string) (bool, error) {
	if nodeID == "" {
		return false, ErrUnexpectedResponse.With("provider", "GitHub")
	}
	body := map[string]any{
		"query":     "mutation($id: ID!) { deleteIssue(input: {issueId: $id}) { clientMutationId } }",
		"variables": map[string]any{"id": nodeID},
	}
	resp, err := g.call(ctx, conn, "POST", g.graphqlURL(), body)
	if err != nil {
		return false, err
	}
	if err := check(resp, nil); err != nil {
		if errors.Is(err, ErrForbidden) {
			return false, nil
		}
		return false, err
	}
	var out struct {
		Errors []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := decode(resp, &out); err != nil {
		return false, err
	}
	if len(out.Errors) == 0 {
		return true, nil
	}
	for _, e := range out.Errors {
		// NOT_FOUND vem de quem não enxerga o que apagar; se a issue sumiu agora há pouco, o fechamento acha isso.
		switch strings.ToUpper(e.Type) {
		case "FORBIDDEN", "INSUFFICIENT_SCOPES", "NOT_FOUND":
			return false, nil
		}
	}
	return false, ErrUnexpectedResponse.With("provider", "GitHub")
}

// nonNil troca a lista nula por uma vazia: o JSON "null" não esvazia as etiquetas, o "[]" sim.
func nonNil(list []string) []string {
	if list == nil {
		return []string{}
	}
	return list
}

func (g *GitHubIntegration) ListLabels(ctx context.Context, conn Connection) ([]string, error) {
	path, err := g.repoPath(conn)
	if err != nil {
		return nil, err
	}
	var names []string
	target := path + "/labels?per_page=100"
	for page := 0; target != ""; page++ {
		if page >= githubMaxPages {
			return names, ErrListTooLong.With("provider", "GitHub")
		}
		resp, err := g.call(ctx, conn, "GET", target, nil)
		if err != nil {
			return names, err
		}
		if err := check(resp, ErrGitHubRepoNotFound); err != nil {
			return names, err
		}
		next := ""
		if m := linkNext.FindStringSubmatch(resp.Header.Get("Link")); m != nil {
			next = m[1]
		}
		var rows []struct {
			Name string `json:"name"`
		}
		if err := decode(resp, &rows); err != nil {
			return names, err
		}
		for _, r := range rows {
			names = append(names, r.Name)
		}
		target = next
	}
	return names, nil
}

func (g *GitHubIntegration) CreateLabel(ctx context.Context, conn Connection, name string) error {
	path, err := g.repoPath(conn)
	if err != nil {
		return err
	}
	resp, err := g.call(ctx, conn, "POST", path+"/labels", map[string]string{"name": name, "color": "ededed"})
	if err != nil {
		return err
	}
	// 422 é a etiqueta que já existe (criada por outra rodada, ou com outra caixa): serve.
	if resp.StatusCode == http.StatusUnprocessableEntity {
		resp.Body.Close()
		return nil
	}
	if err := check(resp, ErrGitHubRepoNotFound); err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// githubLogin é o formato de um login: o que entra na URL da API precisa ter só isso.
var githubLogin = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)

func (g *GitHubIntegration) UserEmail(ctx context.Context, conn Connection, login string) (string, error) {
	if !githubLogin.MatchString(login) {
		return "", nil
	}
	resp, err := g.call(ctx, conn, "GET", "/users/"+login, nil)
	if err != nil {
		return "", err
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return "", nil
	}
	if err := check(resp, nil); err != nil {
		return "", err
	}
	var body struct {
		Email *string `json:"email"`
	}
	if err := decode(resp, &body); err != nil {
		return "", err
	}
	if body.Email == nil {
		return "", nil
	}
	return strings.TrimSpace(*body.Email), nil
}

func (g *GitHubIntegration) FindLoginByEmail(ctx context.Context, conn Connection, email string) (string, error) {
	email = strings.TrimSpace(email)
	if email == "" || strings.ContainsAny(email, " \t\r\n") {
		return "", nil
	}
	q := url.Values{"q": {email + " in:email"}, "per_page": {"2"}}
	resp, err := g.call(ctx, conn, "GET", "/search/users?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	if err := check(resp, nil); err != nil {
		return "", err
	}
	var body struct {
		Total int `json:"total_count"`
		Items []struct {
			Login string `json:"login"`
		} `json:"items"`
	}
	if err := decode(resp, &body); err != nil {
		return "", err
	}
	// Só vale se o e-mail aponta para um único usuário: com dois, escolher um seria adivinhar.
	if body.Total != 1 || len(body.Items) != 1 {
		return "", nil
	}
	return body.Items[0].Login, nil
}
