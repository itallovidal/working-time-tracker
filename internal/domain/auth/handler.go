package auth

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/person"
)

type Handler struct {
	svc          *Service
	cookieSecure bool
}

func NewHandler(svc *Service, cookieSecure bool) *Handler {
	return &Handler{svc: svc, cookieSecure: cookieSecure}
}

// fail traduz os erros do domínio para status HTTP. Erros inesperados viram 500
// com mensagem genérica, e o detalhe fica só no log.
func fail(c *echo.Context, err error) error {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, ErrInvalidCredentials), errors.Is(err, ErrUnauthenticated):
		status = http.StatusUnauthorized
	case errors.Is(err, ErrEmailInUse), errors.Is(err, ErrAccountExists):
		status = http.StatusConflict
	case errors.Is(err, ErrInviteInvalid), errors.Is(err, database.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, ErrWeakPassword), errors.Is(err, ErrLongPassword),
		errors.Is(err, ErrNameRequired), errors.Is(err, ErrOrgNameRequired),
		errors.Is(err, ErrInviteEmailMismatch), errors.Is(err, ErrWrongPassword),
		errors.Is(err, person.ErrInvalidEmail), errors.Is(err, person.ErrInvalidRole):
		status = http.StatusBadRequest
	}
	return apperr.Respond(c, status, err)
}

func badBody(c *echo.Context) error {
	return apperr.Respond(c, http.StatusBadRequest, apperr.ErrInvalidBody)
}

func (h *Handler) Signup(c *echo.Context) error {
	var body SignupInput
	if err := c.Bind(&body); err != nil {
		return badBody(c)
	}
	id, token, err := h.svc.Signup(body)
	if err != nil {
		return fail(c, err)
	}
	setSessionCookie(c, token, h.cookieSecure)
	return c.JSON(http.StatusCreated, id)
}

func (h *Handler) Login(c *echo.Context) error {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.Bind(&body); err != nil {
		return badBody(c)
	}
	id, token, err := h.svc.Login(body.Email, body.Password)
	if err != nil {
		return fail(c, err)
	}
	setSessionCookie(c, token, h.cookieSecure)
	return c.JSON(http.StatusOK, id)
}

func (h *Handler) Logout(c *echo.Context) error {
	if err := h.svc.Logout(currentToken(c)); err != nil {
		return fail(c, err)
	}
	clearSessionCookie(c, h.cookieSecure)
	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) Me(c *echo.Context) error {
	return c.JSON(http.StatusOK, CurrentPerson(c))
}

func (h *Handler) ChangePassword(c *echo.Context) error {
	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := c.Bind(&body); err != nil {
		return badBody(c)
	}
	if err := h.svc.ChangePassword(CurrentPerson(c), currentToken(c), body.CurrentPassword, body.NewPassword); err != nil {
		return fail(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

type inviteResponse struct {
	*Invite
	// Token e Path só aparecem na criação: depois disso o link não pode ser recuperado.
	Token string `json:"token"`
	Path  string `json:"path"`
}

func (h *Handler) CreateInvite(c *echo.Context) error {
	var body struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := c.Bind(&body); err != nil {
		return badBody(c)
	}
	inv, token, err := h.svc.CreateInvite(CurrentPerson(c), body.Email, body.Role)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusCreated, inviteResponse{Invite: inv, Token: token, Path: "/invite/" + token})
}

func (h *Handler) ListInvites(c *echo.Context) error {
	invites, err := h.svc.ListInvites(CurrentPerson(c).OrganizationID)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, invites)
}

func (h *Handler) RevokeInvite(c *echo.Context) error {
	if err := h.svc.RevokeInvite(c.Param("inviteId")); err != nil {
		return fail(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) InviteInfo(c *echo.Context) error {
	info, err := h.svc.InviteInfo(c.Param("token"))
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, info)
}

func (h *Handler) AcceptInvite(c *echo.Context) error {
	var body AcceptInviteInput
	if err := c.Bind(&body); err != nil {
		return badBody(c)
	}
	id, token, err := h.svc.AcceptInvite(c.Param("token"), body)
	if err != nil {
		return fail(c, err)
	}
	setSessionCookie(c, token, h.cookieSecure)
	return c.JSON(http.StatusCreated, id)
}
