package integration

import (
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/domain/auth"
)

// Os endereços da volta do Trello. A autorização do Trello devolve a pessoa a TrelloCallbackPath com o token
// depois do "#" (o fragmento nunca chega ao servidor): a página de retorno o lê no navegador e o entrega a
// TrelloTokenPath. PUBLIC_URL mais TrelloCallbackPath é o return_url mandado ao Trello.
const (
	TrelloCallbackPath = "/integrations/trello/callback"
	TrelloTokenPath    = "/integrations/trello/token"
)

// TrelloConnect começa a conexão: grava o cookie e manda a pessoa autorizar no Trello. A rota já passou
// por RequireOrg e pela permissão integrations.manage do projeto. Com ?integration=<id>, reconecta uma
// integração que já existe em vez de criar outra.
func (h *OAuthHandler) TrelloConnect(c *echo.Context) error {
	back := integrationsTab(c.Param("projectId"))
	if !h.trello.Configured() {
		return h.failBack(c, trelloFlow, back, adapter.ErrTrelloOAuthNotConfigured)
	}
	state, ok := h.begin(c, trelloFlow, back)
	if !ok {
		return nil
	}
	return c.Redirect(http.StatusFound, h.trello.AuthorizeURL(state))
}

// trelloToken é o que a página de retorno entrega: o state que o Trello devolveu na query e o token do
// fragmento.
type trelloToken struct {
	State string `json:"state"`
	Token string `json:"token"`
}

// TrelloToken recebe o token que a página de retorno leu do fragmento. Como a volta do GitHub, a rota só
// exige login: o projeto vem do cookie, e por isso a pessoa, a organização e a permissão são conferidas
// de novo aqui. A resposta é sempre 200 com o endereço para onde a página vai (a aba das integrações, com
// o id da integração ou o código do erro, ou a raiz quando não há conexão em andamento).
func (h *OAuthHandler) TrelloToken(c *echo.Context) error {
	reply := func(to string) error { return c.JSON(http.StatusOK, map[string]string{"redirect": to}) }
	claims, ok := h.readClaims(c, trelloFlow)
	h.clearCookie(c, trelloFlow)
	if !ok {
		return reply("/")
	}
	back := integrationsTab(claims.ProjectID)
	fail := func(err error) error {
		return reply(back + "?" + trelloFlow.errParam + "=" + url.QueryEscape(h.errorCode(c, trelloFlow, err)))
	}

	me := auth.CurrentPerson(c)
	if me == nil || me.PersonID.String() != claims.PersonID || !h.canManage(c, me, claims.ProjectID) {
		return reply("/")
	}
	var body trelloToken
	if err := c.Bind(&body); err != nil {
		return fail(adapter.ErrTrelloOAuthState)
	}
	if subtle.ConstantTimeCompare([]byte(body.State), []byte(claims.State)) != 1 {
		return fail(adapter.ErrTrelloOAuthState)
	}
	token := strings.TrimSpace(body.Token)
	if token == "" {
		// Quem clicou em Negar volta sem token.
		return fail(adapter.ErrTrelloOAuthDenied)
	}
	if !h.trello.Configured() {
		return fail(adapter.ErrTrelloOAuthNotConfigured)
	}

	// A chave do app fica no metadata da integração: o token do Trello pertence à chave que o gerou, e
	// guardar a chave junto faz uma troca da chave no .env não quebrar os tokens que já existem.
	app := map[string]interface{}{"api_key": h.trello.APIKey}
	var (
		it  *Integration
		err error
	)
	if claims.IntegrationID != "" {
		existing, gerr := h.svc.Get(claims.IntegrationID)
		if gerr != nil || existing.ProjectID.String() != claims.ProjectID {
			return fail(ErrNotFound)
		}
		it, err = h.svc.ReauthorizeWith(claims.IntegrationID, token, app)
	} else {
		it, err = h.svc.ConnectWith(claims.ProjectID, "trello", token, app)
	}
	if err != nil {
		return fail(err)
	}
	return reply(back + "?" + trelloFlow.okParam + "=" + it.ID.String())
}
