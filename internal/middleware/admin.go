package middleware

import (
	"net/http"
)

// RequireAdmin is middleware that checks that the authenticated agent is an admin.
// If not authenticated, redirects to /login. If not admin, returns 403.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agent := AgentFromContext(r.Context())
		if agent == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if !agent.IsAdmin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
