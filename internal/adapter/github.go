package adapter

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const defaultGitHubBaseURL = "https://api.github.com"

var githubDescriptor = Descriptor{
	Type:  "github",
	Label: "GitHub",
	Metadata: []Field{
		{Key: "repo", Required: true, Summary: true, Picker: "datalist"},
	},
	ItemNumeric: true,
	Sync:        true,
	// O GitHub liga o responsável pelo e-mail público e filtra a listagem pelo instante da última mudança.
	Caps: SyncCaps{Assignee: true, ServerSince: true},
	// O acesso vem da autorização no GitHub (github_oauth.go), sem token colado.
	Auth: AuthOAuth,
}

// GitHubIntegration fala com a API do GitHub. BaseURL e Client são opcionais e
// existem para apontar o adapter para um servidor fake nos testes.
type GitHubIntegration struct {
	BaseURL string
	Client  *http.Client
}

func (g *GitHubIntegration) baseURL() string {
	if g.BaseURL == "" {
		return defaultGitHubBaseURL
	}
	return strings.TrimRight(g.BaseURL, "/")
}

func (g *GitHubIntegration) client() *http.Client {
	if g.Client == nil {
		return &http.Client{Timeout: 10 * time.Second}
	}
	return g.Client
}

// githubMetadata são os campos que só o GitHub tem.
type githubMetadata struct {
	Repo string
}

var githubRepo = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9._-]+$`)

func parseGithubMetadata(raw map[string]any) (*githubMetadata, error) {
	fields, err := githubDescriptor.fields(raw)
	if err != nil {
		return nil, err
	}
	repo := trimURL(fields["repo"], "github.com")
	// O repositório entra na URL da API: um nome só de pontos subiria de diretório.
	name := repo[strings.LastIndex(repo, "/")+1:]
	if !githubRepo.MatchString(repo) || strings.Trim(name, ".") == "" {
		return nil, ErrGitHubInvalidRepo
	}
	return &githubMetadata{Repo: repo}, nil
}

func (g *GitHubIntegration) Descriptor() Descriptor {
	return githubDescriptor
}

func (g *GitHubIntegration) CheckMetadata(raw map[string]any) (map[string]any, error) {
	meta, err := parseGithubMetadata(raw)
	if err != nil {
		return nil, err
	}
	return map[string]any{"repo": meta.Repo}, nil
}

func (g *GitHubIntegration) get(conn Connection, path string) (*http.Response, error) {
	token, err := githubDescriptor.token(conn)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("GET", g.baseURL()+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := g.client().Do(req)
	if err != nil {
		return nil, ErrProviderUnreachable.With("provider", "GitHub").Wrap(err)
	}
	return resp, nil
}

func (g *GitHubIntegration) Validate(conn Connection) error {
	meta, err := parseGithubMetadata(conn.Metadata)
	if err != nil {
		return err
	}

	resp, err := g.get(conn, "/repos/"+meta.Repo)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return ErrInvalidToken.With("provider", "GitHub")
	case http.StatusNotFound:
		return ErrGitHubRepoNotFound
	default:
		return ErrProviderStatus.With("provider", "GitHub", "status", resp.StatusCode)
	}
}

func (g *GitHubIntegration) FetchItemDetails(conn Connection, itemID string) (*ItemDetails, error) {
	meta, err := parseGithubMetadata(conn.Metadata)
	if err != nil {
		return nil, err
	}
	number, err := issueNumber(itemID)
	if err != nil {
		return nil, err
	}

	resp, err := g.get(conn, "/repos/"+meta.Repo+"/issues/"+number)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrInvalidToken.With("provider", "GitHub")
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrItemNotFound.With("item", number)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, ErrProviderStatus.With("provider", "GitHub", "status", resp.StatusCode)
	}

	var body struct {
		Title   string `json:"title"`
		State   string `json:"state"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, ErrUnexpectedResponse.With("provider", "GitHub").Wrap(err)
	}

	return &ItemDetails{
		Title: body.Title,
		State: body.State,
		URL:   body.HTMLURL,
	}, nil
}

// Account devolve o login de quem o token representa. A conexão OAuth o usa para
// conferir o token recém-obtido e para nomear a integração.
func (g *GitHubIntegration) Account(conn Connection) (string, error) {
	resp, err := g.get(conn, "/user")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return "", ErrInvalidToken.With("provider", "GitHub")
	default:
		return "", ErrProviderStatus.With("provider", "GitHub", "status", resp.StatusCode)
	}

	var body struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || body.Login == "" {
		return "", ErrUnexpectedResponse.With("provider", "GitHub").Wrap(err)
	}
	return body.Login, nil
}

// ListRepositories devolve os repositórios que o token enxerga, os mexidos há menos
// tempo primeiro. É uma página de 100: quem tem mais digita o nome, que o campo aceita.
func (g *GitHubIntegration) ListRepositories(conn Connection) ([]Repository, error) {
	resp, err := g.get(conn, "/user/repos?per_page=100&sort=pushed&affiliation=owner,collaborator,organization_member")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return nil, ErrInvalidToken.With("provider", "GitHub")
	default:
		return nil, ErrProviderStatus.With("provider", "GitHub", "status", resp.StatusCode)
	}

	var body []struct {
		FullName string `json:"full_name"`
		Private  bool   `json:"private"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, ErrUnexpectedResponse.With("provider", "GitHub").Wrap(err)
	}
	repos := make([]Repository, 0, len(body))
	for _, r := range body {
		if r.FullName != "" {
			repos = append(repos, Repository{FullName: r.FullName, Private: r.Private})
		}
	}
	return repos, nil
}
