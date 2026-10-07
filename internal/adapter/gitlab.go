package adapter

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const defaultGitLabBaseURL = "https://gitlab.com/api/v4"

var gitlabDescriptor = Descriptor{
	Type:  "gitlab",
	Label: "GitLab",
	Metadata: []Field{
		{Key: "project_url", Required: true, Summary: true},
	},
	ItemNumeric: true,
	ComingSoon:  true,
}

// GitLabIntegration fala com a API do GitLab. BaseURL e Client são opcionais e
// existem para apontar o adapter para um servidor fake nos testes.
type GitLabIntegration struct {
	BaseURL string
	Client  *http.Client
}

func (g *GitLabIntegration) baseURL() string {
	if g.BaseURL == "" {
		return defaultGitLabBaseURL
	}
	return strings.TrimRight(g.BaseURL, "/")
}

func (g *GitLabIntegration) client() *http.Client {
	if g.Client == nil {
		return &http.Client{Timeout: 10 * time.Second}
	}
	return g.Client
}

// gitlabMetadata são os campos que só o GitLab tem.
type gitlabMetadata struct {
	ProjectURL string
}

var gitlabProject = regexp.MustCompile(`^[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)+$`)

func parseGitlabMetadata(raw map[string]any) (*gitlabMetadata, error) {
	fields, err := gitlabDescriptor.fields(raw)
	if err != nil {
		return nil, err
	}
	project := trimURL(fields["project_url"], "gitlab.com")
	// O endereço de uma página do projeto (.../-/issues) também serve.
	if i := strings.Index(project, "/-/"); i >= 0 {
		project = project[:i]
	}
	if !gitlabProject.MatchString(project) {
		return nil, ErrGitLabInvalidProject
	}
	return &gitlabMetadata{ProjectURL: project}, nil
}

func (g *GitLabIntegration) Descriptor() Descriptor {
	return gitlabDescriptor
}

func (g *GitLabIntegration) CheckMetadata(raw map[string]any) (map[string]any, error) {
	meta, err := parseGitlabMetadata(raw)
	if err != nil {
		return nil, err
	}
	return map[string]any{"project_url": meta.ProjectURL}, nil
}

func (g *GitLabIntegration) get(conn Connection, path string) (*http.Response, error) {
	token, err := gitlabDescriptor.token(conn)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("GET", g.baseURL()+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", token)

	resp, err := g.client().Do(req)
	if err != nil {
		return nil, ErrProviderUnreachable.With("provider", "GitLab").Wrap(err)
	}
	return resp, nil
}

func (g *GitLabIntegration) Validate(conn Connection) error {
	meta, err := parseGitlabMetadata(conn.Metadata)
	if err != nil {
		return err
	}

	resp, err := g.get(conn, "/projects/"+url.PathEscape(meta.ProjectURL))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return ErrInvalidToken.With("provider", "GitLab")
	case http.StatusForbidden:
		return ErrGitLabForbidden
	case http.StatusNotFound:
		return ErrGitLabProjectMissing
	default:
		return ErrProviderStatus.With("provider", "GitLab", "status", resp.StatusCode)
	}
}

func (g *GitLabIntegration) FetchItemDetails(conn Connection, itemID string) (*ItemDetails, error) {
	meta, err := parseGitlabMetadata(conn.Metadata)
	if err != nil {
		return nil, err
	}
	number, err := issueNumber(itemID)
	if err != nil {
		return nil, err
	}

	resp, err := g.get(conn, "/projects/"+url.PathEscape(meta.ProjectURL)+"/issues/"+number)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrInvalidToken.With("provider", "GitLab")
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrItemNotFound.With("item", number)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, ErrProviderStatus.With("provider", "GitLab", "status", resp.StatusCode)
	}

	var body struct {
		Title  string `json:"title"`
		State  string `json:"state"`
		WebURL string `json:"web_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, ErrUnexpectedResponse.With("provider", "GitLab").Wrap(err)
	}

	return &ItemDetails{
		Title: body.Title,
		State: body.State,
		URL:   body.WebURL,
	}, nil
}
