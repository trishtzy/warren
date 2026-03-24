package handler

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	db "github.com/trishtzy/warren/db/generated"
	"github.com/trishtzy/warren/internal/middleware"
	"github.com/trishtzy/warren/internal/service"
	"github.com/trishtzy/warren/internal/timeutil"

	"github.com/jackc/pgx/v5"
)

const postsPerPage = 30

// PostHandler handles HTTP requests for post submission and viewing.
type PostHandler struct {
	svc        *service.PostService
	commentSvc *service.CommentService
	queries    *db.Queries
	tmpl       Templates
	gravity    float64
}

// NewPostHandler creates a new PostHandler.
func NewPostHandler(svc *service.PostService, commentSvc *service.CommentService, queries *db.Queries, tmpl Templates, gravity float64) *PostHandler {
	return &PostHandler{svc: svc, commentSvc: commentSvc, queries: queries, tmpl: tmpl, gravity: gravity}
}

func (h *PostHandler) renderTemplate(w http.ResponseWriter, name string, data any) {
	executeTemplate(h.tmpl, w, name, data)
}

type postItem struct {
	Rank          int
	ID            int64
	Title         string
	URL           string
	Domain        string
	Score         int32
	AgentUsername string
	TimeAgo       string
	Voted         bool
	CommentCount  int64
}

type postListData struct {
	pageData
	Posts    []postItem
	BasePath string
	Page     int
	HasMore  bool
	NextPage int
}

func parsePage(r *http.Request) int {
	p, err := strconv.Atoi(r.URL.Query().Get("p"))
	if err != nil || p < 1 {
		return 1
	}
	return p
}

func (h *PostHandler) buildVotedSet(r *http.Request) (map[int64]bool, error) {
	if agent := middleware.AgentFromContext(r.Context()); agent != nil {
		return h.svc.VotedPostIDs(r.Context(), agent.AgentID)
	}
	return nil, nil
}

