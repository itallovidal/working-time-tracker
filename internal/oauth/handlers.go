package oauth

import (
	"errors"
	"mime"
	"net/http"
	"net/url"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/domain/auth"
)

// ErrRequestInvalid é o pedido de autorização que a tela de consentimento devolveu e não serve mais (o app ou o
// endereço de volta não conferem). O que pode voltar ao app volta como redirecionamento, não como este erro.
var ErrRequestInvalid = apperr.New("oauth.request_invalid", http.StatusBadRequest)

// consentBody é o que a tela de consentimento manda: o pedido que ela mostrou e a resposta da pessoa.
type consentBody struct {
	ClientID            string `json:"client_id"`
	RedirectURI         string `json:"redirect_uri"`
	State               string `json:"state"`
	ResponseType        string `json:"response_type"`
	CodeChallenge       string `json:"code_challenge"`
	CodeChallengeMethod string `json:"code_challenge_method"`
	Resource            string `json:"resource"`
	Scope               string `json:"scope"`
	Approve             bool   `json:"approve"`
}

// Consent é o "Permitir" (ou o "Cancelar") da tela de consentimento, com o cookie de quem está logado. O pedido é
// conferido de novo, e a concessão é sempre da pessoa da sessão. A resposta é o endereço de volta ao app, com o código
// ou com o access_denied; a tela vai para ele.
func (s *Server) Consent(c *echo.Context) error {
	var body consentBody
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, http.StatusBadRequest, apperr.BindError(err))
	}
	q := url.Values{}
	for k, v := range map[string]string{
		"client_id": body.ClientID, "redirect_uri": body.RedirectURI, "state": body.State,
		"response_type": body.ResponseType, "code_challenge": body.CodeChallenge,
		"code_challenge_method": body.CodeChallengeMethod, "resource": body.Resource, "scope": body.Scope,
	} {
		if v != "" {
			q.Set(k, v)
		}
	}
	req, failure := s.ParseAuthorize(c.Request().Context(), q)
	if failure != nil {
		if failure.Redirect != "" {
			return c.JSON(http.StatusOK, map[string]string{"redirect": failure.Redirect})
		}
		return apperr.Respond(c, http.StatusBadRequest, ErrRequestInvalid)
	}
	if !body.Approve {
		return c.JSON(http.StatusOK, map[string]string{
			"redirect": s.errorURL(req.RedirectURI, "access_denied", "the person did not allow the access", req.State),
		})
	}
	code, err := s.auth.CreateOAuthGrant(auth.CurrentPerson(c), auth.OAuthGrantRequest{
		ClientID: req.ClientID, ClientName: req.ClientName, RedirectURI: req.RedirectURI,
		Scope: req.Scope, Resource: req.Resource, CodeChallenge: req.CodeChallenge,
	})
	if err != nil {
		return auth.Respond(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"redirect": s.successURL(req.RedirectURI, code, req.State)})
}

// tokenError é o erro do /oauth/token e do /oauth/revoke (RFC 6749, seção 5.2).
type tokenError struct {
	Error       string `json:"error"`
	Description string `json:"error_description,omitempty"`
}

// tokenResponse é a resposta do /oauth/token (RFC 6749, seção 5.1).
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

