package integration

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/domain/permission"
)

// O cookie que guarda a conexão em andamento entre a ida ao GitHub e a volta. Fica
// restrito ao caminho do callback e dura dez minutos. SameSite=Lax é o que permite o
// navegador mandá-lo na volta, que é uma navegação de outro site.
const (
	oauthCookie     = "wtt_oauth"
	oauthCookiePath = "/integrations/github"
	oauthTTL        = 10 * time.Minute
)

// CallbackPath é o caminho em que o GitHub devolve a pessoa. Quem monta o RedirectURL do
// app (PUBLIC_URL mais isto) é o cmd/main.go, e é o endereço que se cadastra no GitHub.
const CallbackPath = "/integrations/github/callback"

// oauthClaims é o que o cookie guarda: qual conexão a pessoa começou, em que projeto e,
// quando ela está reconectando uma integração que já existe, qual. O state é o que o
// GitHub devolve na volta para provar que a volta é desta conexão.
type oauthClaims struct {
	State         string
	PersonID      string
	ProjectID     string
	IntegrationID string
	Expires       int64
}

// OAuthHandler faz o vai e volta da conexão com o GitHub: Connect leva a pessoa para
// autorizar, GitHubCallback recebe a volta, troca o código por um token e guarda a
// integração. São rotas de página (GET e redirecionamentos), então a defesa contra
// CSRF não é o JSONOnly da API: é o state, selado no cookie.
type OAuthHandler struct {
	svc          *Service
	github       *adapter.GitHubOAuth
	resolver     *auth.Resolver
	encryptKey   string
	cookieSecure bool
}

func NewOAuthHandler(svc *Service, github *adapter.GitHubOAuth, resolver *auth.Resolver, encryptKey string, cookieSecure bool) *OAuthHandler {
	return &OAuthHandler{svc: svc, github: github, resolver: resolver, encryptKey: encryptKey, cookieSecure: cookieSecure}
}

// integrationsTab é a aba para onde a pessoa volta, com sucesso ou com erro.
func integrationsTab(projectID string) string {
	return "/projects/" + projectID + "/management/integrations"
}

// failBack volta para a aba levando o código do erro, que a tela traduz.
func (h *OAuthHandler) failBack(c *echo.Context, back string, err error) error {
	code := apperr.Code(err)
	if code == "" {
		c.Logger().Error("github oauth", "error", err)
		code = apperr.ErrInternal.Code
	}
	return c.Redirect(http.StatusSeeOther, back+"?github_error="+url.QueryEscape(code))
}

