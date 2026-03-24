package handler

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	db "github.com/trishtzy/warren/db/generated"
	"github.com/trishtzy/warren/internal/middleware"
	"github.com/trishtzy/warren/internal/service"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// Mock ModerationQuerier for handler tests
// ---------------------------------------------------------------------------

type mockModQuerier struct {
	flags      []db.Flag
	nextFlagID int64
	posts      map[int64]*mockModPost
	comments   map[int64]*mockModComment
	agents     map[int64]*mockModAgent
	sessions   map[int64][]string
	modLogs    []db.ModerationLog
	nextLogID  int64
}

type mockModPost struct {
	id     int64
	hidden bool
	title  string
}

type mockModComment struct {
	id     int64
	postID int64
	hidden bool
	body   string
}

type mockModAgent struct {
	id       int64
	username string
	isAdmin  bool
	banned   bool
}

func newMockModQuerier() *mockModQuerier {
	return &mockModQuerier{
		nextFlagID: 1,
		nextLogID:  1,
		posts:      make(map[int64]*mockModPost),
		comments:   make(map[int64]*mockModComment),
		agents:     make(map[int64]*mockModAgent),
		sessions:   make(map[int64][]string),
	}
}

func (m *mockModQuerier) CreateFlag(_ context.Context, arg db.CreateFlagParams) (db.Flag, error) {
	f := db.Flag{
		ID:         m.nextFlagID,
		AgentID:    arg.AgentID,
		TargetType: arg.TargetType,
		TargetID:   arg.TargetID,
		Reason:     arg.Reason,
	}
	m.flags = append(m.flags, f)
	m.nextFlagID++
	return f, nil
}