// Token troca o código de autorização, ou o token de renovação, por tokens novos. O corpo é um formulário
// (application/x-www-form-urlencoded) e só ele vale: parâmetros na URL não contam, e um parâmetro repetido é recusado.
// O app é público (sem segredo): o client_id vem no corpo e tem de ser um dos aceitos.
func (s *Server) Token(c *echo.Context) error {
	noStore(c)
	allowAnyOrigin(c, "POST, OPTIONS")
	form, bad := readForm(c)
	if bad != nil {
		return c.JSON(http.StatusBadRequest, bad)
	}
	clientID := form.Get("client_id")
	if _, known := s.clients.pinned[clientID]; !known {
		return c.JSON(http.StatusUnauthorized, tokenError{Error: "invalid_client", Description: "client_id is not an accepted client"})
	}
	resource := ""
	if raw := form.Get("resource"); raw != "" {
		got, err := CanonicalResource(raw)
		if err != nil || got != s.resource {
			return c.JSON(http.StatusBadRequest, tokenError{Error: "invalid_target", Description: "resource must be " + s.resource})
		}
		resource = got
	}

	var tokens *auth.OAuthTokens
	var err error
	switch form.Get("grant_type") {
	case "authorization_code":
		code, redirect, verifier := form.Get("code"), form.Get("redirect_uri"), form.Get("code_verifier")
		if code == "" || redirect == "" || verifier == "" {
			return c.JSON(http.StatusBadRequest, tokenError{Error: "invalid_request", Description: "code, redirect_uri and code_verifier are required"})
		}
		tokens, err = s.auth.ExchangeOAuthCode(auth.OAuthExchange{
			Code: code, ClientID: clientID, RedirectURI: redirect, CodeVerifier: verifier, Resource: resource,
		})
	case "refresh_token":
		refresh := form.Get("refresh_token")
		if refresh == "" {
			return c.JSON(http.StatusBadRequest, tokenError{Error: "invalid_request", Description: "refresh_token is required"})
		}
		tokens, err = s.auth.RefreshOAuthToken(refresh, clientID, resource)
	case "":
		return c.JSON(http.StatusBadRequest, tokenError{Error: "invalid_request", Description: "grant_type is required"})
	default:
		return c.JSON(http.StatusBadRequest, tokenError{Error: "unsupported_grant_type"})
	}
	switch {
	case errors.Is(err, auth.ErrOAuthGrantInvalid):
		return c.JSON(http.StatusBadRequest, tokenError{Error: "invalid_grant"})
	case errors.Is(err, auth.ErrOAuthTargetInvalid):
		return c.JSON(http.StatusBadRequest, tokenError{Error: "invalid_target"})
	case err != nil:
		s.logger.Error("oauth: token request failed", "grant_type", form.Get("grant_type"), "error", err)
		return c.JSON(http.StatusInternalServerError, tokenError{Error: "server_error"})
	}
	return c.JSON(http.StatusOK, tokenResponse{
		AccessToken: tokens.AccessToken, TokenType: "Bearer", ExpiresIn: tokens.ExpiresIn,
		RefreshToken: tokens.RefreshToken, Scope: tokens.Scope,
	})
}

// Revoke é a revogação do RFC 7009: o token de acesso deixa de valer, e o de renovação desconecta o app. A resposta é
// 200 também para um token que não existe, como o RFC pede.
func (s *Server) Revoke(c *echo.Context) error {
	noStore(c)
	allowAnyOrigin(c, "POST, OPTIONS")
	form, bad := readForm(c)
	if bad != nil {
		return c.JSON(http.StatusBadRequest, bad)
	}
	token := form.Get("token")
	if token == "" {
		return c.JSON(http.StatusBadRequest, tokenError{Error: "invalid_request", Description: "token is required"})
	}
	if err := s.auth.RevokeOAuthToken(token); err != nil {
		s.logger.Error("oauth: revoke failed", "error", err)
		return c.JSON(http.StatusInternalServerError, tokenError{Error: "server_error"})
	}
	return c.NoContent(http.StatusOK)
}

// readForm lê o corpo do formulário, e só ele. Um corpo que não é formulário, ou um parâmetro repetido, é um
// invalid_request.
func readForm(c *echo.Context) (url.Values, *tokenError) {
	req := c.Request()
	if mt, _, err := mime.ParseMediaType(req.Header.Get("Content-Type")); err != nil || mt != "application/x-www-form-urlencoded" {
		return nil, &tokenError{Error: "invalid_request", Description: "the body must be application/x-www-form-urlencoded"}
	}
	req.Body = http.MaxBytesReader(c.Response(), req.Body, 64<<10)
	if err := req.ParseForm(); err != nil {
		return nil, &tokenError{Error: "invalid_request", Description: "the body is not a valid form"}
	}
	for name, values := range req.PostForm {
		if len(values) > 1 {
			if name == "resource" {
				return nil, &tokenError{Error: "invalid_target", Description: "only one resource is allowed"}
			}
			return nil, &tokenError{Error: "invalid_request", Description: name + " is repeated"}
		}
	}
	return req.PostForm, nil
}

// noStore: os tokens nunca ficam em cache (RFC 6749, seção 5.1).
func noStore(c *echo.Context) {
	h := c.Response().Header()
	h.Set("Cache-Control", "no-store")
	h.Set("Pragma", "no-cache")
}
