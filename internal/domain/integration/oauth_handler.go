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

// O cookie que guarda a conexão em andamento entre a ida à plataforma e a volta. Fica restrito ao
// caminho da volta de cada plataforma e dura dez minutos. SameSite=Lax é o que permite o navegador
// mandá-lo na volta, que é uma navegação de outro site.
const (
	oauthCookie = "wtt_oauth"
	oauthTTL    = 10 * time.Minute
)

// oauthFlow é o que muda de uma plataforma para outra no vai e volta: o tipo da integração, o caminho a
// que o cookie fica preso e os parâmetros com que a volta avisa a aba (o id da integração, ou o código do
// erro).
type oauthFlow struct {
	typ        string
	cookiePath string
	okParam    string
	errParam   string
}

var (
	githubFlow = oauthFlow{typ: "github", cookiePath: "/integrations/github", okParam: "github", errParam: "github_error"}
	trelloFlow = oauthFlow{typ: "trello", cookiePath: "/integrations/trello", okParam: "trello", errParam: "trello_error"}
)

// CallbackPath é o caminho em que o GitHub devolve a pessoa. Quem monta o RedirectURL do
// app (PUBLIC_URL mais isto) é o cmd/main.go, e é o endereço que se cadastra no GitHub.
const CallbackPath = "/integrations/github/callback"

// oauthClaims é o que o cookie guarda: qual conexão a pessoa começou, em que projeto e,
// quando ela está reconectando uma integração que já existe, qual. O state é o que a
// plataforma devolve na volta para provar que a volta é desta conexão. Type diz de qual
// plataforma é: um cookie de uma não serve à volta da outra.
type oauthClaims struct {
	Type          string
	State         string
	PersonID      string
	ProjectID     string
	IntegrationID string
	Expires       int64
}

// OAuthHandler faz o vai e volta da conexão com o GitHub e com o Trello: Connect leva a pessoa para
// autorizar, e a volta (GitHubCallback, ou TrelloToken, que recebe o token que a página de retorno leu
// do fragmento da URL) guarda a integração. São rotas de página (GET e redirecionamentos), então a
// defesa contra CSRF não é o JSONOnly da API: é o state, selado no cookie.
type OAuthHandler struct {
	svc          *Service
	github       *adapter.GitHubOAuth
	trello       *adapter.TrelloAuth
	resolver     *auth.Resolver
	encryptKey   string
	cookieSecure bool
}

func NewOAuthHandler(svc *Service, github *adapter.GitHubOAuth, trello *adapter.TrelloAuth, resolver *auth.Resolver, encryptKey string, cookieSecure bool) *OAuthHandler {
	return &OAuthHandler{svc: svc, github: github, trello: trello, resolver: resolver, encryptKey: encryptKey, cookieSecure: cookieSecure}
}

// integrationsTab é a aba para onde a pessoa volta, com sucesso ou com erro.
func integrationsTab(projectID string) string {
	return "/projects/" + projectID + "/management/integrations"
}

// failBack volta para a aba levando o código do erro, que a tela traduz.
func (h *OAuthHandler) failBack(c *echo.Context, flow oauthFlow, back string, err error) error {
	return c.Redirect(http.StatusSeeOther, back+"?"+flow.errParam+"="+url.QueryEscape(h.errorCode(c, flow, err)))
}

// errorCode é o código do erro que a tela traduz; um erro sem código é um defeito nosso, que vai para o
// log e vira o erro interno.
func (h *OAuthHandler) errorCode(c *echo.Context, flow oauthFlow, err error) string {
	code := apperr.Code(err)
	if code == "" {
		c.Logger().Error(flow.typ+" oauth", "error", err)
		code = apperr.ErrInternal.Code
	}
	return code
}

