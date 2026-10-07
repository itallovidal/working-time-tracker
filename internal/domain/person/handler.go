package person

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/database"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) ListByOrg(c *echo.Context) error {
	orgID := c.Param("orgId")
	persons, err := h.svc.ListByOrg(orgID)
	if err != nil {
		return apperr.Respond(c, http.StatusInternalServerError, err)
	}
	return c.JSON(http.StatusOK, persons)
}

func (h *Handler) Get(c *echo.Context) error {
	id := c.Param("personId")
	person, err := h.svc.Get(id)
	if err != nil {
		if err == database.ErrNotFound {
			return apperr.Respond(c, http.StatusNotFound, ErrNotFound)
		}
		return apperr.Respond(c, http.StatusInternalServerError, err)
	}
	return c.JSON(http.StatusOK, person)
}

func (h *Handler) Update(c *echo.Context) error {
	id := c.Param("personId")
	var body struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, http.StatusBadRequest, apperr.ErrInvalidBody)
	}
	person, err := h.svc.Update(id, body.Name, body.Email)
	if err != nil {
		if err == database.ErrNotFound {
			return apperr.Respond(c, http.StatusNotFound, ErrNotFound)
		}
		return apperr.Respond(c, http.StatusBadRequest, err)
	}
	return c.JSON(http.StatusOK, person)
}

func (h *Handler) SetRole(c *echo.Context) error {
	id := c.Param("personId")
	var body struct {
		Role string `json:"role"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, http.StatusBadRequest, apperr.ErrInvalidBody)
	}
	person, err := h.svc.SetRole(id, body.Role)
	if err != nil {
		if err == database.ErrNotFound {
			return apperr.Respond(c, http.StatusNotFound, ErrNotFound)
		}
		return apperr.Respond(c, http.StatusBadRequest, err)
	}
	return c.JSON(http.StatusOK, person)
}

// SetWeeklyHours define a jornada semanal da pessoa. A rota é só de admins: a
// jornada é o que a organização combinou com ela.
func (h *Handler) SetWeeklyHours(c *echo.Context) error {
	id := c.Param("personId")
	var body struct {
		WeeklyHours *int `json:"weekly_hours"`
	}
	if err := c.Bind(&body); err != nil {
		return apperr.Respond(c, http.StatusBadRequest, apperr.ErrInvalidBody)
	}
	person, err := h.svc.SetWeeklyHours(id, body.WeeklyHours)
	if err != nil {
		if err == database.ErrNotFound {
			return apperr.Respond(c, http.StatusNotFound, ErrNotFound)
		}
		return apperr.Respond(c, http.StatusBadRequest, err)
	}
	return c.JSON(http.StatusOK, person)
}
