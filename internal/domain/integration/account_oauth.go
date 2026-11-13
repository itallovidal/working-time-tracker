package integration

import (
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/domain/personaccount"
)

// A conta da pessoa no GitHub ou no GitLab (o perfil, Contas conectadas). É o mesmo vai e volta de conectar uma
// integração, e volta pelo mesmo endereço (o único que o app da plataforma aceita), mas pede só o escopo que lê quem
// a pessoa é, não guarda o token (ele é revogado logo depois de ler o login) e não pede permissão nenhuma: qualquer
// pessoa logada liga a própria conta.

// accountPurpose marca, no cookie, a ida que é a conta da pessoa.
const accountPurpose = "account"

// profilePath é para onde a pessoa volta quando a ida não disse outro lugar.
const profilePath = "/profile"

// AccountConnectPath é a ida: /profile/accounts/github/connect?return=/caminho.
const AccountConnectPath = "/profile/accounts/:platform/connect"

// accountFlows são as plataformas com conta e o vai e volta de cada uma.
var accountFlows = map[string]oauthFlow{personaccount.GitHub: githubFlow, personaccount.GitLab: gitlabFlow}

// AccountConnect leva a pessoa a autorizar na plataforma. ?return= é o caminho desta aplicação para onde ela volta
// (o perfil, ou a tarefa que ela estava pegando); qualquer outra coisa volta ao perfil.
func (h *OAuthHandler) AccountConnect(c *echo.Context) error {
	back := safeReturn(c.QueryParam("return"))
	platform := c.Param("platform")
	flow, ok := accountFlows[platform]
	if !ok || h.accounts == nil {
		return failAccount(c, back, personaccount.ErrInvalidPlatform.Code)
	}
	errs := githubErrs
	if platform == personaccount.GitLab {
		errs = gitlabErrs
	}
	if !h.accountConfigured(platform) {
		return failAccount(c, back, h.errorCode(c, flow, errs.notConfigured))
	}
	state, err := randomState()
	if err != nil {
		return failAccount(c, back, h.errorCode(c, flow, err))
	}
	value, err := h.seal(oauthClaims{
		Type:     flow.typ,
		Purpose:  accountPurpose,
		Return:   back,
		State:    state,
		PersonID: auth.CurrentPerson(c).PersonID.String(),
		Expires:  time.Now().Add(oauthTTL).Unix(),
	})
	if err != nil {
		return failAccount(c, back, h.errorCode(c, flow, err))
	}
	h.setCookie(c, flow, value)
	if platform == personaccount.GitLab {
		return c.Redirect(http.StatusFound, h.gitlab.AuthorizeURLScope(state, adapter.GitLabAccountScope))
	}
	return c.Redirect(http.StatusFound, h.github.AuthorizeURLScope(state, adapter.GitHubAccountScope))
}

// AccountsConfigured diz, por plataforma, se dá para ligar a conta: o app OAuth dela está configurado.
func (h *OAuthHandler) AccountsConfigured() map[string]bool {
	out := make(map[string]bool, len(personaccount.Platforms))
	for _, p := range personaccount.Platforms {
		out[p] = h.accounts != nil && h.accountConfigured(p)
	}
	return out
}

func (h *OAuthHandler) accountConfigured(platform string) bool {
	switch platform {
	case personaccount.GitHub:
		return h.github.Configured()
	case personaccount.GitLab:
		return h.gitlab.Configured()
	}
	return false
}

