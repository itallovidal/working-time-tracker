package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

// bearerToken é o token de sessão do Clerk que o navegador manda em "Authorization: Bearer ...", ou vazio.
func bearerToken(c *echo.Context) string {
	scheme, token, ok := strings.Cut(c.Request().Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

// clerkRespond escreve o resultado de um login pelo Clerk: o cookie, quando houve sessão, e o corpo.
func (h *Handler) clerkRespond(c *echo.Context, res *ClerkResult) error {
	status := http.StatusOK
	if res.Token != "" {
		setSessionCookie(c, res.Token, h.cookieSecure)
		if res.Created {
			status = http.StatusCreated
		}
	}
	return c.JSON(status, res)
}

// ClerkLogin entra pelo Clerk: o corpo pode levar o token de um convite e a senha de uma conta antiga.
func (h *Handler) ClerkLogin(c *echo.Context) error {
	var body ClerkLoginInput
	if err := c.Bind(&body); err != nil {
		return badBody(c)
	}
	res, err := h.svc.ClerkLogin(c.Request().Context(), bearerToken(c), body)
	if err != nil {
		return fail(c, err)
	}
	return h.clerkRespond(c, res)
}

// ClerkSignup cria uma organização nova para a pessoa do Clerk.
func (h *Handler) ClerkSignup(c *echo.Context) error {
	var body ClerkSignupInput
	if err := c.Bind(&body); err != nil {
		return badBody(c)
	}
	res, err := h.svc.ClerkSignup(c.Request().Context(), bearerToken(c), body)
	if err != nil {
		return fail(c, err)
	}
	return h.clerkRespond(c, res)
}

// ClerkJoin aceita um convite pendente feito para o e-mail da pessoa do Clerk.
func (h *Handler) ClerkJoin(c *echo.Context) error {
	var body ClerkJoinInput
	if err := c.Bind(&body); err != nil {
		return badBody(c)
	}
	res, err := h.svc.ClerkJoin(c.Request().Context(), bearerToken(c), body)
	if err != nil {
		return fail(c, err)
	}
	return h.clerkRespond(c, res)
}

// logClerkCause deixa no log o motivo técnico de uma falha do Clerk (o rastro da resposta, o motivo da recusa
// do token), que a resposta não leva. O token da pessoa nunca está nesse texto.
func logClerkCause(c *echo.Context, err error) {
	if cause := errors.Unwrap(err); cause != nil {
		c.Logger().Warn("clerk", "code", err.Error(), "cause", cause.Error())
	}
}
