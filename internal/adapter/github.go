package adapter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const defaultGitHubBaseURL = "https://api.github.com"

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

type githubConfig struct {
	Token string `json:"token"`
	Repo  string `json:"repo"`
}

func (g *GitHubIntegration) ValidateConfig(config map[string]interface{}) error {
	cfg, err := parseGithubConfig(config)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/repos/%s", g.baseURL(), cfg.Repo), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := g.client().Do(req)
	if err != nil {
		return fmt.Errorf("não foi possível falar com o GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("token do GitHub inválido")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("o GitHub respondeu com status %d", resp.StatusCode)
	}
	return nil
}

func (g *GitHubIntegration) FetchItemDetails(config map[string]interface{}, itemID string) (*ItemDetails, error) {
	cfg, err := parseGithubConfig(config)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/repos/%s/issues/%s", g.baseURL(), cfg.Repo, itemID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := g.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("não foi possível falar com o GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("token do GitHub inválido")
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("item %s não encontrado", itemID)
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

func parseGithubConfig(config map[string]interface{}) (*githubConfig, error) {
	token, _ := config["token"].(string)
	repo, _ := config["repo"].(string)
	if token == "" {
		return nil, fmt.Errorf("informe o token do GitHub")
	}
	if repo == "" {
		return nil, fmt.Errorf("informe o repositório do GitHub (owner/repo)")
	}
	return &githubConfig{Token: token, Repo: repo}, nil
}