// GitHubConnect começa a conexão: grava o cookie e manda a pessoa para o GitHub. A
// rota já passou por RequireOrg e pela permissão integrations.manage do projeto.
// Com ?integration=<id>, reconecta uma integração que já existe em vez de criar outra.
func (h *OAuthHandler) GitHubConnect(c *echo.Context) error {
	projectID := c.Param("projectId")
	back := integrationsTab(projectID)
	if !h.github.Configured() {
		return h.failBack(c, back, adapter.ErrGitHubOAuthNotConfigured)
	}

	integrationID := c.QueryParam("integration")
	if integrationID != "" {
		it, err := h.svc.Get(integrationID)
		if err != nil || it.ProjectID.String() != projectID || it.Type != "github" {
			return h.failBack(c, back, ErrNotFound)
		}
	}

	state, err := randomState()
	if err != nil {
		return h.failBack(c, back, err)
	}
	value, err := h.seal(oauthClaims{
		State:         state,
		PersonID:      auth.CurrentPerson(c).PersonID.String(),
		ProjectID:     projectID,
		IntegrationID: integrationID,
		Expires:       time.Now().Add(oauthTTL).Unix(),
	})
	if err != nil {
		return h.failBack(c, back, err)
	}
	c.SetCookie(&http.Cookie{
		Name:     oauthCookie,
		Value:    value,
		Path:     oauthCookiePath,
		MaxAge:   int(oauthTTL.Seconds()),
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	return c.Redirect(http.StatusFound, h.github.AuthorizeURL(state))
}

// GitHubCallback recebe a volta do GitHub. A rota só exige login: o projeto vem do
// cookie, e por isso a pessoa, a organização e a permissão são conferidas de novo aqui,
// porque podem ter mudado nos minutos que a ida e a volta levaram.
func (h *OAuthHandler) GitHubCallback(c *echo.Context) error {
	claims, ok := h.readClaims(c)
	h.clearCookie(c)
	if !ok {
		// Sem uma conexão em andamento não há para onde voltar: o link é velho ou não
		// foi este servidor que o começou.
		return c.Redirect(http.StatusSeeOther, "/")
	}
	back := integrationsTab(claims.ProjectID)

	me := auth.CurrentPerson(c)
	if me == nil || me.PersonID.String() != claims.PersonID || !h.canManage(c, me, claims.ProjectID) {
		return c.Redirect(http.StatusSeeOther, "/")
	}
	if subtle.ConstantTimeCompare([]byte(c.QueryParam("state")), []byte(claims.State)) != 1 {
		return h.failBack(c, back, adapter.ErrGitHubOAuthState)
	}
	if reason := c.QueryParam("error"); reason != "" {
		if reason == "access_denied" {
			return h.failBack(c, back, adapter.ErrGitHubOAuthDenied)
		}
		return h.failBack(c, back, adapter.ErrGitHubOAuthExchange)
	}
	code := c.QueryParam("code")
	if code == "" {
		return h.failBack(c, back, adapter.ErrGitHubOAuthExchange)
	}

	token, err := h.github.Exchange(code)
	if err != nil {
		return h.failBack(c, back, err)
	}

	var it *Integration
	if claims.IntegrationID != "" {
		existing, gerr := h.svc.Get(claims.IntegrationID)
		if gerr != nil || existing.ProjectID.String() != claims.ProjectID {
			return h.failBack(c, back, ErrNotFound)
		}
		it, err = h.svc.Reauthorize(claims.IntegrationID, token)
	} else {
		it, err = h.svc.Connect(claims.ProjectID, "github", token)
	}
	if err != nil {
		return h.failBack(c, back, err)
	}
	return c.Redirect(http.StatusSeeOther, back+"?github="+it.ID.String())
}

// canManage confere, na volta, o que o middleware conferiu na ida: o projeto é da
// organização da pessoa e ela ainda pode gerenciar integrações nele.
func (h *OAuthHandler) canManage(c *echo.Context, me *auth.Identity, projectID string) bool {
	if !h.resolver.SameOrganization(c, auth.KindProject, projectID) {
		return false
	}
	id, err := uuid.Parse(projectID)
	if err != nil {
		return false
	}
	set, err := h.resolver.ProjectSet(me, id)
	return err == nil && set.HasAny(permission.IntegrationsManage)
}

// seal e open guardam as claims no cookie com a mesma criptografia autenticada das
// credenciais (AES-GCM): quem não tem a chave não lê nem altera o que está nele.
func (h *OAuthHandler) seal(claims oauthClaims) (string, error) {
	sealed, err := adapter.EncryptConfig(map[string]interface{}{
		"state":          claims.State,
		"person_id":      claims.PersonID,
		"project_id":     claims.ProjectID,
		"integration_id": claims.IntegrationID,
		"expires":        claims.Expires,
	}, h.encryptKey)
	if err != nil {
		return "", err
	}
	value, _ := sealed["encrypted_data"].(string)
	return value, nil
}

// readClaims lê o cookie da conexão em andamento. Cookie ausente, adulterado ou vencido
// é o mesmo que nenhuma conexão em andamento.
func (h *OAuthHandler) readClaims(c *echo.Context) (oauthClaims, bool) {
	cookie, err := c.Cookie(oauthCookie)
	if err != nil || cookie.Value == "" {
		return oauthClaims{}, false
	}
	opened, err := adapter.DecryptConfig(map[string]interface{}{"encrypted_data": cookie.Value}, h.encryptKey)
	if err != nil {
		return oauthClaims{}, false
	}
	text := func(key string) string { s, _ := opened[key].(string); return s }
	expires, _ := opened["expires"].(float64)
	claims := oauthClaims{
		State:         text("state"),
		PersonID:      text("person_id"),
		ProjectID:     text("project_id"),
		IntegrationID: text("integration_id"),
		Expires:       int64(expires),
	}
	if claims.State == "" || claims.ProjectID == "" || time.Now().Unix() > claims.Expires {
		return oauthClaims{}, false
	}
	return claims, true
}

// clearCookie apaga o cookie: ele vale para uma volta só.
func (h *OAuthHandler) clearCookie(c *echo.Context) {
	c.SetCookie(&http.Cookie{
		Name:     oauthCookie,
		Value:    "",
		Path:     oauthCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func randomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
