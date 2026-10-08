package page

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

// Help é a ajuda: uma página só com o passo a passo do primeiro uso e o que cada tela faz.
// É pública, para dar o link a quem ainda vai se cadastrar; quem está logado a vê com a
// barra superior.
func (h *Handler) Help(c *echo.Context) error {
	return h.render(c, "help", Data{TitleKey: "titles.help", Section: "help", Script: "help", Props: map[string]any{"emailInvites": h.deps.InviteByEmail}})
}

// ToHelp leva o endereço em português para a ajuda.
func (h *Handler) ToHelp(c *echo.Context) error {
	return c.Redirect(http.StatusSeeOther, "/help")
}
