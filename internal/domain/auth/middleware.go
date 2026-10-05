package auth

import (
	"errors"
	"mime"
	"net/http"
	"net/url"

	"github.com/labstack/echo/v5"
)

const (
	identityKey = "auth.identity"
	tokenKey    = "auth.token"
)

// CurrentPerson devolve a pessoa logada, ou nil quando a requisição não tem sessão.
func CurrentPerson(c *echo.Context) *Identity {
	id, _ := c.Get(identityKey).(*Identity)
	return id
}

func currentToken(c *echo.Context) string {
	token, _ := c.Get(tokenKey).(string)
	return token
}

type Middleware struct {
	svc          *Service
	resolver     *Resolver
	cookieSecure bool
}

func NewMiddleware(svc *Service, resolver *Resolver, cookieSecure bool) *Middleware {
	return &Middleware{svc: svc, resolver: resolver, cookieSecure: cookieSecure}
}

// LoadSession lê o cookie de sessão e, se ele for válido, põe a pessoa no contexto.
// Não bloqueia nada: quem exige login são RequireAPI e RequirePage.
func (m *Middleware) LoadSession(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		cookie, err := c.Cookie(CookieName)
		if err != nil || cookie.Value == "" {
			return next(c)
		}
		id, err := m.svc.Authenticate(cookie.Value)
		switch {
		case err == nil:
			c.Set(identityKey, id)
			c.Set(tokenKey, cookie.Value)
		case errors.Is(err, ErrUnauthenticated):
			clearSessionCookie(c, m.cookieSecure)
		default:
			return err
		}
		return next(c)
	}
}

func (m *Middleware) RequireAPI(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if CurrentPerson(c) == nil {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": ErrUnauthenticated.Error()})
		}
		return next(c)
	}
}

// RequirePage manda quem não está logado para o login, lembrando a página pedida.
func (m *Middleware) RequirePage(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if CurrentPerson(c) == nil {
			return c.Redirect(http.StatusSeeOther, "/login?next="+url.QueryEscape(c.Request().URL.RequestURI()))
		}
		return next(c)
	}
}

// RedirectIfAuthenticated tira quem já está logado das páginas de login e signup.
func (m *Middleware) RedirectIfAuthenticated(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if CurrentPerson(c) != nil {
			return c.Redirect(http.StatusSeeOther, "/")
		}
		return next(c)
	}
}

func (m *Middleware) RequireAdmin(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if !CurrentPerson(c).IsAdmin() {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "só admins podem fazer isso"})
		}
		return next(c)
	}
}

// RequireAdminPage faz a mesma checagem para páginas, com a resposta de 404 dada.
func (m *Middleware) RequireAdminPage(notFound echo.HandlerFunc) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if !CurrentPerson(c).IsAdmin() {
				return notFound(c)
			}
			return next(c)
		}
	}
}

// RequireSelfOrAdmin libera a rota para a própria pessoa do parâmetro ou para um admin.
func (m *Middleware) RequireSelfOrAdmin(param string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			id := CurrentPerson(c)
			if !id.IsAdmin() && (id == nil || id.PersonID.String() != c.Param(param)) {
				return c.JSON(http.StatusForbidden, map[string]string{"error": "você só pode alterar o seu próprio perfil"})
			}
			return next(c)
		}
	}
}

// JSONOnly recusa POST, PUT e PATCH com corpo que não seja JSON. Um formulário de
// outro site não consegue enviar application/json sem preflight de CORS, então
// isso completa o SameSite=Lax do cookie contra CSRF.
func (m *Middleware) JSONOnly(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		switch c.Request().Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
			ct := c.Request().Header.Get("Content-Type")
			if ct != "" {
				mt, _, err := mime.ParseMediaType(ct)
				if err != nil || mt != "application/json" {
					return c.JSON(http.StatusUnsupportedMediaType, map[string]string{
						"error": "envie o corpo como JSON (Content-Type: application/json)",
					})
				}
			}
		}
		return next(c)
	}
}