// accountCallback é a volta da conta da pessoa: troca o código, lê quem ela é, revoga o token e guarda a conta.
func (h *OAuthHandler) accountCallback(c *echo.Context, flow oauthFlow, errs codeErrs, claims oauthClaims,
	exchange func(code string) (adapter.Credential, error)) error {
	back := safeReturn(claims.Return)
	me := auth.CurrentPerson(c)
	if me == nil || me.PersonID.String() != claims.PersonID || h.accounts == nil {
		return c.Redirect(http.StatusSeeOther, "/")
	}
	if subtle.ConstantTimeCompare([]byte(c.QueryParam("state")), []byte(claims.State)) != 1 {
		return failAccount(c, back, h.errorCode(c, flow, errs.state))
	}
	if reason := c.QueryParam("error"); reason != "" {
		if reason == "access_denied" {
			return failAccount(c, back, h.errorCode(c, flow, errs.denied))
		}
		return failAccount(c, back, h.errorCode(c, flow, errs.exchange))
	}
	code := c.QueryParam("code")
	if code == "" {
		return failAccount(c, back, h.errorCode(c, flow, errs.exchange))
	}
	cred, err := exchange(code)
	if err != nil {
		return failAccount(c, back, h.errorCode(c, flow, err))
	}
	login, email, err := lookupUser(flow.typ, cred.Token)
	h.revoke(c, flow.typ, cred.Token)
	if err != nil {
		return failAccount(c, back, h.errorCode(c, flow, err))
	}
	if _, err := h.accounts.Link(personaccount.Account{
		PersonID: me.PersonID, OrganizationID: me.OrganizationID, Platform: flow.typ,
		Username: login, Email: email, Source: personaccount.SourceOAuth,
	}); err != nil {
		return failAccount(c, back, h.errorCode(c, flow, err))
	}
	return c.Redirect(http.StatusSeeOther, withQuery(back, "account", flow.typ))
}

// lookupUser lê o login e o e-mail de quem o token representa.
func lookupUser(platform, token string) (string, string, error) {
	impl, err := adapter.GetIntegration(platform)
	if err != nil {
		return "", "", err
	}
	lookup, ok := impl.(adapter.UserLookup)
	if !ok {
		return "", "", personaccount.ErrInvalidPlatform
	}
	return lookup.User(adapter.Connection{Token: token})
}

// revoke revoga o token que só serviu para saber quem a pessoa é. Uma falha não desfaz a conta: o token fica válido
// na plataforma até a pessoa o tirar lá, e o servidor não o guardou.
func (h *OAuthHandler) revoke(c *echo.Context, platform, token string) {
	var err error
	switch platform {
	case personaccount.GitHub:
		err = h.github.Revoke(token)
	case personaccount.GitLab:
		err = h.gitlab.Revoke(token)
	}
	if err != nil {
		c.Logger().Warn(platform+" account: revoking the token", "error", err)
	}
}

// linkConnector liga a quem conectou (ou reconectou) uma integração do GitHub ou do GitLab o login com que ela
// autorizou. Só entra se a pessoa ainda não tem conta na plataforma (a integração pode ter sido conectada com a conta
// de um robô); uma falha fica no log e não atrapalha a integração.
func (h *OAuthHandler) linkConnector(c *echo.Context, me *auth.Identity, platform, login string) {
	if h.accounts == nil || login == "" || !personaccount.ValidPlatform(platform) {
		return
	}
	if _, err := h.accounts.Link(personaccount.Account{
		PersonID: me.PersonID, OrganizationID: me.OrganizationID, Platform: platform,
		Username: login, Source: personaccount.SourceIntegration,
	}); err != nil {
		c.Logger().Warn(platform+" oauth: linking the account of who connected", "error", err)
	}
}

// failAccount volta para onde a pessoa estava com o código do erro, que a tela traduz.
func failAccount(c *echo.Context, back, code string) error {
	return c.Redirect(http.StatusSeeOther, withQuery(back, "account_error", code))
}

// withQuery acrescenta um parâmetro ao caminho de volta, que pode já ter outros.
func withQuery(back, key, value string) string {
	sep := "?"
	if strings.Contains(back, "?") {
		sep = "&"
	}
	return back + sep + key + "=" + url.QueryEscape(value)
}

// safeReturn só aceita um caminho desta aplicação ("/projects/..."): nada de outro site ("//site", "https://"),
// para a volta não virar um redirecionamento aberto. O resto vira o perfil.
func safeReturn(s string) string {
	if !strings.HasPrefix(s, "/") || strings.HasPrefix(s, "//") || strings.ContainsAny(s, "\\\r\n\t") {
		return profilePath
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil {
		return profilePath
	}
	return u.String()
}
