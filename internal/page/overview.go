package page

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/domain/permission"
)

// projectHome é a página em que o projeto abre, para todos: a Visão geral de quem
// está olhando.
func projectHome() string {
	return "overview"
}

// MyOverview é o Início do projeto, a mesma tela para admin e membro: o relógio,
// as suas tarefas, o seu tempo neste projeto e as suas sessões.
func (h *Handler) MyOverview(c *echo.Context) error {
	return h.projectPage(c, "project_my_overview", "titles.my_overview", "overview", h.integrationTypes(c))
}

// Overview é a Visão geral da Gestão: pessoas, times, horas, custo, receita,
// tempo de projeto e integrações numa tela só, para admins (S26). Os tipos de
// integração vão junto para a lista mostrar o nome de cada plataforma.
func (h *Handler) Overview(c *echo.Context) error {
	return h.managementPage(c, "project_overview", "titles.overview", "overview", h.integrationTypes(c))
}

// Management leva da raiz da Gestão para a primeira aba que a pessoa pode abrir.
func (h *Handler) Management(c *echo.Context) error {
	set := auth.ProjectPermissions(c)
	tab := "overview"
	switch {
	case set.Has(permission.BillingView):
	case set.HasAny(permission.CollaboratorsManage, permission.TeamsManage, permission.RatesView, permission.RatesManage):
		tab = "teams"
	case set.Has(permission.IntegrationsManage):
		tab = "integrations"
	default:
		tab = "settings"
	}
	return c.Redirect(http.StatusSeeOther, "/projects/"+c.Param("projectId")+"/management/"+tab)
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
