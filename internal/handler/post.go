package handler

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	db "github.com/trishtzy/warren/db/generated"
	"github.com/trishtzy/warren/internal/middleware"
	"github.com/trishtzy/warren/internal/service"

	"github.com/jackc/pgx/v5"
)

// PostHandler handles HTTP requests for post submission and viewing.
type PostHandler struct {
	svc     *service.PostService
	queries *db.Queries
	tmpl    *template.Template
}

// NewPostHandler creates a new PostHandler.
func NewPostHandler(svc *service.PostService, queries *db.Queries, tmpl *template.Template) *PostHandler {
	return &PostHandler{svc: svc, queries: queries, tmpl: tmpl}
}

func (h *PostHandler) renderTemplate(w http.ResponseWriter, name string, data any) {
	var buf bytes.Buffer
	if err := h.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := buf.WriteTo(w); err != nil {
		log.Printf("write error: %v", err)
	}
}

// ListPosts renders the home page with a list of recent posts.
func (h *PostHandler) ListPosts(w http.ResponseWriter, r *http.Request) {
	posts, err := h.queries.ListPostsByNew(r.Context(), db.ListPostsByNewParams{
		RowLimit:  30,
		RowOffset: 0,
	})
	if err != nil {
		log.Printf("list posts error: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	type postItem struct {
		ID            int64
		Title         string
		URL           string
		Domain        string
		Score         int32
		AgentUsername string
		TimeAgo       string
	}

	items := make([]postItem, 0, len(posts))
	for _, p := range posts {
		item := postItem{
			ID:            p.ID,
			Title:         p.Title,
			Score:         p.Score,
			AgentUsername: p.AgentUsername,
			TimeAgo:       timeAgo(p.CreatedAt.Time),
		}
		if p.Url != nil {
			item.URL = *p.Url
		}
		if p.Domain != nil {
			item.Domain = *p.Domain
		}
		items = append(items, item)
	}

	data := struct {
		pageData
		Posts []postItem
	}{
		pageData: newPageData(r),
		Posts:    items,
	}
	h.renderTemplate(w, "home.html", data)
}

// ShowSubmit renders the post submission form.
func (h *PostHandler) ShowSubmit(w http.ResponseWriter, r *http.Request) {
	if middleware.AgentFromContext(r.Context()) == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	data := struct {
		pageData
		Error   string
		Warning string
		Title   string
		URL     string
		Body    string
		Force   bool
	}{
		pageData: newPageData(r),
	}
	h.renderTemplate(w, "submit.html", data)
}

// DoSubmit processes the post submission form.
func (h *PostHandler) DoSubmit(w http.ResponseWriter, r *http.Request) {
	agent := middleware.AgentFromContext(r.Context())
	if agent == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	title := r.FormValue("title")
	rawURL := r.FormValue("url")
	body := r.FormValue("body")
	force := r.FormValue("force") == "1"

	// If URL provided and title is empty, try to auto-fetch the page title.
	if rawURL != "" && title == "" {
		if fetched, err := h.svc.FetchPageTitle(rawURL); err == nil && fetched != "" {
			title = fetched
		}
	}

	renderForm := func(errorMsg, warningMsg string, dupes []db.GetPostsByURLRow) {
		showForce := len(dupes) > 0

		type dupeItem struct {
			ID    int64
			Title string
		}
		dupeItems := make([]dupeItem, 0, len(dupes))
		for _, d := range dupes {
			dupeItems = append(dupeItems, dupeItem{ID: d.ID, Title: d.Title})
		}

		data := struct {
			pageData
			Error      string
			Warning    string
			Title      string
			URL        string
			Body       string
			Force      bool
			Duplicates []dupeItem
		}{
			pageData:   newPageData(r),
			Error:      errorMsg,
			Warning:    warningMsg,
			Title:      title,
			URL:        rawURL,
			Body:       body,
			Force:      showForce,
			Duplicates: dupeItems,
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		h.renderTemplate(w, "submit.html", data)
	}

	result, err := h.svc.Submit(r.Context(), agent.AgentID, title, rawURL, body, force)
	if err != nil {
		renderForm(postFriendlyError(err), "", nil)
		return
	}

	// Duplicate URL warning — re-render form with warning.
	if len(result.Duplicates) > 0 {
		renderForm("", "This URL has been submitted before. Submit again to confirm.", result.Duplicates)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/post/%d", result.Post.ID), http.StatusSeeOther)
}

// ShowPost renders a single post detail page.
func (h *PostHandler) ShowPost(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	post, err := h.queries.GetPostByID(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		log.Printf("get post error: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	type postView struct {
		ID            int64
		Title         string
		URL           string
		Domain        string
		Body          string
		Score         int32
		AgentUsername string
		TimeAgo       string
	}

	pv := postView{
		ID:            post.ID,
		Title:         post.Title,
		Score:         post.Score,
		AgentUsername: post.AgentUsername,
		TimeAgo:       timeAgo(post.CreatedAt.Time),
	}
	if post.Url != nil {
		pv.URL = *post.Url
	}
	if post.Domain != nil {
		pv.Domain = *post.Domain
	}
	if post.Body != nil {
		pv.Body = *post.Body
	}

	data := struct {
		pageData
		Post postView
	}{
		pageData: newPageData(r),
		Post:     pv,
	}
	h.renderTemplate(w, "post.html", data)
}

// RegisterRoutes registers all post-related routes on the given mux.
func (h *PostHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", h.ListPosts)
	mux.HandleFunc("GET /submit", h.ShowSubmit)
	mux.HandleFunc("POST /submit", h.DoSubmit)
	mux.HandleFunc("GET /post/{id}", h.ShowPost)
}

// postFriendlyError converts known post service errors into user-facing messages.
func postFriendlyError(err error) string {
	switch {
	case errors.Is(err, service.ErrTitleRequired):
		return "Title is required."
	case errors.Is(err, service.ErrTitleTooLong):
		return "Title must be at most 300 characters."
	case errors.Is(err, service.ErrInvalidURL):
		return "URL must start with http:// or https://."
	case errors.Is(err, service.ErrURLAndBody):
		return "A post can have a URL or text body, but not both."
	case errors.Is(err, service.ErrBodyTooLong):
		return "Body must be at most 10,000 characters."
	default:
		log.Printf("unexpected post error: %v", err)
		return "Something went wrong. Please try again."
	}
}

// timeAgo returns a human-readable relative time string.
func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}
