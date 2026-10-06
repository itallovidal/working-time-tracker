package page

import (
	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/domain/auth"
)

// projectHome é a aba em que o projeto abre para quem está logado: a Visão
// geral, que é só de admins, ou as Tarefas.
func projectHome(c *echo.Context) string {
	if auth.CurrentPerson(c).IsAdmin() {
		return "overview"
	}
	return "tasks"
}

// Overview é a Visão geral do projeto: pessoas, times, horas, custo, receita,
// tempo de projeto e integrações numa tela só, para admins (S26). Os tipos de
// integração vão junto para a lista mostrar o nome de cada plataforma.
func (h *Handler) Overview(c *echo.Context) error {
	return h.projectPage(c, "project_overview", "titles.overview", "overview", h.integrationTypes(c))
}
