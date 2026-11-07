package receivable

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/customer"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RequireSolo deixa passar só a organização que trabalha no modo solo; para as outras, as rotas deste domínio não
// existem (404). Vem depois do RequireOrg e da permissão da rota. A organização é lida a cada requisição: desligar o
// modo esconde as rotas na hora, e os recebíveis ficam guardados.
func (h *Handler) RequireSolo(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if !h.svc.Solo(c.Param("orgId")) {
			return apperr.Respond(c, http.StatusNotFound, apperr.ErrNotFound)
		}
		return next(c)
	}
}

// fail responde com o status do erro: o que não existe é 404, o que o estado atual não deixa (o período aberto, o
// recebível que já foi confirmado ou recebido) é 409, e o resto dos erros com código é pedido mal formado (400).
func fail(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, customer.ErrNotFound):
		return apperr.Respond(c, http.StatusNotFound, err)
	case errors.Is(err, database.ErrNotFound):
		return apperr.Respond(c, http.StatusNotFound, ErrNotFound)
	case errors.Is(err, ErrPeriodOpen), errors.Is(err, ErrAlreadyConfirmed), errors.Is(err, ErrAlreadyReceived), errors.Is(err, ErrNotReceived):
		return apperr.Respond(c, http.StatusConflict, err)
	}
	var coded *apperr.Error
	if errors.As(err, &coded) {
		return apperr.Respond(c, http.StatusBadRequest, err)
	}
	return apperr.Respond(c, http.StatusInternalServerError, err)
}

// List devolve os cartões e as três listas da página A receber: a confirmar, em aberto e recebidos.
func (h *Handler) List(c *echo.Context) error {
	summary, err := h.svc.Summary(c.Param("orgId"))
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, summary)
}

// Confirm confirma um período de um cliente (só o dono, pela rota). Responde 201 com o recebível.
func (h *Handler) Confirm(c *echo.Context) error {
	var body ConfirmInput
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, http.StatusBadRequest, apperr.BindError(err))
	}
	r, err := h.svc.Confirm(c.Param("orgId"), body)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusCreated, r)
}

// Update edita o valor, o vencimento, a nota e a observação de um recebível ainda não recebido.
func (h *Handler) Update(c *echo.Context) error {
	var body UpdateInput
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, http.StatusBadRequest, apperr.BindError(err))
	}
	r, err := h.svc.Update(c.Param("orgId"), c.Param("id"), body)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, r)
}

// Unconfirm desfaz a confirmação. Responde 204.
func (h *Handler) Unconfirm(c *echo.Context) error {
	if err := h.svc.Unconfirm(c.Param("orgId"), c.Param("id")); err != nil {
		return fail(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// LateCharges devolve a sugestão de multa e juros de receber o recebível na data ?on= (hoje, sem ela).
func (h *Handler) LateCharges(c *echo.Context) error {
	out, err := h.svc.LateCharges(c.Param("orgId"), c.Param("id"), c.QueryParam("on"))
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

// Receive registra o recebimento. Responde 200 com o recebível.
func (h *Handler) Receive(c *echo.Context) error {
	var body ReceiveInput
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, http.StatusBadRequest, apperr.BindError(err))
	}
	r, err := h.svc.Receive(c.Param("orgId"), c.Param("id"), body)
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(http.StatusOK, r)
}

// Unreceive desfaz o recebimento. Responde 204.
func (h *Handler) Unreceive(c *echo.Context) error {
	if err := h.svc.Unreceive(c.Param("orgId"), c.Param("id")); err != nil {
		return fail(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
