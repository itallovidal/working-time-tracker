package auth

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

const CookieName = "wtt_session"

func setSessionCookie(c *echo.Context, token string, secure bool) {
	c.SetCookie(&http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(SessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(c *echo.Context, secure bool) {
	c.SetCookie(&http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}
