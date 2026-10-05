package page

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/domain/auth"
)

func (h *Handler) Login(c *echo.Context) error {
	return h.render(c, "login", Data{
		Title:  "Entrar",
		Script: "auth",
		Props:  map[string]any{"next": safeNext(c.QueryParam("next"))},
	})
}

func (h *Handler) Signup(c *echo.Context) error {
	return h.render(c, "signup", Data{Title: "Criar conta", Script: "auth"})
}

func (h *Handler) Invite(c *echo.Context) error {
	return h.render(c, "invite", Data{
		Title:  "Convite",
		Script: "auth",
		Props:  map[string]any{"token": c.Param("token")},
	})
}

// Profile tem os dados e a senha de quem está logado.
func (h *Handler) Profile(c *echo.Context) error {
	return h.render(c, "profile", Data{Title: "Seu perfil", Section: "profile", Script: "org"})
}

// Home leva a pessoa logada para a página da organização dela.
func (h *Handler) Home(c *echo.Context) error {
	return c.Redirect(http.StatusSeeOther, "/orgs/"+auth.CurrentPerson(c).OrganizationID.String())
}
