package permission

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

// Handler serve o catálogo para as telas de permissões.
type Handler struct{}

func NewHandler() *Handler { return &Handler{} }

// Catalog é a resposta de GET /api/permissions: as permissões de cada escopo, na ordem em
// que as telas as mostram, e os grupos prontos do projeto. Os textos vêm do catálogo de
// idiomas, na chave permissions.<permissão com ponto virando sublinhado>.
type Catalog struct {
	Project      []string `json:"project"`
	Organization []string `json:"organization"`
	Presets      []Preset `json:"presets"`
}

// List devolve o catálogo.
func (h *Handler) List(c *echo.Context) error {
	return c.JSON(http.StatusOK, Catalog{Project: ProjectKeys, Organization: OrganizationKeys, Presets: Presets})
}
