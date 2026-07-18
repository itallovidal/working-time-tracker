package handlers

import (
	"net/http"
	"working-time-tracker/internal/service"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

type PersonHandler struct {
	svc *service.PersonService
}

func NewPersonHandler(svc *service.PersonService) *PersonHandler {
	return &PersonHandler{svc: svc}
}

func (h *PersonHandler) Create(c *echo.Context) error {
	orgID := c.Param("orgId")
	var body struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	person, err := h.svc.Create(orgID, body.Name, body.Email)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusCreated, person)
}

func (h *PersonHandler) ListByOrg(c *echo.Context) error {
	orgID := c.Param("orgId")
	persons, err := h.svc.ListByOrg(orgID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, persons)
}

func (h *PersonHandler) Get(c *echo.Context) error {
	id := c.Param("personId")
	person, err := h.svc.Get(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "person not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, person)
}

func (h *PersonHandler) Update(c *echo.Context) error {
	id := c.Param("personId")
	var body struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	person, err := h.svc.Update(id, body.Name, body.Email)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, person)
}
