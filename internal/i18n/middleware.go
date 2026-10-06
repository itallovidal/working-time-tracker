package i18n

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

// CookieName é o cookie com o idioma escolhido no toggle.
const CookieName = "wtt_lang"

const ctxKey = "i18n.lang"

// Middleware resolve o idioma de cada requisição e o guarda no contexto. Vem
// depois do LoadSession e antes de qualquer handler que escreva texto.
func (c *Catalog) Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx *echo.Context) error {
			c.Lang(ctx)
			return next(ctx)
		}
	}
}

// Lang é o idioma da requisição: o cookie, depois o Accept-Language, depois o
// padrão. O resultado fica no contexto. Quem escreve texto chama Lang direto, sem
// depender do middleware, porque o Echo não o executa para rotas que não existem
// (a página de 404 também precisa saber o idioma).
func (c *Catalog) Lang(ctx *echo.Context) string {
	if lang, ok := ctx.Get(ctxKey).(string); ok && lang != "" {
		return lang
	}
	cookie := ""
	if ck, err := ctx.Cookie(CookieName); err == nil {
		cookie = ck.Value
	}
	lang := c.Match(cookie, ctx.Request().Header.Get("Accept-Language"))
	ctx.Set(ctxKey, lang)
	// A página muda conforme o cookie e o cabeçalho; um cache no meio não pode
	// servir o idioma de outra pessoa.
	ctx.Response().Header().Add("Vary", "Cookie")
	ctx.Response().Header().Add("Vary", "Accept-Language")
	return lang
}

// SetCookie grava a escolha de idioma por um ano.
func SetCookie(ctx *echo.Context, lang string, secure bool) {
	ctx.SetCookie(&http.Cookie{
		Name:     CookieName,
		Value:    lang,
		Path:     "/",
		MaxAge:   365 * 24 * 60 * 60,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}