func (m *mockModQuerier) HasAgentFlagged(_ context.Context, arg db.HasAgentFlaggedParams) (bool, error) {
	for _, f := range m.flags {
		if f.AgentID == arg.AgentID && f.TargetType == arg.TargetType && f.TargetID == arg.TargetID {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockModQuerier) ListFlaggedPosts(_ context.Context, _ db.ListFlaggedPostsParams) ([]db.ListFlaggedPostsRow, error) {
	return []db.ListFlaggedPostsRow{}, nil
}

func (m *mockModQuerier) ListFlaggedComments(_ context.Context, _ db.ListFlaggedCommentsParams) ([]db.ListFlaggedCommentsRow, error) {
	return []db.ListFlaggedCommentsRow{}, nil
}

func (m *mockModQuerier) UpdatePostHidden(_ context.Context, arg db.UpdatePostHiddenParams) error {
	p, ok := m.posts[arg.ID]
	if !ok {
		return errors.New("post not found")
	}
	p.hidden = arg.Hidden
	return nil
}

func (m *mockModQuerier) UpdateCommentHidden(_ context.Context, arg db.UpdateCommentHiddenParams) error {
	c, ok := m.comments[arg.ID]
	if !ok {
		return errors.New("comment not found")
	}
	c.hidden = arg.Hidden
	return nil
}

func (m *mockModQuerier) UpdateAgentBanned(_ context.Context, arg db.UpdateAgentBannedParams) error {
	a, ok := m.agents[arg.ID]
	if !ok {
		return errors.New("agent not found")
	}
	a.banned = arg.Banned
	return nil
}

func (m *mockModQuerier) DeleteSessionsByAgent(_ context.Context, agentID int64) error {
	delete(m.sessions, agentID)
	return nil
}

func (m *mockModQuerier) GetAgentByIDForAdmin(_ context.Context, id int64) (db.GetAgentByIDForAdminRow, error) {
	a, ok := m.agents[id]
	if !ok {
		return db.GetAgentByIDForAdminRow{}, errors.New("agent not found")
	}
	return db.GetAgentByIDForAdminRow{
		ID:       a.id,
		Username: a.username,
		IsAdmin:  a.isAdmin,
		Banned:   a.banned,
	}, nil
}

func (m *mockModQuerier) CreateModerationLog(_ context.Context, arg db.CreateModerationLogParams) (db.ModerationLog, error) {
	log := db.ModerationLog{
		ID:       m.nextLogID,
		AdminID:  arg.AdminID,
		Action:   arg.Action,
		TargetID: arg.TargetID,
		Reason:   arg.Reason,
	}
	m.modLogs = append(m.modLogs, log)
	m.nextLogID++
	return log, nil
}

func (m *mockModQuerier) ListModerationLog(_ context.Context, _ db.ListModerationLogParams) ([]db.ListModerationLogRow, error) {
	return []db.ListModerationLogRow{}, nil
}

// ExecTx runs fn directly — no real transaction in the mock.
func (m *mockModQuerier) ExecTx(_ context.Context, fn func(service.ModerationQuerier) error) error {
	return fn(m)
}

var _ service.ModerationStore = (*mockModQuerier)(nil)

// Suppress unused import.
var _ = pgtype.Timestamptz{}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func moderationTemplates() Templates {
	tmpl := make(Templates)
	t := template.Must(template.New("admin_moderation.html").Parse(
		`{{define "admin_moderation.html"}}Moderation Dashboard{{end}}`))
	tmpl["admin_moderation.html"] = t
	return tmpl
}

func withAdminAgent(r *http.Request, agentID int64, username string) *http.Request {
	info := &middleware.AgentInfo{AgentID: agentID, Username: username, IsAdmin: true}
	ctx := context.WithValue(r.Context(), middleware.AgentKeyForTest(), info)
	return r.WithContext(ctx)
}

func withRegularAgent(r *http.Request, agentID int64, username string) *http.Request {
	info := &middleware.AgentInfo{AgentID: agentID, Username: username, IsAdmin: false}
	ctx := context.WithValue(r.Context(), middleware.AgentKeyForTest(), info)
	return r.WithContext(ctx)
}

func newModerationHandler(mq *mockModQuerier) *ModerationHandler {
	svc := service.NewModerationService(mq)
	return NewModerationHandler(svc, moderationTemplates())
}

// ---------------------------------------------------------------------------
// Flagging routes
// ---------------------------------------------------------------------------

func TestDoFlagPost_RequiresAuth(t *testing.T) {
	mq := newMockModQuerier()
	h := newModerationHandler(mq)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	form := url.Values{"reason": {"spam"}}
	req := httptest.NewRequest("POST", "/post/1/flag", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// No agent in context.

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %q", loc)
	}
}

func TestDoFlagPost_AuthenticatedSuccess(t *testing.T) {
	mq := newMockModQuerier()
	mq.posts[1] = &mockModPost{id: 1, title: "Test Post"}
	h := newModerationHandler(mq)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	form := url.Values{"reason": {"spam"}}
	req := httptest.NewRequest("POST", "/post/1/flag", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withRegularAgent(req, 5, "flagger")

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/post/1" {
		t.Errorf("expected redirect to /post/1, got %q", loc)
	}

	// Verify flag was created.
	if len(mq.flags) != 1 {
		t.Fatalf("expected 1 flag, got %d", len(mq.flags))
	}
	if mq.flags[0].TargetType != db.FlagTargetTypePost {
		t.Errorf("target_type = %q, want %q", mq.flags[0].TargetType, db.FlagTargetTypePost)
	}
	if mq.flags[0].TargetID != 1 {
		t.Errorf("target_id = %d, want 1", mq.flags[0].TargetID)
	}
}

func TestDoFlagPost_AlreadyFlagged(t *testing.T) {
	mq := newMockModQuerier()
	mq.posts[1] = &mockModPost{id: 1, title: "Test Post"}
	h := newModerationHandler(mq)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Flag once.
	form := url.Values{"reason": {"spam"}}
	req := httptest.NewRequest("POST", "/post/1/flag", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withRegularAgent(req, 5, "flagger")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	// Flag again — should silently redirect.
	form = url.Values{"reason": {"spam again"}}
	req = httptest.NewRequest("POST", "/post/1/flag", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withRegularAgent(req, 5, "flagger")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/post/1" {
		t.Errorf("expected redirect to /post/1, got %q", loc)
	}
	// Only one flag should exist.
	if len(mq.flags) != 1 {
		t.Errorf("expected 1 flag (duplicate ignored), got %d", len(mq.flags))
	}
}

func TestDoFlagPost_InvalidID(t *testing.T) {
	mq := newMockModQuerier()
	h := newModerationHandler(mq)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	form := url.Values{"reason": {"spam"}}
	req := httptest.NewRequest("POST", "/post/abc/flag", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withRegularAgent(req, 5, "flagger")

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestDoFlagComment_RequiresAuth(t *testing.T) {
	mq := newMockModQuerier()
	h := newModerationHandler(mq)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	form := url.Values{"reason": {"spam"}, "post_id": {"1"}}
	req := httptest.NewRequest("POST", "/comment/1/flag", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %q", loc)
	}
}

func TestDoFlagComment_AuthenticatedSuccess(t *testing.T) {
	mq := newMockModQuerier()
	mq.comments[1] = &mockModComment{id: 1, postID: 5, body: "bad comment"}
	h := newModerationHandler(mq)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	form := url.Values{"reason": {"offensive"}, "post_id": {"5"}}
	req := httptest.NewRequest("POST", "/comment/1/flag", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withRegularAgent(req, 5, "flagger")

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rr.Code)
	}
	expectedLoc := "/post/5#comment-1"
	if loc := rr.Header().Get("Location"); loc != expectedLoc {
		t.Errorf("expected redirect to %q, got %q", expectedLoc, loc)
	}

	if len(mq.flags) != 1 {
		t.Fatalf("expected 1 flag, got %d", len(mq.flags))
	}
	if mq.flags[0].TargetType != db.FlagTargetTypeComment {
		t.Errorf("target_type = %q, want %q", mq.flags[0].TargetType, db.FlagTargetTypeComment)
	}
}

// ---------------------------------------------------------------------------
// Admin moderation page
// ---------------------------------------------------------------------------

func TestShowModeration_AdminAccess(t *testing.T) {
	mq := newMockModQuerier()
	h := newModerationHandler(mq)

	req := httptest.NewRequest("GET", "/admin/moderation", nil)
	req = withAdminAgent(req, 10, "admin")

	rr := httptest.NewRecorder()
	h.ShowModeration(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Moderation Dashboard") {
		t.Error("expected response to contain 'Moderation Dashboard'")
	}
}

func TestShowModeration_RequireAdminMiddleware(t *testing.T) {
	mq := newMockModQuerier()
	h := newModerationHandler(mq)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Test unauthenticated access.
	req := httptest.NewRequest("GET", "/admin/moderation", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("unauthenticated: expected 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/login" {
		t.Errorf("unauthenticated: expected redirect to /login, got %q", loc)
	}
}

func TestShowModeration_NonAdminForbidden(t *testing.T) {
	mq := newMockModQuerier()
	h := newModerationHandler(mq)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/admin/moderation", nil)
	req = withRegularAgent(req, 5, "regular_user")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("non-admin: expected 403, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Admin hide/unhide post
// ---------------------------------------------------------------------------

func TestDoHidePost_Success(t *testing.T) {
	mq := newMockModQuerier()
	mq.posts[1] = &mockModPost{id: 1, title: "Test Post"}
	h := newModerationHandler(mq)

	form := url.Values{"post_id": {"1"}, "reason": {"spam"}}
	req := httptest.NewRequest("POST", "/admin/moderation/hide-post", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withAdminAgent(req, 10, "admin")

	rr := httptest.NewRecorder()
	h.DoHidePost(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rr.Code)
	}
	if !strings.Contains(rr.Header().Get("Location"), "success=Post+1+hidden") {
		t.Errorf("expected success redirect, got %q", rr.Header().Get("Location"))
	}

	// Verify post is hidden.
	if !mq.posts[1].hidden {
		t.Error("expected post to be hidden")
	}

	// Verify moderation log.
	if len(mq.modLogs) != 1 {
		t.Fatalf("expected 1 moderation log, got %d", len(mq.modLogs))
	}
	if mq.modLogs[0].Action != db.ModerationActionHidePost {
		t.Errorf("action = %q, want %q", mq.modLogs[0].Action, db.ModerationActionHidePost)
	}
}

func TestDoHidePost_InvalidPostID(t *testing.T) {
	mq := newMockModQuerier()
	h := newModerationHandler(mq)

	form := url.Values{"post_id": {"abc"}}
	req := httptest.NewRequest("POST", "/admin/moderation/hide-post", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withAdminAgent(req, 10, "admin")

	rr := httptest.NewRecorder()
	h.DoHidePost(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestDoUnhidePost_Success(t *testing.T) {
	mq := newMockModQuerier()
	mq.posts[1] = &mockModPost{id: 1, title: "Test Post", hidden: true}
	h := newModerationHandler(mq)

	form := url.Values{"post_id": {"1"}}
	req := httptest.NewRequest("POST", "/admin/moderation/unhide-post", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withAdminAgent(req, 10, "admin")

	rr := httptest.NewRecorder()
	h.DoUnhidePost(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rr.Code)
	}
	if mq.posts[1].hidden {
		t.Error("expected post to be unhidden")
	}
}

// ---------------------------------------------------------------------------
// Admin hide/unhide comment
// ---------------------------------------------------------------------------

func TestDoHideComment_Success(t *testing.T) {
	mq := newMockModQuerier()
	mq.comments[1] = &mockModComment{id: 1, postID: 1, body: "bad comment"}
	h := newModerationHandler(mq)

	form := url.Values{"comment_id": {"1"}, "reason": {"off-topic"}}
	req := httptest.NewRequest("POST", "/admin/moderation/hide-comment", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withAdminAgent(req, 10, "admin")

	rr := httptest.NewRecorder()
	h.DoHideComment(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rr.Code)
	}
	if !mq.comments[1].hidden {
		t.Error("expected comment to be hidden")
	}
}

func TestDoUnhideComment_Success(t *testing.T) {
	mq := newMockModQuerier()
	mq.comments[1] = &mockModComment{id: 1, postID: 1, body: "comment", hidden: true}
	h := newModerationHandler(mq)

	form := url.Values{"comment_id": {"1"}}
	req := httptest.NewRequest("POST", "/admin/moderation/unhide-comment", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withAdminAgent(req, 10, "admin")

	rr := httptest.NewRecorder()
	h.DoUnhideComment(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rr.Code)
	}
	if mq.comments[1].hidden {
		t.Error("expected comment to be unhidden")
	}
}

func TestDoHideComment_InvalidID(t *testing.T) {
	mq := newMockModQuerier()
	h := newModerationHandler(mq)

	form := url.Values{"comment_id": {"xyz"}}
	req := httptest.NewRequest("POST", "/admin/moderation/hide-comment", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withAdminAgent(req, 10, "admin")

	rr := httptest.NewRecorder()
	h.DoHideComment(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Admin ban/unban agent
// ---------------------------------------------------------------------------

func TestDoBanAgent_Success(t *testing.T) {
	mq := newMockModQuerier()
	mq.agents[10] = &mockModAgent{id: 10, username: "admin", isAdmin: true}
	mq.agents[20] = &mockModAgent{id: 20, username: "baduser", isAdmin: false}
	mq.sessions[20] = []string{"tok1", "tok2"}
	h := newModerationHandler(mq)

	form := url.Values{"agent_id": {"20"}, "reason": {"spamming"}}
	req := httptest.NewRequest("POST", "/admin/moderation/ban-agent", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withAdminAgent(req, 10, "admin")

	rr := httptest.NewRecorder()
	h.DoBanAgent(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rr.Code)
	}
	if !strings.Contains(rr.Header().Get("Location"), "success=Agent+20+banned") {
		t.Errorf("expected success redirect, got %q", rr.Header().Get("Location"))
	}

	// Verify agent is banned.
	if !mq.agents[20].banned {
		t.Error("expected agent to be banned")
	}

	// Verify sessions are deleted.
	if len(mq.sessions[20]) != 0 {
		t.Errorf("expected sessions to be deleted, got %d", len(mq.sessions[20]))
	}
}

func TestDoBanAgent_CannotBanSelf(t *testing.T) {
	mq := newMockModQuerier()
	mq.agents[10] = &mockModAgent{id: 10, username: "admin", isAdmin: true}
	h := newModerationHandler(mq)

	form := url.Values{"agent_id": {"10"}}
	req := httptest.NewRequest("POST", "/admin/moderation/ban-agent", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withAdminAgent(req, 10, "admin")

	rr := httptest.NewRecorder()
	h.DoBanAgent(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "cannot ban yourself") {
		t.Errorf("expected 'cannot ban yourself' error, got %q", rr.Body.String())
	}
}

func TestDoBanAgent_CannotBanAdmin(t *testing.T) {
	mq := newMockModQuerier()
	mq.agents[10] = &mockModAgent{id: 10, username: "admin1", isAdmin: true}
	mq.agents[20] = &mockModAgent{id: 20, username: "admin2", isAdmin: true}
	h := newModerationHandler(mq)

	form := url.Values{"agent_id": {"20"}}
	req := httptest.NewRequest("POST", "/admin/moderation/ban-agent", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withAdminAgent(req, 10, "admin1")

	rr := httptest.NewRecorder()
	h.DoBanAgent(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "cannot ban an admin") {
		t.Errorf("expected 'cannot ban an admin' error, got %q", rr.Body.String())
	}
}

func TestDoBanAgent_InvalidAgentID(t *testing.T) {
	mq := newMockModQuerier()
	h := newModerationHandler(mq)

	form := url.Values{"agent_id": {"notanumber"}}
	req := httptest.NewRequest("POST", "/admin/moderation/ban-agent", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withAdminAgent(req, 10, "admin")

	rr := httptest.NewRecorder()
	h.DoBanAgent(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestDoUnbanAgent_Success(t *testing.T) {
	mq := newMockModQuerier()
	mq.agents[10] = &mockModAgent{id: 10, username: "admin", isAdmin: true}
	mq.agents[20] = &mockModAgent{id: 20, username: "banned_user", isAdmin: false, banned: true}
	h := newModerationHandler(mq)

	form := url.Values{"agent_id": {"20"}}
	req := httptest.NewRequest("POST", "/admin/moderation/unban-agent", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withAdminAgent(req, 10, "admin")

	rr := httptest.NewRecorder()
	h.DoUnbanAgent(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rr.Code)
	}
	if mq.agents[20].banned {
		t.Error("expected agent to be unbanned")
	}
}

// ---------------------------------------------------------------------------
// Route registration tests
// ---------------------------------------------------------------------------

func TestModerationRoutes_Registered(t *testing.T) {
	mq := newMockModQuerier()
	h := newModerationHandler(mq)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	routes := []struct {
		method string
		path   string
	}{
		{"POST", "/post/1/flag"},
		{"POST", "/comment/1/flag"},
		{"GET", "/admin/moderation"},
		{"POST", "/admin/moderation/hide-post"},
		{"POST", "/admin/moderation/unhide-post"},
		{"POST", "/admin/moderation/hide-comment"},
		{"POST", "/admin/moderation/unhide-comment"},
		{"POST", "/admin/moderation/ban-agent"},
		{"POST", "/admin/moderation/unban-agent"},
	}

	for _, rt := range routes {
		req := httptest.NewRequest(rt.method, rt.path, nil)
		_, pattern := mux.Handler(req)
		if pattern == "" {
			t.Errorf("%s %s should be registered, got no matching pattern", rt.method, rt.path)
		}
	}
}

// ---------------------------------------------------------------------------
// Admin routes require admin middleware (via RegisterRoutes)
// ---------------------------------------------------------------------------

func TestAdminRoutes_RequireAdmin(t *testing.T) {
	mq := newMockModQuerier()
	h := newModerationHandler(mq)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	adminRoutes := []struct {
		method string
		path   string
		form   url.Values
	}{
		{"GET", "/admin/moderation", nil},
		{"POST", "/admin/moderation/hide-post", url.Values{"post_id": {"1"}}},
		{"POST", "/admin/moderation/unhide-post", url.Values{"post_id": {"1"}}},
		{"POST", "/admin/moderation/hide-comment", url.Values{"comment_id": {"1"}}},
		{"POST", "/admin/moderation/unhide-comment", url.Values{"comment_id": {"1"}}},
		{"POST", "/admin/moderation/ban-agent", url.Values{"agent_id": {"1"}}},
		{"POST", "/admin/moderation/unban-agent", url.Values{"agent_id": {"1"}}},
	}

	for _, rt := range adminRoutes {
		t.Run(fmt.Sprintf("%s_%s_unauthenticated", rt.method, rt.path), func(t *testing.T) {
			var req *http.Request
			if rt.form != nil {
				req = httptest.NewRequest(rt.method, rt.path, strings.NewReader(rt.form.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			} else {
				req = httptest.NewRequest(rt.method, rt.path, nil)
			}

			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != http.StatusSeeOther {
				t.Errorf("unauthenticated %s %s: expected 303, got %d", rt.method, rt.path, rr.Code)
			}
		})

		t.Run(fmt.Sprintf("%s_%s_non_admin", rt.method, rt.path), func(t *testing.T) {
			var req *http.Request
			if rt.form != nil {
				req = httptest.NewRequest(rt.method, rt.path, strings.NewReader(rt.form.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			} else {
				req = httptest.NewRequest(rt.method, rt.path, nil)
			}
			req = withRegularAgent(req, 5, "regular_user")

			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != http.StatusForbidden {
				t.Errorf("non-admin %s %s: expected 403, got %d", rt.method, rt.path, rr.Code)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Full E2E flow via mux: flag -> admin hide -> verify
// ---------------------------------------------------------------------------

func TestE2E_FlagPostAndAdminHide(t *testing.T) {
	mq := newMockModQuerier()
	mq.posts[1] = &mockModPost{id: 1, title: "Spam Post"}
	mq.agents[10] = &mockModAgent{id: 10, username: "admin", isAdmin: true}
	h := newModerationHandler(mq)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Step 1: Regular user flags the post.
	flagForm := url.Values{"reason": {"spam"}}
	flagReq := httptest.NewRequest("POST", "/post/1/flag", strings.NewReader(flagForm.Encode()))
	flagReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	flagReq = withRegularAgent(flagReq, 5, "reporter")
	flagRR := httptest.NewRecorder()
	mux.ServeHTTP(flagRR, flagReq)

	if flagRR.Code != http.StatusSeeOther {
		t.Fatalf("flag: expected 303, got %d", flagRR.Code)
	}
	if len(mq.flags) != 1 {
		t.Fatalf("expected 1 flag after flagging, got %d", len(mq.flags))
	}

	// Step 2: Admin hides the post.
	hideForm := url.Values{"post_id": {"1"}, "reason": {"confirmed spam"}}
	hideReq := httptest.NewRequest("POST", "/admin/moderation/hide-post", strings.NewReader(hideForm.Encode()))
	hideReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	hideReq = withAdminAgent(hideReq, 10, "admin")
	hideRR := httptest.NewRecorder()
	mux.ServeHTTP(hideRR, hideReq)

	if hideRR.Code != http.StatusSeeOther {
		t.Fatalf("hide: expected 303, got %d", hideRR.Code)
	}

	// Step 3: Verify post is hidden.
	if !mq.posts[1].hidden {
		t.Error("expected post to be hidden after admin action")
	}

	// Step 4: Verify moderation log was created.
	if len(mq.modLogs) != 1 {
		t.Fatalf("expected 1 moderation log, got %d", len(mq.modLogs))
	}
	if mq.modLogs[0].Action != db.ModerationActionHidePost {
		t.Errorf("log action = %q, want %q", mq.modLogs[0].Action, db.ModerationActionHidePost)
	}
}

func TestE2E_BanAgentInvalidatesSessions(t *testing.T) {
	mq := newMockModQuerier()
	mq.agents[10] = &mockModAgent{id: 10, username: "admin", isAdmin: true}
	mq.agents[20] = &mockModAgent{id: 20, username: "spammer", isAdmin: false}
	mq.sessions[20] = []string{"session-a", "session-b"}
	h := newModerationHandler(mq)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Admin bans the agent.
	banForm := url.Values{"agent_id": {"20"}, "reason": {"spamming"}}
	banReq := httptest.NewRequest("POST", "/admin/moderation/ban-agent", strings.NewReader(banForm.Encode()))
	banReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	banReq = withAdminAgent(banReq, 10, "admin")
	banRR := httptest.NewRecorder()
	mux.ServeHTTP(banRR, banReq)

	if banRR.Code != http.StatusSeeOther {
		t.Fatalf("ban: expected 303, got %d", banRR.Code)
	}

	// Agent is banned.
	if !mq.agents[20].banned {
		t.Error("expected agent to be banned")
	}

	// Sessions are cleared.
	if len(mq.sessions[20]) != 0 {
		t.Errorf("expected 0 sessions after ban, got %d", len(mq.sessions[20]))
	}

	// Admin unbans the agent.
	unbanForm := url.Values{"agent_id": {"20"}}
	unbanReq := httptest.NewRequest("POST", "/admin/moderation/unban-agent", strings.NewReader(unbanForm.Encode()))
	unbanReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	unbanReq = withAdminAgent(unbanReq, 10, "admin")
	unbanRR := httptest.NewRecorder()
	mux.ServeHTTP(unbanRR, unbanReq)

	if unbanRR.Code != http.StatusSeeOther {
		t.Fatalf("unban: expected 303, got %d", unbanRR.Code)
	}

	if mq.agents[20].banned {
		t.Error("expected agent to be unbanned")
	}
}
