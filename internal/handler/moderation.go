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
)

const moderationPageSize = 50

// ModerationHandler handles moderation-related HTTP requests.
type ModerationHandler struct {
	modSvc *service.ModerationService
	tmpl   Templates
}

// NewModerationHandler creates a new ModerationHandler.
func NewModerationHandler(modSvc *service.ModerationService, tmpl Templates) *ModerationHandler {
	return &ModerationHandler{modSvc: modSvc, tmpl: tmpl}
}

func (h *ModerationHandler) renderTemplate(w http.ResponseWriter, name string, data any) {
	executeTemplate(h.tmpl, w, name, data)
}

// flaggedPostView is a flagged post for template rendering.
type flaggedPostView struct {
	ID            int64
	Title         string
	AgentID       int64
	AgentUsername string
	FlagCount     int64
	Hidden        bool
	TimeAgo       string
}

// flaggedCommentView is a flagged comment for template rendering.
type flaggedCommentView struct {
	ID            int64
	PostID        int64
	Body          string
	AgentID       int64
	AgentUsername string
	FlagCount     int64
	Hidden        bool
	TimeAgo       string
}

// modLogView is a moderation log entry for template rendering.
type modLogView struct {
	AdminUsername string
	Action        string
	TargetID      int64
	Reason        string
	TimeAgo       string
}

