package page

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/domain/personaccount"
	"working-time-tracker/internal/mcp"
)

// authProps junta as props de uma página de entrada com os dados do Clerk, quando ele está ligado.
func (h *Handler) authProps(props map[string]any) map[string]any {
	if h.deps.Clerk != nil {
		props["clerk"] = h.deps.Clerk
	}
	return props
}

func (h *Handler) Login(c *echo.Context) error {
	return h.render(c, "login", Data{
		TitleKey: "titles.login",
		Script:   "auth",
		Props: h.authProps(map[string]any{
			"next": safeNext(c.QueryParam("next")),
			// out=1 é o logout: com o Clerk, a página também desconecta dele, senão ele entraria de novo sozinho.
			"signedOut": c.QueryParam("out") == "1",
		}),
	})
}

func (h *Handler) Signup(c *echo.Context) error {
	return h.render(c, "signup", Data{TitleKey: "titles.signup", Script: "auth", Props: h.authProps(map[string]any{})})
}

func (h *Handler) Invite(c *echo.Context) error {
	return h.render(c, "invite", Data{
		TitleKey: "titles.invite",
		Script:   "auth",
		Props:    h.authProps(map[string]any{"token": c.Param("token")}),
	})
}

// ClerkContinue é a página para onde o Clerk volta depois de a pessoa entrar. Sem o Clerk ligado não há o que
// continuar: vai para o login.
func (h *Handler) ClerkContinue(c *echo.Context) error {
	if h.deps.Clerk == nil {
		return c.Redirect(http.StatusSeeOther, "/login")
	}
	return h.render(c, "clerk_continue", Data{
		TitleKey: "titles.clerk_continue",
		Script:   "auth",
		Props: h.authProps(map[string]any{
			"next":   safeNext(c.QueryParam("next")),
			"invite": c.QueryParam("invite"),
		}),
	})
}

// Profile tem os dados e a senha de quem está logado. Com o OAuth do MCP ligado, a seção Conector MCP mostra o
// endereço para colar no claude.ai: ele sai do PUBLIC_URL, nunca do endereço da requisição, que quem chama pode forjar.
func (h *Handler) Profile(c *echo.Context) error {
	return h.render(c, "profile", Data{TitleKey: "titles.profile", Section: "profile", Script: "org",
		Props: map[string]any{"mcpURL": h.baseURL(c) + mcp.Path, "oauth": h.deps.OAuth != nil,
			"accountPlatforms": h.accountPlatforms()}})
}

// accountPlatforms são as plataformas em que a pessoa pode ligar a conta (Contas conectadas): as que têm o app OAuth
// configurado no servidor, na ordem da tela.
func (h *Handler) accountPlatforms() []string {
	out := []string{}
	for _, p := range personaccount.Platforms {
		if h.deps.OAuthConfigured[p] {
			out = append(out, p)
		}
	}
	return out
}

// baseURL é o endereço do sistema: o PUBLIC_URL, ou, sem ele, o da requisição (o protocolo que o proxy da frente diz).
func (h *Handler) baseURL(c *echo.Context) string {
	if h.deps.PublicURL != "" {
		return h.deps.PublicURL
	}
	return c.Scheme() + "://" + c.Request().Host
}

// Home leva a pessoa logada para a página da organização dela.
func (h *Handler) Home(c *echo.Context) error {
	return c.Redirect(http.StatusSeeOther, "/orgs/"+auth.CurrentPerson(c).OrganizationID.String())
}
