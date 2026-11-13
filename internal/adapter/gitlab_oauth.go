package adapter

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultGitLabSiteURL = "https://gitlab.com"

// gitlabOAuthScope é o escopo que lê e escreve pela API: a sincronização das issues muda título, descrição,
// etiquetas, responsáveis e estado, e cria etiquetas (gitlab_sync.go). read_api só leria.
const gitlabOAuthScope = "api"

// GitLabAccountScope é o escopo da conta que a pessoa liga no perfil: só lê o usuário dela (o username e o e-mail).
const GitLabAccountScope = "read_user"

// Credential é o segredo de uma integração. Token é o de acesso, o único que o adapter recebe. RefreshToken e
// ExpiresAt só existem no token que vence, o do OAuth do GitLab (2 horas): o servidor o renova antes de usar.
// O token colado (e o do GitHub e do Trello) só tem Token.
type Credential struct {
	Token        string
	RefreshToken string
	ExpiresAt    time.Time
}

// TokenRefresher troca o token de renovação por um par novo. A renovação gira o par: o token de renovação usado
// deixa de valer, então quem chama precisa guardar o novo antes de qualquer outra renovação.
type TokenRefresher interface {
	Refresh(refreshToken string) (Credential, error)
}

// GitLabOAuth é o app OAuth que o servidor tem cadastrado no gitlab.com. Ele monta o endereço de autorização,
// troca o código da volta por um par de tokens e renova o par; o que se faz com o token (quem é, quais
// projetos) está no GitLabIntegration. SiteURL e Client são opcionais e existem para apontar para o fake.
type GitLabOAuth struct {
	ClientID     string
	ClientSecret string
	// RedirectURL é o endereço de retorno cadastrado no app (PUBLIC_URL mais o caminho do callback). O GitLab
	// só devolve a pessoa para ele, e pede o mesmo endereço na troca e na renovação.
	RedirectURL string
	SiteURL     string
	Client      *http.Client
}

var _ TokenRefresher = (*GitLabOAuth)(nil)

// Configured diz se o servidor tem tudo para começar uma conexão.
func (o *GitLabOAuth) Configured() bool {
	return o != nil && o.ClientID != "" && o.ClientSecret != "" && o.RedirectURL != ""
}

func (o *GitLabOAuth) siteURL() string {
	if o.SiteURL == "" {
		return defaultGitLabSiteURL
	}
	return strings.TrimRight(o.SiteURL, "/")
}

func (o *GitLabOAuth) client() *http.Client {
	if o.Client == nil {
		return &http.Client{Timeout: 10 * time.Second}
	}
	return o.Client
}

// AuthorizeURL é para onde a pessoa vai autorizar o acesso. O state volta igual no retorno, e é ele que prova
// que a volta é de uma conexão que este servidor começou.
func (o *GitLabOAuth) AuthorizeURL(state string) string {
	return o.AuthorizeURLScope(state, gitlabOAuthScope)
}

// AuthorizeURLScope é o AuthorizeURL com outro escopo: a conta do perfil pede só GitLabAccountScope.
func (o *GitLabOAuth) AuthorizeURLScope(state, scope string) string {
	q := url.Values{
		"client_id":     {o.ClientID},
		"redirect_uri":  {o.RedirectURL},
		"response_type": {"code"},
		"scope":         {scope},
		"state":         {state},
	}
	return o.siteURL() + "/oauth/authorize?" + q.Encode()
}

// Revoke revoga um token do app (o de acesso ou o de renovação).
func (o *GitLabOAuth) Revoke(token string) error {
	form := url.Values{"token": {token}, "client_id": {o.ClientID}, "client_secret": {o.ClientSecret}}
	req, err := http.NewRequest("POST", o.siteURL()+"/oauth/revoke", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := o.client().Do(req)
	if err != nil {
		return ErrProviderUnreachable.With("provider", "GitLab").Wrap(err)
	}
	defer closeBody(resp)
	if resp.StatusCode != http.StatusOK {
		return ErrProviderStatus.With("provider", "GitLab", "status", resp.StatusCode)
	}
	return nil
}

// Exchange troca o código da volta pelo par de tokens.
func (o *GitLabOAuth) Exchange(code string) (Credential, error) {
	cred, status, err := o.token(url.Values{"grant_type": {"authorization_code"}, "code": {code}})
	if err != nil {
		return Credential{}, err
	}
	if status != http.StatusOK || cred.Token == "" {
		return Credential{}, ErrGitLabOAuthExchange
	}
	return cred, nil
}

// Refresh troca o token de renovação por um par novo. O GitLab responde 400 invalid_grant ao token de renovação
// que já foi usado ou foi revogado (a pessoa tirou a autorização do app): é o token recusado, e a integração
// pede Reconectar.
func (o *GitLabOAuth) Refresh(refreshToken string) (Credential, error) {
	cred, status, err := o.token(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refreshToken}})
	if err != nil {
		return Credential{}, err
	}
	switch {
	case status == http.StatusOK && cred.Token != "":
		return cred, nil
	case status == http.StatusBadRequest || status == http.StatusUnauthorized:
		return Credential{}, ErrInvalidToken.With("provider", "GitLab")
	default:
		return Credential{}, ErrProviderStatus.With("provider", "GitLab", "status", status)
	}
}

// token faz o pedido ao /oauth/token com as credenciais do app e devolve o par e o status da resposta.
func (o *GitLabOAuth) token(form url.Values) (Credential, int, error) {
	form.Set("client_id", o.ClientID)
	form.Set("client_secret", o.ClientSecret)
	form.Set("redirect_uri", o.RedirectURL)
	req, err := http.NewRequest("POST", o.siteURL()+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return Credential{}, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	start := time.Now()
	resp, err := o.client().Do(req)
	if err != nil {
		return Credential{}, 0, ErrProviderUnreachable.With("provider", "GitLab").Wrap(err)
	}
	defer closeBody(resp)
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return Credential{}, resp.StatusCode, nil
	}
	var body struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return Credential{}, resp.StatusCode, ErrUnexpectedResponse.With("provider", "GitLab").Wrap(err)
	}
	cred := Credential{Token: body.AccessToken, RefreshToken: body.RefreshToken}
	// O vencimento conta a partir de quando o pedido saiu, para nunca passar do que o GitLab conta.
	if body.ExpiresIn > 0 {
		cred.ExpiresAt = start.Add(time.Duration(body.ExpiresIn) * time.Second)
	}
	return cred, resp.StatusCode, nil
}
