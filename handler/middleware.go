package handler

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}

func BasicAuth(user, password string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			u, p, ok := r.BasicAuth()
			if !ok ||
				subtle.ConstantTimeCompare([]byte(u), []byte(user)) != 1 ||
				subtle.ConstantTimeCompare([]byte(p), []byte(password)) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="Admin"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next(w, r)
		}
	}
}

// sessionIDKey is the context key for the session ID.
type sessionIDKey struct{}

// SessionCookie middleware issues a long-lived cookie ("nikki_sid") on the
// first visit and stores its value in the request context.
func SessionCookie(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sid string
		if c, err := r.Cookie("nikki_sid"); err == nil && c.Value != "" {
			sid = c.Value
		} else {
			b := make([]byte, 16)
			rand.Read(b)
			sid = hex.EncodeToString(b)
			http.SetCookie(w, &http.Cookie{
				Name:     "nikki_sid",
				Value:    sid,
				MaxAge:   365 * 24 * 60 * 60,
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
				Path:     "/",
			})
		}
		ctx := context.WithValue(r.Context(), sessionIDKey{}, sid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// SessionID returns the session ID stored in the request context by
// SessionCookie, or an empty string if the middleware was not applied.
func SessionID(r *http.Request) string {
	if v := r.Context().Value(sessionIDKey{}); v != nil {
		return v.(string)
	}
	return ""
}

// ClientIP extracts the client IP, honouring X-Forwarded-For from reverse
// proxies such as Caddy.
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i != -1 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
