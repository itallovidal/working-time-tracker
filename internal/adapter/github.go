package adapter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const defaultGitHubBaseURL = "https://api.github.com"

var githubDescriptor = Descriptor{
	Type:        "github",
	Label:       "GitHub",
	Description: "Issues de um repositório",
	TokenHint:   "Token pessoal com permissão de leitura de issues.",
	Metadata: []Field{
		{Key: "repo", Label: "Repositório", Placeholder: "dono/repositorio", Hint: "Aceita também o endereço do repositório.", Required: true, Summary: true},
	},
	ItemLabel:       "Número da issue",
	ItemPlaceholder: "Ex.: 42",
	ItemNumeric:     true,
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
		return nil, fmt.Errorf("repositório do GitHub inválido: use dono/repositorio")
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
		return nil, fmt.Errorf("não foi possível falar com o GitHub: %w", err)
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
		return fmt.Errorf("token do GitHub inválido")
	case http.StatusNotFound:
		return fmt.Errorf("repositório do GitHub não encontrado ou o token não tem acesso a ele")
	default:
		return fmt.Errorf("o GitHub respondeu com status %d", resp.StatusCode)
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
		return nil, fmt.Errorf("token do GitHub inválido")
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("item %s não encontrado", number)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("o GitHub respondeu com status %d", resp.StatusCode)
	}

	var body struct {
		Title   string `json:"title"`
		State   string `json:"state"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("resposta inesperada do GitHub: %w", err)
	}

	return &ItemDetails{
		Title: body.Title,
		State: body.State,
		URL:   body.HTMLURL,
	}, nil
}