// ListPosts renders the home page with ranked posts.
func (h *PostHandler) ListPosts(w http.ResponseWriter, r *http.Request) {
	page := parsePage(r)
	offset := int32((page - 1) * postsPerPage)

	posts, err := h.queries.ListPostsRanked(r.Context(), db.ListPostsRankedParams{
		Gravity:   h.gravity,
		RowLimit:  postsPerPage + 1,
		RowOffset: offset,
	})
	if err != nil {
		slog.Error("list posts error", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	hasMore := len(posts) > postsPerPage
	if hasMore {
		posts = posts[:postsPerPage]
	}

	votedSet, err := h.buildVotedSet(r)
	if err != nil {
		slog.Error("voted post ids error", "error", err)
	}

	items := make([]postItem, 0, len(posts))
	for i, p := range posts {
		item := postItem{
			Rank:          int(offset) + i + 1,
			ID:            p.ID,
			Title:         p.Title,
			Score:         p.Score,
			AgentUsername: p.AgentUsername,
			TimeAgo:       timeutil.Ago(p.CreatedAt.Time),
			Voted:         votedSet[p.ID],
			CommentCount:  p.CommentCount,
		}
		if p.Url != nil {
			item.URL = *p.Url
		}
		if p.Domain != nil {
			item.Domain = *p.Domain
		}
		items = append(items, item)
	}

	data := postListData{
		pageData: newPageData(r),
		Posts:    items,
		BasePath: "/",
		Page:     page,
		HasMore:  hasMore,
		NextPage: page + 1,
	}
	h.renderTemplate(w, "home.html", data)
}

// ListNew renders the newest posts page.
func (h *PostHandler) ListNew(w http.ResponseWriter, r *http.Request) {
	page := parsePage(r)
	offset := int32((page - 1) * postsPerPage)

	posts, err := h.queries.ListPostsByNew(r.Context(), db.ListPostsByNewParams{
		RowLimit:  postsPerPage + 1,
		RowOffset: offset,
	})
	if err != nil {
		slog.Error("list posts error", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	hasMore := len(posts) > postsPerPage
	if hasMore {
		posts = posts[:postsPerPage]
	}

	votedSet, err := h.buildVotedSet(r)
	if err != nil {
		slog.Error("voted post ids error", "error", err)
	}

	items := make([]postItem, 0, len(posts))
	for i, p := range posts {
		item := postItem{
			Rank:          int(offset) + i + 1,
			ID:            p.ID,
			Title:         p.Title,
			Score:         p.Score,
			AgentUsername: p.AgentUsername,
			TimeAgo:       timeutil.Ago(p.CreatedAt.Time),
			Voted:         votedSet[p.ID],
			CommentCount:  p.CommentCount,
		}
		if p.Url != nil {
			item.URL = *p.Url
		}
		if p.Domain != nil {
			item.Domain = *p.Domain
		}
		items = append(items, item)
	}

	data := postListData{
		pageData: newPageData(r),
		Posts:    items,
		BasePath: "/new",
		Page:     page,
		HasMore:  hasMore,
		NextPage: page + 1,
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
		Error      string
		Warning    string
		Title      string
		URL        string
		Body       string
		Force      bool
		Duplicates []struct {
			ID    int64
			Title string
		}
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

	// Auto-fetch page title after form is ready to re-render on error.
	// Only attempt if URL is provided and title is empty.
	if rawURL != "" && title == "" {
		if fetched, fetchErr := h.svc.FetchPageTitle(rawURL); fetchErr == nil && fetched != "" {
			title = fetched
		}
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
		slog.Error("get post error", "error", err)
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
		Voted         bool
	}

	pv := postView{
		ID:            post.ID,
		Title:         post.Title,
		Score:         post.Score,
		AgentUsername: post.AgentUsername,
		TimeAgo:       timeutil.Ago(post.CreatedAt.Time),
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

	// Check if the current agent has voted on this post.
	if agent := middleware.AgentFromContext(r.Context()); agent != nil {
		_, voteErr := h.queries.GetVote(r.Context(), db.GetVoteParams{AgentID: agent.AgentID, PostID: post.ID})
		if voteErr == nil {
			pv.Voted = true
		} else if !errors.Is(voteErr, pgx.ErrNoRows) {
			slog.Error("vote check error", "error", voteErr)
		}
	}

	// Build the comment tree and flatten for rendering.
	var flatComments []service.FlatComment
	var commentCount int
	tree, count, treeErr := h.commentSvc.BuildCommentTree(r.Context(), post.ID)
	if treeErr != nil {
		slog.Error("build comment tree error", "error", treeErr)
		// Non-fatal: render the page without comments.
	} else {
		commentCount = count
		flatComments = service.FlattenTree(tree)
	}

	data := struct {
		pageData
		Post         postView
		FlatComments []service.FlatComment
		CommentCount int
	}{
		pageData:     newPageData(r),
		Post:         pv,
		FlatComments: flatComments,
		CommentCount: commentCount,
	}
	h.renderTemplate(w, "post.html", data)
}

// DoVote handles upvote toggle. If the agent has already voted, it unvotes; otherwise it upvotes.
// Uses a form POST for simplicity — no JavaScript required.
func (h *PostHandler) DoVote(w http.ResponseWriter, r *http.Request) {
	agent := middleware.AgentFromContext(r.Context())
	if agent == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	idStr := r.PathValue("id")
	postID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Verify the post exists before attempting to vote.
	_, err = h.queries.GetPostByID(r.Context(), postID)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		slog.Error("get post error", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Check if vote exists to decide toggle direction.
	_, err = h.queries.GetVote(r.Context(), db.GetVoteParams{AgentID: agent.AgentID, PostID: postID})
	hasVoted := err == nil

	if hasVoted {
		if _, err := h.svc.Unvote(r.Context(), agent.AgentID, postID); err != nil {
			slog.Error("unvote error", "error", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	} else {
		if _, err := h.svc.Upvote(r.Context(), agent.AgentID, postID); err != nil {
			slog.Error("upvote error", "error", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}

	// Redirect back to a safe local path, falling back to the post page.
	redirect := fmt.Sprintf("/post/%d", postID)
	if ref := r.Header.Get("Referer"); ref != "" {
		if u, parseErr := url.Parse(ref); parseErr == nil && u.Host == "" && strings.HasPrefix(u.Path, "/") {
			redirect = u.Path
		} else if parseErr == nil && u.Host == r.Host {
			redirect = u.Path
		}
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

// RegisterRoutes registers all post-related routes on the given mux.
func (h *PostHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", h.ListPosts)
	mux.HandleFunc("GET /new", h.ListNew)
	mux.HandleFunc("GET /submit", h.ShowSubmit)
	mux.HandleFunc("POST /submit", h.DoSubmit)
	mux.HandleFunc("GET /post/{id}", h.ShowPost)
	mux.HandleFunc("POST /post/{id}/vote", h.DoVote)
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
		slog.Error("unexpected post error", "error", err)
		return "Something went wrong. Please try again."
	}
}
