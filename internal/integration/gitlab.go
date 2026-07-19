package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type GitLabIntegration struct{}

type gitlabConfig struct {
	Token       string `json:"token"`
	ProjectURL  string `json:"project_url"`
}

func (g *GitLabIntegration) ValidateConfig(config map[string]interface{}) error {
	cfg, err := parseGitlabConfig(config)
	if err != nil {
		return err
	}

	apiURL := fmt.Sprintf("https://gitlab.com/api/v4/projects/%s", url.PathEscape(cfg.ProjectURL))
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("PRIVATE-TOKEN", cfg.Token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach GitLab API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("invalid GitLab token")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitLab API returned status %d", resp.StatusCode)
	}
	return nil
}

func (g *GitLabIntegration) FetchItemDetails(config map[string]interface{}, itemID string) (*ItemDetails, error) {
	cfg, err := parseGitlabConfig(config)
	if err != nil {
		return nil, err
	}

	apiURL := fmt.Sprintf("https://gitlab.com/api/v4/projects/%s/issues/%s",
		url.PathEscape(cfg.ProjectURL), itemID)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", cfg.Token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach GitLab API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("issue %s not found", itemID)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitLab API returned status %d", resp.StatusCode)
	}

	var body struct {
		Title    string `json:"title"`
		State    string `json:"state"`
		WebURL   string `json:"web_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("failed to parse GitLab response: %w", err)
	}

	return &ItemDetails{
		Title: body.Title,
		State: body.State,
		URL:   body.WebURL,
	}, nil
}

func parseGitlabConfig(config map[string]interface{}) (*gitlabConfig, error) {
	token, _ := config["token"].(string)
	projectURL, _ := config["project_url"].(string)
	if token == "" {
		return nil, fmt.Errorf("gitlab token is required")
	}
	if projectURL == "" {
		return nil, fmt.Errorf("gitlab project_url is required")
	}
	projectURL = strings.TrimPrefix(projectURL, "https://")
	projectURL = strings.TrimPrefix(projectURL, "http://")
	projectURL = strings.TrimPrefix(projectURL, "gitlab.com/")
	return &gitlabConfig{Token: token, ProjectURL: projectURL}, nil
}
