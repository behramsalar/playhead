package auth

import (
	"net/http"
	"time"
)

// CookieName is the session cookie's name.
const CookieName = "vp_session"

// SetSessionCookie sets the signed session cookie. Secure is deliberately
// never set: many people reach this app over plain HTTP directly on their
// tailnet (not through `tailscale serve`), and Tailscale already encrypts
// the transport at the WireGuard layer — requiring Secure would silently
// break login for them. SameSite=Lax is enough for a same-origin app with
// no cross-site form posts to protect against.
func SetSessionCookie(w http.ResponseWriter, token string, expiry time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiry,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookie logs the browser out by expiring the cookie
// immediately.
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// ReadSessionCookie returns the raw token from the request's session
// cookie, if present.
func ReadSessionCookie(r *http.Request) (string, bool) {
	c, err := r.Cookie(CookieName)
	if err != nil || c.Value == "" {
		return "", false
	}
	return c.Value, true
}
