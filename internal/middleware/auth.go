package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"

	db "github.com/trishtzy/warren/db/generated"
)

// contextKey is an unexported type for context keys in this package.
type contextKey struct{}

// agentKey is the context key for AgentInfo.
var agentKey = contextKey{}

// AgentInfo holds the authenticated agent's information extracted from a session.
type AgentInfo struct {
	AgentID  int64
	Username string
	IsAdmin  bool
	Banned   bool
}

// AgentFromContext returns the AgentInfo stored in the context, or nil if
// the request is not authenticated.
func AgentFromContext(ctx context.Context) *AgentInfo {
	info, _ := ctx.Value(agentKey).(*AgentInfo)
	return info
}

// hashToken returns the hex-encoded SHA-256 hash of a session token.
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// Auth returns middleware that reads the "session" cookie, validates it against
// the database, and stores the agent info in the request context. If the cookie
// is missing or the session is invalid, the request proceeds without
// authentication (for public pages).
func Auth(queries *db.Queries) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("session")
			if err != nil {
				// No session cookie — continue unauthenticated.
				next.ServeHTTP(w, r)
				return
			}

			// H3: Hash the cookie value before querying the database.
			session, err := queries.GetSession(r.Context(), hashToken(cookie.Value))
			if err != nil {
				// Invalid or expired session — continue unauthenticated.
				next.ServeHTTP(w, r)
				return
			}

			info := &AgentInfo{
				AgentID:  session.AgentID,
				Username: session.AgentUsername,
				IsAdmin:  session.IsAdmin,
				Banned:   session.Banned,
			}

			// H2: If the agent is banned, do not inject them into context.
			// They proceed as an unauthenticated user.
			if info.Banned {
				next.ServeHTTP(w, r)
				return
			}

			ctx := context.WithValue(r.Context(), agentKey, info)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAuth is middleware that checks for an authenticated agent in the
// context. If not present, it redirects to /login.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if AgentFromContext(r.Context()) == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}
