package handler

import (
	"bytes"
	"errors"
	"html/template"
	"log"
	"net/http"

	db "github.com/trishtzy/warren/db/generated"
	"github.com/trishtzy/warren/internal/middleware"
	"github.com/trishtzy/warren/internal/service"

	"github.com/jackc/pgx/v5"
)

const sessionCookieMaxAge = 30 * 24 * 3600 // 30 days in seconds

// AuthHandler handles HTTP requests for registration, login, logout, and profiles.
type AuthHandler struct {
	svc     *service.AuthService
	queries *db.Queries
	tmpl    *template.Template
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(svc *service.AuthService, queries *db.Queries, tmpl *template.Template) *AuthHandler {
	return &AuthHandler{svc: svc, queries: queries, tmpl: tmpl}
}

// pageData is the base data passed to every template.
type pageData struct {
	Agent *middleware.AgentInfo
}

func newPageData(r *http.Request) pageData {
	return pageData{Agent: middleware.AgentFromContext(r.Context())}
}

// renderTemplate executes a named template into a buffer first, then writes
// to the response. This prevents partial responses when template execution fails (M3).
func (h *AuthHandler) renderTemplate(w http.ResponseWriter, name string, data any) {
	executeTemplate(h.tmpl, w, name, data)
}

// executeTemplate is the shared implementation for rendering templates to a response.
func executeTemplate(tmpl *template.Template, w http.ResponseWriter, name string, data any) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := buf.WriteTo(w); err != nil {
		log.Printf("write error: %v", err)
	}
}

// setSessionCookie sets the session cookie on the response.
func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true, // M1: always set Secure flag
		SameSite: http.SameSiteLaxMode,
		MaxAge:   sessionCookieMaxAge,
	})
}

// clearSessionCookie removes the session cookie.
func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// ShowRegister renders the registration form.
func (h *AuthHandler) ShowRegister(w http.ResponseWriter, r *http.Request) {
	// Redirect if already logged in.
	if middleware.AgentFromContext(r.Context()) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	data := struct {
		pageData
		Error    string
		Username string
		Email    string
	}{
		pageData: newPageData(r),
	}
	h.renderTemplate(w, "register.html", data)
}

// DoRegister processes the registration form submission.
// TODO: Add CSRF protection — tracked as a follow-up issue (H4).
func (h *AuthHandler) DoRegister(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	username := r.FormValue("username")
	email := r.FormValue("email")
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")

	renderError := func(msg string) {
		data := struct {
			pageData
			Error    string
			Username string
			Email    string
		}{
			pageData: newPageData(r),
			Error:    msg,
			Username: username,
			Email:    email,
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		h.renderTemplate(w, "register.html", data)
	}

	if password != confirmPassword {
		renderError("Passwords do not match.")
		return
	}

	_, err := h.svc.Register(r.Context(), username, email, password)
	if err != nil {
		renderError(friendlyError(err))
		return
	}

	// Auto-login after registration.
	token, err := h.svc.Login(r.Context(), username, password)
	if err != nil {
		// Registration succeeded but login failed — send to login page.
		log.Printf("auto-login after registration failed: %v", err)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	setSessionCookie(w, token)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ShowLogin renders the login form.
func (h *AuthHandler) ShowLogin(w http.ResponseWriter, r *http.Request) {
	if middleware.AgentFromContext(r.Context()) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	data := struct {
		pageData
		Error      string
		Identifier string
	}{
		pageData: newPageData(r),
	}
	h.renderTemplate(w, "login.html", data)
}

// DoLogin processes the login form submission.
// TODO: Add CSRF protection — tracked as a follow-up issue (H4).
func (h *AuthHandler) DoLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	identifier := r.FormValue("identifier")
	password := r.FormValue("password")

	token, err := h.svc.Login(r.Context(), identifier, password)
	if err != nil {
		data := struct {
			pageData
			Error      string
			Identifier string
		}{
			pageData:   newPageData(r),
			Error:      friendlyError(err),
			Identifier: identifier,
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		h.renderTemplate(w, "login.html", data)
		return
	}

	setSessionCookie(w, token)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// DoLogout processes a logout request.
// TODO: Add CSRF protection — tracked as a follow-up issue (H4).
func (h *AuthHandler) DoLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err == nil {
		if logoutErr := h.svc.Logout(r.Context(), cookie.Value); logoutErr != nil {
			log.Printf("logout error: %v", logoutErr)
		}
	}
	clearSessionCookie(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ShowProfile renders an agent's public profile page.
func (h *AuthHandler) ShowProfile(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	if username == "" {
		http.NotFound(w, r)
		return
	}

	agent, err := h.queries.GetAgentByUsername(r.Context(), username)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		log.Printf("get agent error: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	type profileView struct {
		Username string
		JoinedAt string
	}

	data := struct {
		pageData
		Profile profileView
	}{
		pageData: newPageData(r),
		Profile: profileView{
			Username: agent.Username,
			JoinedAt: agent.CreatedAt.Time.Format("January 2, 2006"),
		},
	}
	h.renderTemplate(w, "profile.html", data)
}

// friendlyError converts known service errors into user-facing messages.
func friendlyError(err error) string {
	switch {
	case errors.Is(err, service.ErrInvalidUsername):
		return "Username must be 3-30 characters (letters, numbers, underscores only)."
	case errors.Is(err, service.ErrInvalidEmail):
		return "Please enter a valid email address."
	case errors.Is(err, service.ErrPasswordTooShort):
		return "Password must be at least 8 characters."
	case errors.Is(err, service.ErrPasswordTooLong):
		return "Password must be at most 72 characters."
	case errors.Is(err, service.ErrUsernameTaken):
		return "That username is already taken."
	case errors.Is(err, service.ErrEmailTaken):
		return "That email is already registered."
	case errors.Is(err, service.ErrInvalidCredentials):
		return "Invalid username/email or password."
	default:
		log.Printf("unexpected error: %v", err)
		return "Something went wrong. Please try again."
	}
}

// RegisterRoutes registers all auth-related routes on the given mux.
func (h *AuthHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /register", h.ShowRegister)
	mux.HandleFunc("POST /register", h.DoRegister)
	mux.HandleFunc("GET /login", h.ShowLogin)
	mux.HandleFunc("POST /login", h.DoLogin)
	mux.HandleFunc("POST /logout", h.DoLogout)
	mux.HandleFunc("GET /agent/{username}", h.ShowProfile)
}
