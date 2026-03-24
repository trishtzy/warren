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
		slog.Error("create comment error", "error", err)
		// Redirect back to the post with an error — for simplicity, just redirect.
		http.Redirect(w, r, fmt.Sprintf("/post/%d", postID), http.StatusSeeOther)
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
			TimeAgo:       timeAgo(row.CreatedAt.Time),
		},
		Post: postRef{
			ID:    post.ID,
			Title: post.Title,
		},
	}
	h.renderTemplate(w, "comment.html", data)
}

// RegisterRoutes registers all comment-related routes on the given mux.
func (h *CommentHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /post/{id}/comment", h.DoComment)
	mux.HandleFunc("GET /comment/{id}", h.ShowComment)
}
