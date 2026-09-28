// Package page serve as páginas HTML da interface web. As páginas são cascas:
// o servidor resolve quem está logado e o contexto (org, projeto), e os
// componentes Alpine buscam e alteram os dados pela API JSON em /api.
package page

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/domain/auth"
)

// Crumb identifica um recurso no cabeçalho da página.
type Crumb struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Data é o que todo template recebe.
type Data struct {
	Title   string
	Me      *auth.Identity
	Section string // item ativo do menu da organização: projects, people, settings
	Project *Crumb
	Tab     string // aba ativa do projeto: tasks, time, teams, integrations, settings
	Script  string // página em /static/pages/<Script>.js com os componentes Alpine
	Props   map[string]any
}

// Boot vira window.BOOT na página: os dados iniciais que o JavaScript precisa.
func (d Data) Boot() map[string]any {
	boot := map[string]any{"me": d.Me}
	if d.Project != nil {
		boot["project"] = d.Project
	}
	for k, v := range d.Props {
		boot[k] = v
	}
	return boot
}

func (d Data) OrgID() string {
	if d.Me == nil {
		return ""
	}
	return d.Me.OrganizationID.String()
}

func (d Data) IsAdmin() bool {
	return d.Me.IsAdmin()
}

func (d Data) Initials() string {
	if d.Me == nil {
		return ""
	}
	parts := strings.Fields(d.Me.Name)
	out := ""
	for i, p := range parts {
		if i == 2 {
			break
		}
		out += strings.ToUpper(string([]rune(p)[0]))
	}
	return out
}

type Handler struct {
	deps Deps
}

func NewHandler(deps Deps) *Handler {
	return &Handler{deps: deps}
}

func (h *Handler) render(c *echo.Context, name string, d Data) error {
	d.Me = auth.CurrentPerson(c)
	return c.Render(http.StatusOK, name, d)
}

// NotFound é a página de 404 para rotas do navegador.
func (h *Handler) NotFound(c *echo.Context) error {
	return c.Render(http.StatusNotFound, "notfound", Data{Title: "Página não encontrada", Me: auth.CurrentPerson(c)})
}

// safeNext só aceita caminhos locais, para o ?next= do login não virar um
// redirecionamento para outro site.
func safeNext(next string) string {
	if strings.HasPrefix(next, "/") && !strings.HasPrefix(next, "//") && !strings.HasPrefix(next, "/\\") {
		return next
	}
	return "/"
}
