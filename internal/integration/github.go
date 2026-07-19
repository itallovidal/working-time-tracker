package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type GitHubIntegration struct{}

type githubConfig struct {
	Token string `json:"token"`
	Repo  string `json:"repo"`
}

func (g *GitHubIntegration) ValidateConfig(config map[string]interface{}) error {
	cfg, err := parseGithubConfig(config)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.github.com/repos/%s", cfg.Repo), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach GitHub API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("invalid GitHub token")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}
	return nil
}

func (g *GitHubIntegration) FetchItemDetails(config map[string]interface{}, itemID string) (*ItemDetails, error) {
	cfg, err := parseGithubConfig(config)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.github.com/repos/%s/issues/%s", cfg.Repo, itemID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach GitHub API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("issue %s not found", itemID)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var body struct {
		Title string `json:"title"`
		State string `json:"state"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("failed to parse GitHub response: %w", err)
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
		return nil, fmt.Errorf("github token is required")
	}
	if repo == "" {
		return nil, fmt.Errorf("github repo is required")
	}
	return &githubConfig{Token: token, Repo: repo}, nil
}
