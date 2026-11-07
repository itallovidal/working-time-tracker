package page

import (
	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/domain/auth"
)

// Receivables é a página A receber do modo solo, só do dono (a rota pede receivables.manage): o que cobrar de cada
// cliente, o que foi confirmado e o que entrou. Fora do modo solo a página não existe (404): desligar o modo a esconde,
// e os recebíveis ficam guardados.
func (h *Handler) Receivables(c *echo.Context) error {
	if me := auth.CurrentPerson(c); me == nil || !me.OrganizationSolo {
		return h.NotFound(c)
	}
	d := h.orgData(c, Data{TitleKey: "titles.receivables", Section: "receivables"})
	d.Script = "receivables"
	return h.render(c, "org_receivables", d)
}