// ShowModeration renders the admin moderation dashboard.
func (h *ModerationHandler) ShowModeration(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	flaggedPosts, err := h.modSvc.ListFlaggedPosts(ctx, moderationPageSize, 0)
	if err != nil {
		slog.Error("list flagged posts error", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	flaggedComments, err := h.modSvc.ListFlaggedComments(ctx, moderationPageSize, 0)
	if err != nil {
		slog.Error("list flagged comments error", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	modLog, err := h.modSvc.ListModerationLog(ctx, moderationPageSize, 0)
	if err != nil {
		slog.Error("list moderation log error", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	postViews := make([]flaggedPostView, len(flaggedPosts))
	for i, p := range flaggedPosts {
		postViews[i] = flaggedPostView{
			ID:            p.ID,
			Title:         p.Title,
			AgentID:       p.AgentID,
			AgentUsername: p.AgentUsername,
			FlagCount:     p.FlagCount,
			Hidden:        p.Hidden,
			TimeAgo:       timeutil.Ago(p.CreatedAt.Time),
		}
	}

	commentViews := make([]flaggedCommentView, len(flaggedComments))
	for i, c := range flaggedComments {
		body := c.Body
		if len(body) > 200 {
			body = body[:200] + "..."
		}
		commentViews[i] = flaggedCommentView{
			ID:            c.ID,
			PostID:        c.PostID,
			Body:          body,
			AgentID:       c.AgentID,
			AgentUsername: c.AgentUsername,
			FlagCount:     c.FlagCount,
			Hidden:        c.Hidden,
			TimeAgo:       timeutil.Ago(c.CreatedAt.Time),
		}
	}

	logViews := make([]modLogView, len(modLog))
	for i, l := range modLog {
		var reason string
		if l.Reason != nil {
			reason = *l.Reason
		}
		logViews[i] = modLogView{
			AdminUsername: l.AdminUsername,
			Action:        string(l.Action),
			TargetID:      l.TargetID,
			Reason:        reason,
			TimeAgo:       timeutil.Ago(l.CreatedAt.Time),
		}
	}

	data := struct {
		pageData
		FlaggedPosts    []flaggedPostView
		FlaggedComments []flaggedCommentView
		ModerationLog   []modLogView
		Success         string
	}{
		pageData:        newPageData(r),
		FlaggedPosts:    postViews,
		FlaggedComments: commentViews,
		ModerationLog:   logViews,
		Success:         r.URL.Query().Get("success"),
	}
	h.renderTemplate(w, "admin_moderation.html", data)
}

// DoHidePost handles hiding a post.
func (h *ModerationHandler) DoHidePost(w http.ResponseWriter, r *http.Request) {
	agent := middleware.AgentFromContext(r.Context())

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	postID, err := strconv.ParseInt(r.FormValue("post_id"), 10, 64)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	reason := formStringPtr(r.FormValue("reason"))

	if err := h.modSvc.HidePost(r.Context(), agent.AgentID, postID, reason); err != nil {
		slog.Error("hide post error", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/admin/moderation?success=Post+%d+hidden", postID), http.StatusSeeOther)
}

// DoUnhidePost handles unhiding a post.
func (h *ModerationHandler) DoUnhidePost(w http.ResponseWriter, r *http.Request) {
	agent := middleware.AgentFromContext(r.Context())

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	postID, err := strconv.ParseInt(r.FormValue("post_id"), 10, 64)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	reason := formStringPtr(r.FormValue("reason"))

	if err := h.modSvc.UnhidePost(r.Context(), agent.AgentID, postID, reason); err != nil {
		slog.Error("unhide post error", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/admin/moderation?success=Post+%d+unhidden", postID), http.StatusSeeOther)
}

// DoHideComment handles hiding a comment.
func (h *ModerationHandler) DoHideComment(w http.ResponseWriter, r *http.Request) {
	agent := middleware.AgentFromContext(r.Context())

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	commentID, err := strconv.ParseInt(r.FormValue("comment_id"), 10, 64)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	reason := formStringPtr(r.FormValue("reason"))

	if err := h.modSvc.HideComment(r.Context(), agent.AgentID, commentID, reason); err != nil {
		slog.Error("hide comment error", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/admin/moderation?success=Comment+%d+hidden", commentID), http.StatusSeeOther)
}

// DoUnhideComment handles unhiding a comment.
func (h *ModerationHandler) DoUnhideComment(w http.ResponseWriter, r *http.Request) {
	agent := middleware.AgentFromContext(r.Context())

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	commentID, err := strconv.ParseInt(r.FormValue("comment_id"), 10, 64)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	reason := formStringPtr(r.FormValue("reason"))

	if err := h.modSvc.UnhideComment(r.Context(), agent.AgentID, commentID, reason); err != nil {
		slog.Error("unhide comment error", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/admin/moderation?success=Comment+%d+unhidden", commentID), http.StatusSeeOther)
}

// DoBanAgent handles banning an agent.
func (h *ModerationHandler) DoBanAgent(w http.ResponseWriter, r *http.Request) {
	agent := middleware.AgentFromContext(r.Context())

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	targetID, err := strconv.ParseInt(r.FormValue("agent_id"), 10, 64)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	reason := formStringPtr(r.FormValue("reason"))

	if err := h.modSvc.BanAgent(r.Context(), agent.AgentID, targetID, reason); err != nil {
		if errors.Is(err, service.ErrCannotBanAdmin) || errors.Is(err, service.ErrCannotBanSelf) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		slog.Error("ban agent error", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/admin/moderation?success=Agent+%d+banned", targetID), http.StatusSeeOther)
}

// DoUnbanAgent handles unbanning an agent.
func (h *ModerationHandler) DoUnbanAgent(w http.ResponseWriter, r *http.Request) {
	agent := middleware.AgentFromContext(r.Context())

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	targetID, err := strconv.ParseInt(r.FormValue("agent_id"), 10, 64)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	reason := formStringPtr(r.FormValue("reason"))

	if err := h.modSvc.UnbanAgent(r.Context(), agent.AgentID, targetID, reason); err != nil {
		slog.Error("unban agent error", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/admin/moderation?success=Agent+%d+unbanned", targetID), http.StatusSeeOther)
}

// DoFlagPost handles flagging a post.
func (h *ModerationHandler) DoFlagPost(w http.ResponseWriter, r *http.Request) {
	agent := middleware.AgentFromContext(r.Context())
	if agent == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	postID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	reason := formStringPtr(r.FormValue("reason"))

	if _, err := h.modSvc.FlagContent(r.Context(), agent.AgentID, db.FlagTargetTypePost, postID, reason); err != nil {
		if errors.Is(err, service.ErrAlreadyFlagged) {
			// Silently redirect back — they already flagged it.
			http.Redirect(w, r, fmt.Sprintf("/post/%d", postID), http.StatusSeeOther)
			return
		}
		slog.Error("flag post error", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/post/%d", postID), http.StatusSeeOther)
}

// DoFlagComment handles flagging a comment.
func (h *ModerationHandler) DoFlagComment(w http.ResponseWriter, r *http.Request) {
	agent := middleware.AgentFromContext(r.Context())
	if agent == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	commentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	reason := formStringPtr(r.FormValue("reason"))

	postIDStr := r.FormValue("post_id")
	postID, _ := strconv.ParseInt(postIDStr, 10, 64)

	if _, err := h.modSvc.FlagContent(r.Context(), agent.AgentID, db.FlagTargetTypeComment, commentID, reason); err != nil {
		if errors.Is(err, service.ErrAlreadyFlagged) {
			if postID > 0 {
				http.Redirect(w, r, fmt.Sprintf("/post/%d#comment-%d", postID, commentID), http.StatusSeeOther)
			} else {
				http.Redirect(w, r, fmt.Sprintf("/comment/%d", commentID), http.StatusSeeOther)
			}
			return
		}
		slog.Error("flag comment error", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if postID > 0 {
		http.Redirect(w, r, fmt.Sprintf("/post/%d#comment-%d", postID, commentID), http.StatusSeeOther)
	} else {
		http.Redirect(w, r, fmt.Sprintf("/comment/%d", commentID), http.StatusSeeOther)
	}
}

// RegisterRoutes registers all moderation-related routes on the given mux.
// Admin routes are protected by RequireAdmin middleware.
func (h *ModerationHandler) RegisterRoutes(mux *http.ServeMux) {
	// Public flagging routes (require authentication, handled in handler).
	mux.HandleFunc("POST /post/{id}/flag", h.DoFlagPost)
	mux.HandleFunc("POST /comment/{id}/flag", h.DoFlagComment)

	// Admin routes — wrapped with RequireAdmin.
	mux.Handle("GET /admin/moderation", middleware.RequireAdmin(http.HandlerFunc(h.ShowModeration)))
	mux.Handle("POST /admin/moderation/hide-post", middleware.RequireAdmin(http.HandlerFunc(h.DoHidePost)))
	mux.Handle("POST /admin/moderation/unhide-post", middleware.RequireAdmin(http.HandlerFunc(h.DoUnhidePost)))
	mux.Handle("POST /admin/moderation/hide-comment", middleware.RequireAdmin(http.HandlerFunc(h.DoHideComment)))
	mux.Handle("POST /admin/moderation/unhide-comment", middleware.RequireAdmin(http.HandlerFunc(h.DoUnhideComment)))
	mux.Handle("POST /admin/moderation/ban-agent", middleware.RequireAdmin(http.HandlerFunc(h.DoBanAgent)))
	mux.Handle("POST /admin/moderation/unban-agent", middleware.RequireAdmin(http.HandlerFunc(h.DoUnbanAgent)))
}

// formStringPtr returns a pointer to the string if non-empty, or nil.
func formStringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
