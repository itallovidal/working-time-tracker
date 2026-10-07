package page

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

// projectHome é a página em que o projeto abre, para todos: o Ponto.
func projectHome() string {
	return "time-tracking"
}

// Overview é a Visão geral da Gestão: pessoas, times, horas, custo, receita,
// tempo de projeto e integrações numa tela só, para admins (S26). Os tipos de
// integração vão junto para a lista mostrar o nome de cada plataforma.
func (h *Handler) Overview(c *echo.Context) error {
	return h.managementPage(c, "project_overview", "titles.overview", "overview", h.integrationTypes(c))
}

// Management leva da raiz da Gestão para a primeira aba dela.
func (h *Handler) Management(c *echo.Context) error {
	return c.Redirect(http.StatusSeeOther, "/projects/"+c.Param("projectId")+"/management/overview")
}

// ToManagement é o caminho antigo de uma aba que mudou para a Gestão: leva ao
// novo, com a query (a aba Colaboradores guarda a visão em ?view=). Quem não é
// admin recebe o 404 da página nova.
func (h *Handler) ToManagement(tab string) echo.HandlerFunc {
	return func(c *echo.Context) error {
		to := "/projects/" + c.Param("projectId") + "/management/" + tab
		if q := c.QueryString(); q != "" {
			to += "?" + q
		}
		return c.Redirect(http.StatusSeeOther, to)
	}
}