// begin é o começo comum da conexão: confere a reconexão (?integration=<id> tem de ser uma integração
// deste tipo e deste projeto), sorteia o state e grava o cookie. Devolve o state para ir na ida; quando
// algo falha, já respondeu a pessoa e devolve ok falso.
func (h *OAuthHandler) begin(c *echo.Context, flow oauthFlow, back string) (state string, ok bool) {
	projectID := c.Param("projectId")
	integrationID := c.QueryParam("integration")
	if integrationID != "" {
		it, err := h.svc.Get(integrationID)
		if err != nil || it.ProjectID.String() != projectID || it.Type != flow.typ {
			h.failBack(c, flow, back, ErrNotFound)
			return "", false
		}
	}
	state, err := randomState()
	if err != nil {
		h.failBack(c, flow, back, err)
		return "", false
	}
	value, err := h.seal(oauthClaims{
		Type:          flow.typ,
		State:         state,
		PersonID:      auth.CurrentPerson(c).PersonID.String(),
		ProjectID:     projectID,
		IntegrationID: integrationID,
		Expires:       time.Now().Add(oauthTTL).Unix(),
	})
	if err != nil {
		h.failBack(c, flow, back, err)
		return "", false
	}
	c.SetCookie(&http.Cookie{
		Name:     oauthCookie,
		Value:    value,
		Path:     flow.cookiePath,
		MaxAge:   int(oauthTTL.Seconds()),
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	return state, true
}

// GitHubConnect começa a conexão: grava o cookie e manda a pessoa para o GitHub. A
// rota já passou por RequireOrg e pela permissão integrations.manage do projeto.
// Com ?integration=<id>, reconecta uma integração que já existe em vez de criar outra.
func (h *OAuthHandler) GitHubConnect(c *echo.Context) error {
	back := integrationsTab(c.Param("projectId"))
	if !h.github.Configured() {
		return h.failBack(c, githubFlow, back, adapter.ErrGitHubOAuthNotConfigured)
	}
	state, ok := h.begin(c, githubFlow, back)
	if !ok {
		return nil
	}
	return c.Redirect(http.StatusFound, h.github.AuthorizeURL(state))
}

// GitHubCallback recebe a volta do GitHub. A rota só exige login: o projeto vem do
// cookie, e por isso a pessoa, a organização e a permissão são conferidas de novo aqui,
// porque podem ter mudado nos minutos que a ida e a volta levaram.
func (h *OAuthHandler) GitHubCallback(c *echo.Context) error {
	claims, ok := h.readClaims(c, githubFlow)
	h.clearCookie(c, githubFlow)
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
		return h.failBack(c, githubFlow, back, adapter.ErrGitHubOAuthState)
	}
	if reason := c.QueryParam("error"); reason != "" {
		if reason == "access_denied" {
			return h.failBack(c, githubFlow, back, adapter.ErrGitHubOAuthDenied)
		}
		return h.failBack(c, githubFlow, back, adapter.ErrGitHubOAuthExchange)
	}
	code := c.QueryParam("code")
	if code == "" {
		return h.failBack(c, githubFlow, back, adapter.ErrGitHubOAuthExchange)
	}

	token, err := h.github.Exchange(code)
	if err != nil {
		return h.failBack(c, githubFlow, back, err)
	}

	var it *Integration
	if claims.IntegrationID != "" {
		existing, gerr := h.svc.Get(claims.IntegrationID)
		if gerr != nil || existing.ProjectID.String() != claims.ProjectID {
			return h.failBack(c, githubFlow, back, ErrNotFound)
		}
		it, err = h.svc.Reauthorize(claims.IntegrationID, token)
	} else {
		it, err = h.svc.Connect(claims.ProjectID, "github", token)
	}
	if err != nil {
		return h.failBack(c, githubFlow, back, err)
	}
	return c.Redirect(http.StatusSeeOther, back+"?"+githubFlow.okParam+"="+it.ID.String())
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
		"type":           claims.Type,
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

// readClaims lê o cookie da conexão em andamento. Cookie ausente, adulterado, vencido ou de outra
// plataforma é o mesmo que nenhuma conexão em andamento.
func (h *OAuthHandler) readClaims(c *echo.Context, flow oauthFlow) (oauthClaims, bool) {
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
		Type:          text("type"),
		State:         text("state"),
		PersonID:      text("person_id"),
		ProjectID:     text("project_id"),
		IntegrationID: text("integration_id"),
		Expires:       int64(expires),
	}
	if claims.Type != flow.typ || claims.State == "" || claims.ProjectID == "" || time.Now().Unix() > claims.Expires {
		return oauthClaims{}, false
	}
	return claims, true
}

// clearCookie apaga o cookie: ele vale para uma volta só.
func (h *OAuthHandler) clearCookie(c *echo.Context, flow oauthFlow) {
	c.SetCookie(&http.Cookie{
		Name:     oauthCookie,
		Value:    "",
		Path:     flow.cookiePath,
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
