package api

import (
	"time"

	"shares/internal/config"
)

// SetAuthCookies applies consistent security settings when issuing auth cookies.
// expiresUnix expects a Unix timestamp in seconds. When the value is non-positive,
// a reasonable default (2h) is used.
func SetAuthCookies(c *Context, openID, sessionID string, expiresUnix int64) {
	maxAge := 2 * 60 * 60
	if expiresUnix > 0 {
		if delta := int(expiresUnix - time.Now().Unix()); delta > 0 {
			maxAge = delta
		}
	}

	ginCtx := c.GetGinCtx()
	ginCtx.SetSameSite(config.GetCookieSameSite())
	domain := config.GetCookieDomain(ginCtx.Request.Host)
	secure := config.ShouldUseSecureCookies()

	ginCtx.SetCookie(UserToken, openID, maxAge, "/", domain, secure, true)
	ginCtx.SetCookie(SessionToken, sessionID, maxAge, "/", domain, secure, true)
}
