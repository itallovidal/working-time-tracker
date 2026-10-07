package adapter

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultGitHubSiteURL = "https://github.com"

// githubOAuthScope é o único escopo que dá acesso a repositório privado num OAuth App.
// Ele também permite escrita, que o sistema não usa: só lê issues.
const githubOAuthScope = "repo"

// GitHubOAuth é o app OAuth que o servidor tem cadastrado no GitHub. Ele só monta o
// endereço de autorização e troca o código da volta por um token; o que se faz com o
// token (quem é, quais repositórios) está no GitHubIntegration. SiteURL e Client são
// opcionais e existem para apontar para um GitHub Enterprise ou um servidor fake.
type GitHubOAuth struct {
	ClientID     string
	ClientSecret string
	// RedirectURL é o endereço de retorno cadastrado no app (PUBLIC_URL mais o caminho
	// do callback). O GitHub só devolve a pessoa para ele.
	RedirectURL string
	SiteURL     string
	Client      *http.Client
}

// Configured diz se o servidor tem tudo para começar uma conexão.
func (o *GitHubOAuth) Configured() bool {
	return o != nil && o.ClientID != "" && o.ClientSecret != "" && o.RedirectURL != ""
}

func (o *GitHubOAuth) siteURL() string {
	if o.SiteURL == "" {
		return defaultGitHubSiteURL
	}
	return strings.TrimRight(o.SiteURL, "/")
}

func (o *GitHubOAuth) client() *http.Client {
	if o.Client == nil {
		return &http.Client{Timeout: 10 * time.Second}
	}
	return o.Client
}

// AuthorizeURL é para onde a pessoa vai autorizar o acesso. O state volta igual no
// retorno, e é ele que prova que a volta é de uma conexão que este servidor começou.
func (o *GitHubOAuth) AuthorizeURL(state string) string {
	q := url.Values{
		"client_id":    {o.ClientID},
		"redirect_uri": {o.RedirectURL},
		"scope":        {githubOAuthScope},
		"state":        {state},
	}
	return o.siteURL() + "/login/oauth/authorize?" + q.Encode()
}

// Exchange troca o código da volta pelo token de acesso. O GitHub responde 200 também
// quando recusa (o motivo vem no campo error), então a resposta é lida sempre.
func (o *GitHubOAuth) Exchange(code string) (string, error) {
	payload, err := json.Marshal(map[string]string{
		"client_id":     o.ClientID,
		"client_secret": o.ClientSecret,
		"code":          code,
		"redirect_uri":  o.RedirectURL,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest("POST", o.siteURL()+"/login/oauth/access_token", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := o.client().Do(req)
	if err != nil {
		return "", ErrProviderUnreachable.With("provider", "GitHub").Wrap(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", ErrProviderStatus.With("provider", "GitHub", "status", resp.StatusCode)
	}

	var body struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return "", ErrUnexpectedResponse.With("provider", "GitHub").Wrap(err)
	}
	if body.Error != "" || body.AccessToken == "" {
		return "", ErrGitHubOAuthExchange
	}
	return body.AccessToken, nil
}
