package page

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

// OAuthAuthorize é o /oauth/authorize do OAuth do MCP: a tela em que a pessoa logada deixa (ou não) o Claude agir em
// nome dela. Quem não está logado já passou pelo login (RequirePage) e voltou com o pedido inteiro na URL.
//
// Um pedido com o app desconhecido ou com um endereço de volta que não é do app vira a página de erro, sem
// redirecionar: mandar o erro para um endereço que não se sabe de quem é abriria um redirecionamento aberto. Os outros
// erros voltam ao app, como o OAuth manda.
func (h *Handler) OAuthAuthorize(c *echo.Context) error {
	if h.deps.OAuth == nil {
		return h.NotFound(c)
	}
	// A tela não pode ficar dentro de outro site (um clique escondido autorizaria), não fica em cache e não conta a
	// ninguém de onde a pessoa veio.
	header := c.Response().Header()
	header.Set("X-Frame-Options", "DENY")
	header.Set("Content-Security-Policy", "frame-ancestors 'none'")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("Cache-Control", "no-store")

	req, failure := h.deps.OAuth.ParseAuthorize(c.Request().Context(), c.QueryParams())
	if failure != nil {
		if failure.Redirect != "" {
			return c.Redirect(http.StatusFound, failure.Redirect)
		}
		return c.Render(http.StatusBadRequest, "oauth_error", h.prepare(c, Data{
			TitleKey: "titles.oauth_error", Bare: true,
			Props: map[string]any{"reason": failure.Reason},
		}))
	}
	return h.render(c, "oauth_consent", Data{
		TitleKey: "titles.oauth_consent", Bare: true, Script: "oauth",
		Props: map[string]any{
			"oauth":        req,
			"redirectHost": req.RedirectHost(),
			"loopback":     req.Loopback(),
			// Para trocar de conta e voltar a esta mesma tela.
			"authorizeURL": c.Request().URL.RequestURI(),
		},
	})
}
