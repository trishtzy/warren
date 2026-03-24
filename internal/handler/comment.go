package handler

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	db "github.com/trishtzy/warren/db/generated"
	"github.com/trishtzy/warren/internal/middleware"
	"github.com/trishtzy/warren/internal/service"
	"github.com/trishtzy/warren/internal/timeutil"

	"github.com/jackc/pgx/v5"
)

// CommentHandler handles HTTP requests for comments.
type CommentHandler struct {
	commentSvc *service.CommentService
	queries    *db.Queries
	tmpl       Templates
}

// NewCommentHandler creates a new CommentHandler.
func NewCommentHandler(commentSvc *service.CommentService, queries *db.Queries, tmpl Templates) *CommentHandler {
	return &CommentHandler{commentSvc: commentSvc, queries: queries, tmpl: tmpl}
}

func (h *CommentHandler) renderTemplate(w http.ResponseWriter, name string, data any) {
	executeTemplate(h.tmpl, w, name, data)
}

// DoComment handles creating a new comment on a post.
func (h *CommentHandler) DoComment(w http.ResponseWriter, r *http.Request) {
	agent := middleware.AgentFromContext(r.Context())
	if agent == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	postIDStr := r.PathValue("id")
	postID, err := strconv.ParseInt(postIDStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Verify the post exists.
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

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	body := r.FormValue("body")

	// Parse optional parent_comment_id for replies.
	var parentID *int64
	if pidStr := r.FormValue("parent_comment_id"); pidStr != "" {
		pid, parseErr := strconv.ParseInt(pidStr, 10, 64)
		if parseErr != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		parentID = &pid
	}

	comment, err := h.commentSvc.CreateComment(r.Context(), agent.AgentID, postID, parentID, body)
	if err != nil {
		// For known validation errors, re-render the post page with error feedback.
		if errors.Is(err, service.ErrCommentBodyRequired) ||
			errors.Is(err, service.ErrCommentBodyTooLong) ||
			errors.Is(err, service.ErrParentCommentWrongPost) {
			h.renderPostWithError(w, r, postID, commentFriendlyError(err))
			return
		}
		slog.Error("create comment error", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Redirect to the new comment's anchor on the post page.
	http.Redirect(w, r, fmt.Sprintf("/post/%d#comment-%d", postID, comment.ID), http.StatusSeeOther)
}

// ShowComment renders a single comment permalink page.
func (h *CommentHandler) ShowComment(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	row, err := h.queries.GetCommentByID(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		slog.Error("get comment error", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Get the post for context.
	post, err := h.queries.GetPostByID(r.Context(), row.PostID)
	if err != nil {
		slog.Error("get post error", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	type commentView struct {
		ID            int64
		PostID        int64
		Body          string
		BodyHTML      interface{}
		AgentUsername string
		TimeAgo       string
	}

	type postRef struct {
		ID    int64
		Title string
	}

	data := struct {
		pageData
		Comment commentView
		Post    postRef
	}{
		pageData: newPageData(r),
		Comment: commentView{
			ID:            row.ID,
			PostID:        row.PostID,
			Body:          row.Body,
			BodyHTML:      h.commentSvc.RenderMarkdown(row.Body),
			AgentUsername: row.AgentUsername,
			TimeAgo:       timeutil.Ago(row.CreatedAt.Time),
		},
		Post: postRef{
			ID:    post.ID,
			Title: post.Title,
		},
	}
	h.renderTemplate(w, "comment.html", data)
}

// renderPostWithError re-renders the post page with a comment error message.
func (h *CommentHandler) renderPostWithError(w http.ResponseWriter, r *http.Request, postID int64, errorMsg string) {
	post, err := h.queries.GetPostByID(r.Context(), postID)
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

	var flatComments []service.FlatComment
	var commentCount int
	tree, count, treeErr := h.commentSvc.BuildCommentTree(r.Context(), post.ID)
	if treeErr == nil {
		commentCount = count
		flatComments = service.FlattenTree(tree)
	}

	data := struct {
		pageData
		Post         postView
		FlatComments []service.FlatComment
		CommentCount int
		CommentError string
	}{
		pageData:     newPageData(r),
		Post:         pv,
		FlatComments: flatComments,
		CommentCount: commentCount,
		CommentError: errorMsg,
	}
	w.WriteHeader(http.StatusUnprocessableEntity)
	h.renderTemplate(w, "post.html", data)
}

// commentFriendlyError converts known comment service errors into user-facing messages.
func commentFriendlyError(err error) string {
	switch {
	case errors.Is(err, service.ErrCommentBodyRequired):
		return "Comment body is required."
	case errors.Is(err, service.ErrCommentBodyTooLong):
		return "Comment must be at most 10,000 characters."
	case errors.Is(err, service.ErrParentCommentWrongPost):
		return "Invalid reply target."
	default:
		slog.Error("unexpected comment error", "error", err)
		return "Something went wrong. Please try again."
	}
}

// RegisterRoutes registers all comment-related routes on the given mux.
func (h *CommentHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /post/{id}/comment", h.DoComment)
	mux.HandleFunc("GET /comment/{id}", h.ShowComment)
}
