package service

import (
	"context"
	"errors"
	"testing"

	db "github.com/trishtzy/warren/db/generated"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// Mock
// ---------------------------------------------------------------------------

type mockModerationQuerier struct {
	flags          []db.Flag
	nextFlagID     int64
	posts          map[int64]*mockPost
	comments       map[int64]*mockComment
	agents         map[int64]*mockAgent
	sessions       map[int64][]string // agentID -> session tokens
	moderationLogs []db.ModerationLog
	nextLogID      int64
}

type mockPost struct {
	id     int64
	hidden bool
	title  string
}

type mockComment struct {
	id     int64
	postID int64
	hidden bool
	body   string
}

type mockAgent struct {
	id       int64
	username string
	isAdmin  bool
	banned   bool
}

func newMockModerationQuerier() *mockModerationQuerier {
	return &mockModerationQuerier{
		nextFlagID: 1,
		nextLogID:  1,
		posts:      make(map[int64]*mockPost),
		comments:   make(map[int64]*mockComment),
		agents:     make(map[int64]*mockAgent),
		sessions:   make(map[int64][]string),
	}
}

func (m *mockModerationQuerier) CreateFlag(_ context.Context, arg db.CreateFlagParams) (db.Flag, error) {
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

func (m *mockModerationQuerier) HasAgentFlagged(_ context.Context, arg db.HasAgentFlaggedParams) (bool, error) {
	for _, f := range m.flags {
		if f.AgentID == arg.AgentID && f.TargetType == arg.TargetType && f.TargetID == arg.TargetID {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockModerationQuerier) ListFlaggedPosts(_ context.Context, arg db.ListFlaggedPostsParams) ([]db.ListFlaggedPostsRow, error) {
	var rows []db.ListFlaggedPostsRow
	// Count flags per post.
	flagCounts := make(map[int64]int64)
	for _, f := range m.flags {
		if f.TargetType == db.FlagTargetTypePost {
			flagCounts[f.TargetID]++
		}
	}
	for postID, count := range flagCounts {
		p, ok := m.posts[postID]
		if !ok {
			continue
		}
		agentUsername := "unknown"
		// Find the agent who posted it (simplified: just use first agent).
		rows = append(rows, db.ListFlaggedPostsRow{
			ID:            p.id,
			Title:         p.title,
			AgentUsername: agentUsername,
			FlagCount:     count,
			Hidden:        p.hidden,
		})
	}
	return rows, nil
}

func (m *mockModerationQuerier) ListFlaggedComments(_ context.Context, arg db.ListFlaggedCommentsParams) ([]db.ListFlaggedCommentsRow, error) {
	var rows []db.ListFlaggedCommentsRow
	flagCounts := make(map[int64]int64)
	for _, f := range m.flags {
		if f.TargetType == db.FlagTargetTypeComment {
			flagCounts[f.TargetID]++
		}
	}
	for commentID, count := range flagCounts {
		c, ok := m.comments[commentID]
		if !ok {
			continue
		}
		rows = append(rows, db.ListFlaggedCommentsRow{
			ID:        c.id,
			PostID:    c.postID,
			Body:      c.body,
			FlagCount: count,
			Hidden:    c.hidden,
		})
	}
	return rows, nil
}

func (m *mockModerationQuerier) UpdatePostHidden(_ context.Context, arg db.UpdatePostHiddenParams) error {
	p, ok := m.posts[arg.ID]
	if !ok {
		return errors.New("post not found")
	}
	p.hidden = arg.Hidden
	return nil
}

func (m *mockModerationQuerier) UpdateCommentHidden(_ context.Context, arg db.UpdateCommentHiddenParams) error {
	c, ok := m.comments[arg.ID]
	if !ok {
		return errors.New("comment not found")
	}
	c.hidden = arg.Hidden
	return nil
}

func (m *mockModerationQuerier) UpdateAgentBanned(_ context.Context, arg db.UpdateAgentBannedParams) error {
	a, ok := m.agents[arg.ID]
	if !ok {
		return errors.New("agent not found")
	}
	a.banned = arg.Banned
	return nil
}

func (m *mockModerationQuerier) DeleteSessionsByAgent(_ context.Context, agentID int64) error {
	delete(m.sessions, agentID)
	return nil
}

func (m *mockModerationQuerier) GetAgentByIDForAdmin(_ context.Context, id int64) (db.GetAgentByIDForAdminRow, error) {
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

func (m *mockModerationQuerier) CreateModerationLog(_ context.Context, arg db.CreateModerationLogParams) (db.ModerationLog, error) {
	log := db.ModerationLog{
		ID:       m.nextLogID,
		AdminID:  arg.AdminID,
		Action:   arg.Action,
		TargetID: arg.TargetID,
		Reason:   arg.Reason,
	}
	m.moderationLogs = append(m.moderationLogs, log)
	m.nextLogID++
	return log, nil
}

func (m *mockModerationQuerier) ListModerationLog(_ context.Context, arg db.ListModerationLogParams) ([]db.ListModerationLogRow, error) {
	var rows []db.ListModerationLogRow
	for _, l := range m.moderationLogs {
		adminUsername := "admin"
		if a, ok := m.agents[l.AdminID]; ok {
			adminUsername = a.username
		}
		rows = append(rows, db.ListModerationLogRow{
			ID:            l.ID,
			AdminID:       l.AdminID,
			Action:        l.Action,
			TargetID:      l.TargetID,
			Reason:        l.Reason,
			AdminUsername: adminUsername,
		})
	}
	return rows, nil
}

// helper to add test data.
func (m *mockModerationQuerier) addPost(id int64, title string) {
	m.posts[id] = &mockPost{id: id, title: title}
}

func (m *mockModerationQuerier) addComment(id, postID int64, body string) {
	m.comments[id] = &mockComment{id: id, postID: postID, body: body}
}

func (m *mockModerationQuerier) addAgent(id int64, username string, isAdmin bool) {
	m.agents[id] = &mockAgent{id: id, username: username, isAdmin: isAdmin}
}

func (m *mockModerationQuerier) addSession(agentID int64, token string) {
	m.sessions[agentID] = append(m.sessions[agentID], token)
}

// ExecTx runs fn directly — no real transaction in the mock, but it exercises the code path.
func (m *mockModerationQuerier) ExecTx(_ context.Context, fn func(ModerationQuerier) error) error {
	return fn(m)
}

// Ensure the mock satisfies the ModerationStore interface.
var _ ModerationStore = (*mockModerationQuerier)(nil)

// Suppress unused import.
var _ = pgtype.Timestamptz{}

// ---------------------------------------------------------------------------
// FlagContent tests
// ---------------------------------------------------------------------------

func TestFlagContent_PostSuccess(t *testing.T) {
	mq := newMockModerationQuerier()
	mq.addPost(1, "Test Post")
	svc := NewModerationService(mq)

	reason := "spam"
	flag, err := svc.FlagContent(context.Background(), 1, db.FlagTargetTypePost, 1, &reason)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if flag.ID == 0 {
		t.Fatal("expected non-zero flag ID")
	}
	if flag.TargetType != db.FlagTargetTypePost {
		t.Errorf("target_type = %q, want %q", flag.TargetType, db.FlagTargetTypePost)
	}
	if flag.TargetID != 1 {
		t.Errorf("target_id = %d, want 1", flag.TargetID)
	}
	if flag.Reason == nil || *flag.Reason != "spam" {
		t.Errorf("reason = %v, want %q", flag.Reason, "spam")
	}
}

func TestFlagContent_CommentSuccess(t *testing.T) {
	mq := newMockModerationQuerier()
	mq.addComment(1, 1, "some comment")
	svc := NewModerationService(mq)

	reason := "offensive"
	flag, err := svc.FlagContent(context.Background(), 2, db.FlagTargetTypeComment, 1, &reason)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if flag.TargetType != db.FlagTargetTypeComment {
		t.Errorf("target_type = %q, want %q", flag.TargetType, db.FlagTargetTypeComment)
	}
}

func TestFlagContent_NilReason(t *testing.T) {
	mq := newMockModerationQuerier()
	mq.addPost(1, "Test Post")
	svc := NewModerationService(mq)

	flag, err := svc.FlagContent(context.Background(), 1, db.FlagTargetTypePost, 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if flag.Reason != nil {
		t.Errorf("reason = %v, want nil", flag.Reason)
	}
}

func TestFlagContent_AlreadyFlagged(t *testing.T) {
	mq := newMockModerationQuerier()
	mq.addPost(1, "Test Post")
	svc := NewModerationService(mq)

	reason := "spam"
	_, err := svc.FlagContent(context.Background(), 1, db.FlagTargetTypePost, 1, &reason)
	if err != nil {
		t.Fatalf("first flag: %v", err)
	}

	// Flag again — should return ErrAlreadyFlagged.
	_, err = svc.FlagContent(context.Background(), 1, db.FlagTargetTypePost, 1, &reason)
	if !errors.Is(err, ErrAlreadyFlagged) {
		t.Errorf("expected ErrAlreadyFlagged, got: %v", err)
	}
}

func TestFlagContent_DifferentAgentCanFlag(t *testing.T) {
	mq := newMockModerationQuerier()
	mq.addPost(1, "Test Post")
	svc := NewModerationService(mq)

	reason := "spam"
	_, err := svc.FlagContent(context.Background(), 1, db.FlagTargetTypePost, 1, &reason)
	if err != nil {
		t.Fatalf("first flag: %v", err)
	}

	// Different agent flags same post — should succeed.
	_, err = svc.FlagContent(context.Background(), 2, db.FlagTargetTypePost, 1, &reason)
	if err != nil {
		t.Fatalf("second agent flag: %v", err)
	}

	if len(mq.flags) != 2 {
		t.Errorf("expected 2 flags, got %d", len(mq.flags))
	}
}

// ---------------------------------------------------------------------------
// HidePost / UnhidePost tests
// ---------------------------------------------------------------------------

func TestHidePost_Success(t *testing.T) {
	mq := newMockModerationQuerier()
	mq.addPost(1, "Test Post")
	mq.addAgent(10, "admin", true)
	svc := NewModerationService(mq)

	reason := "violates policy"
	err := svc.HidePost(context.Background(), 10, 1, &reason)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify post is hidden.
	if !mq.posts[1].hidden {
		t.Error("expected post to be hidden")
	}

	// Verify moderation log was created.
	if len(mq.moderationLogs) != 1 {
		t.Fatalf("expected 1 moderation log, got %d", len(mq.moderationLogs))
	}
	if mq.moderationLogs[0].Action != db.ModerationActionHidePost {
		t.Errorf("action = %q, want %q", mq.moderationLogs[0].Action, db.ModerationActionHidePost)
	}
	if mq.moderationLogs[0].TargetID != 1 {
		t.Errorf("target_id = %d, want 1", mq.moderationLogs[0].TargetID)
	}
	if mq.moderationLogs[0].AdminID != 10 {
		t.Errorf("admin_id = %d, want 10", mq.moderationLogs[0].AdminID)
	}
}

func TestUnhidePost_Success(t *testing.T) {
	mq := newMockModerationQuerier()
	mq.addPost(1, "Test Post")
	mq.posts[1].hidden = true
	mq.addAgent(10, "admin", true)
	svc := NewModerationService(mq)

	err := svc.UnhidePost(context.Background(), 10, 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mq.posts[1].hidden {
		t.Error("expected post to be unhidden")
	}

	if len(mq.moderationLogs) != 1 {
		t.Fatalf("expected 1 moderation log, got %d", len(mq.moderationLogs))
	}
	if mq.moderationLogs[0].Action != db.ModerationActionUnhidePost {
		t.Errorf("action = %q, want %q", mq.moderationLogs[0].Action, db.ModerationActionUnhidePost)
	}
}

// ---------------------------------------------------------------------------
// HideComment / UnhideComment tests
// ---------------------------------------------------------------------------

func TestHideComment_Success(t *testing.T) {
	mq := newMockModerationQuerier()
	mq.addComment(1, 1, "bad comment")
	mq.addAgent(10, "admin", true)
	svc := NewModerationService(mq)

	err := svc.HideComment(context.Background(), 10, 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mq.comments[1].hidden {
		t.Error("expected comment to be hidden")
	}

	if len(mq.moderationLogs) != 1 {
		t.Fatalf("expected 1 moderation log, got %d", len(mq.moderationLogs))
	}
	if mq.moderationLogs[0].Action != db.ModerationActionHideComment {
		t.Errorf("action = %q, want %q", mq.moderationLogs[0].Action, db.ModerationActionHideComment)
	}
}

func TestUnhideComment_Success(t *testing.T) {
	mq := newMockModerationQuerier()
	mq.addComment(1, 1, "restored comment")
	mq.comments[1].hidden = true
	mq.addAgent(10, "admin", true)
	svc := NewModerationService(mq)

	err := svc.UnhideComment(context.Background(), 10, 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mq.comments[1].hidden {
		t.Error("expected comment to be unhidden")
	}

	if mq.moderationLogs[0].Action != db.ModerationActionUnhideComment {
		t.Errorf("action = %q, want %q", mq.moderationLogs[0].Action, db.ModerationActionUnhideComment)
	}
}

// ---------------------------------------------------------------------------
// BanAgent tests
// ---------------------------------------------------------------------------

func TestBanAgent_Success(t *testing.T) {
	mq := newMockModerationQuerier()
	mq.addAgent(10, "admin", true)
	mq.addAgent(20, "baduser", false)
	mq.addSession(20, "session-token-1")
	mq.addSession(20, "session-token-2")
	svc := NewModerationService(mq)

	reason := "spamming"
	err := svc.BanAgent(context.Background(), 10, 20, &reason)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify agent is banned.
	if !mq.agents[20].banned {
		t.Error("expected agent to be banned")
	}

	// Verify sessions are deleted.
	if sessions, ok := mq.sessions[20]; ok && len(sessions) > 0 {
		t.Errorf("expected sessions to be deleted, got %d", len(sessions))
	}

	// Verify moderation log.
	if len(mq.moderationLogs) != 1 {
		t.Fatalf("expected 1 moderation log, got %d", len(mq.moderationLogs))
	}
	if mq.moderationLogs[0].Action != db.ModerationActionBanAgent {
		t.Errorf("action = %q, want %q", mq.moderationLogs[0].Action, db.ModerationActionBanAgent)
	}
	if mq.moderationLogs[0].Reason == nil || *mq.moderationLogs[0].Reason != "spamming" {
		t.Errorf("reason = %v, want %q", mq.moderationLogs[0].Reason, "spamming")
	}
}

func TestBanAgent_CannotBanSelf(t *testing.T) {
	mq := newMockModerationQuerier()
	mq.addAgent(10, "admin", true)
	svc := NewModerationService(mq)

	err := svc.BanAgent(context.Background(), 10, 10, nil)
	if !errors.Is(err, ErrCannotBanSelf) {
		t.Errorf("expected ErrCannotBanSelf, got: %v", err)
	}
}

func TestBanAgent_CannotBanAdmin(t *testing.T) {
	mq := newMockModerationQuerier()
	mq.addAgent(10, "admin1", true)
	mq.addAgent(20, "admin2", true)
	svc := NewModerationService(mq)

	err := svc.BanAgent(context.Background(), 10, 20, nil)
	if !errors.Is(err, ErrCannotBanAdmin) {
		t.Errorf("expected ErrCannotBanAdmin, got: %v", err)
	}
}

func TestBanAgent_SessionsInvalidated(t *testing.T) {
	mq := newMockModerationQuerier()
	mq.addAgent(10, "admin", true)
	mq.addAgent(20, "target", false)
	mq.addSession(20, "tok1")
	mq.addSession(20, "tok2")
	mq.addSession(20, "tok3")
	svc := NewModerationService(mq)

	err := svc.BanAgent(context.Background(), 10, 20, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All sessions for the banned agent should be gone.
	if len(mq.sessions[20]) != 0 {
		t.Errorf("expected 0 sessions after ban, got %d", len(mq.sessions[20]))
	}
}

// ---------------------------------------------------------------------------
// UnbanAgent tests
// ---------------------------------------------------------------------------

func TestUnbanAgent_Success(t *testing.T) {
	mq := newMockModerationQuerier()
	mq.addAgent(10, "admin", true)
	mq.addAgent(20, "banneduser", false)
	mq.agents[20].banned = true
	svc := NewModerationService(mq)

	err := svc.UnbanAgent(context.Background(), 10, 20, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mq.agents[20].banned {
		t.Error("expected agent to be unbanned")
	}

	if len(mq.moderationLogs) != 1 {
		t.Fatalf("expected 1 moderation log, got %d", len(mq.moderationLogs))
	}
	if mq.moderationLogs[0].Action != db.ModerationActionUnbanAgent {
		t.Errorf("action = %q, want %q", mq.moderationLogs[0].Action, db.ModerationActionUnbanAgent)
	}
}

// ---------------------------------------------------------------------------
// ListFlaggedPosts / ListFlaggedComments / ListModerationLog tests
// ---------------------------------------------------------------------------

func TestListFlaggedPosts_SortedByFlagCount(t *testing.T) {
	mq := newMockModerationQuerier()
	mq.addPost(1, "Post A")
	mq.addPost(2, "Post B")
	svc := NewModerationService(mq)

	// Flag post 1 once, post 2 twice.
	reason := "spam"
	_, _ = svc.FlagContent(context.Background(), 1, db.FlagTargetTypePost, 1, &reason)
	_, _ = svc.FlagContent(context.Background(), 2, db.FlagTargetTypePost, 2, &reason)
	_, _ = svc.FlagContent(context.Background(), 3, db.FlagTargetTypePost, 2, &reason)

	flagged, err := svc.ListFlaggedPosts(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(flagged) != 2 {
		t.Fatalf("expected 2 flagged posts, got %d", len(flagged))
	}
}

func TestListFlaggedComments(t *testing.T) {
	mq := newMockModerationQuerier()
	mq.addComment(1, 1, "bad comment")
	svc := NewModerationService(mq)

	reason := "off-topic"
	_, _ = svc.FlagContent(context.Background(), 1, db.FlagTargetTypeComment, 1, &reason)

	flagged, err := svc.ListFlaggedComments(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(flagged) != 1 {
		t.Fatalf("expected 1 flagged comment, got %d", len(flagged))
	}
	if flagged[0].FlagCount != 1 {
		t.Errorf("flag_count = %d, want 1", flagged[0].FlagCount)
	}
}

func TestListModerationLog(t *testing.T) {
	mq := newMockModerationQuerier()
	mq.addPost(1, "Test Post")
	mq.addAgent(10, "admin", true)
	mq.addAgent(20, "target", false)
	svc := NewModerationService(mq)

	_ = svc.HidePost(context.Background(), 10, 1, nil)
	_ = svc.BanAgent(context.Background(), 10, 20, nil)

	log, err := svc.ListModerationLog(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(log) != 2 {
		t.Fatalf("expected 2 log entries, got %d", len(log))
	}
}

// ---------------------------------------------------------------------------
// Full flow: flag -> hide -> verify log
// ---------------------------------------------------------------------------

func TestFullModerationFlow_FlagThenHidePost(t *testing.T) {
	mq := newMockModerationQuerier()
	mq.addPost(1, "Spam Post")
	mq.addAgent(10, "admin", true)
	svc := NewModerationService(mq)

	// Step 1: Multiple agents flag the post.
	reason := "spam"
	_, err := svc.FlagContent(context.Background(), 1, db.FlagTargetTypePost, 1, &reason)
	if err != nil {
		t.Fatalf("flag 1: %v", err)
	}
	_, err = svc.FlagContent(context.Background(), 2, db.FlagTargetTypePost, 1, &reason)
	if err != nil {
		t.Fatalf("flag 2: %v", err)
	}

	// Step 2: Verify it appears in flagged list.
	flagged, err := svc.ListFlaggedPosts(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("list flagged: %v", err)
	}
	if len(flagged) != 1 {
		t.Fatalf("expected 1 flagged post, got %d", len(flagged))
	}
	if flagged[0].FlagCount != 2 {
		t.Errorf("flag_count = %d, want 2", flagged[0].FlagCount)
	}

	// Step 3: Admin hides the post.
	hideReason := "confirmed spam"
	err = svc.HidePost(context.Background(), 10, 1, &hideReason)
	if err != nil {
		t.Fatalf("hide post: %v", err)
	}

	// Step 4: Verify post is hidden.
	if !mq.posts[1].hidden {
		t.Error("expected post to be hidden")
	}

	// Step 5: Verify moderation log.
	logs, err := svc.ListModerationLog(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("list log: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(logs))
	}
	if logs[0].Action != db.ModerationActionHidePost {
		t.Errorf("action = %q, want %q", logs[0].Action, db.ModerationActionHidePost)
	}
}

func TestFullModerationFlow_BanAndUnban(t *testing.T) {
	mq := newMockModerationQuerier()
	mq.addAgent(10, "admin", true)
	mq.addAgent(20, "spammer", false)
	mq.addSession(20, "sess1")
	svc := NewModerationService(mq)

	// Step 1: Ban the agent.
	banReason := "repeatedly posting spam"
	err := svc.BanAgent(context.Background(), 10, 20, &banReason)
	if err != nil {
		t.Fatalf("ban: %v", err)
	}
	if !mq.agents[20].banned {
		t.Error("expected agent to be banned")
	}
	if len(mq.sessions[20]) != 0 {
		t.Error("expected sessions to be cleared after ban")
	}

	// Step 2: Unban the agent.
	err = svc.UnbanAgent(context.Background(), 10, 20, nil)
	if err != nil {
		t.Fatalf("unban: %v", err)
	}
	if mq.agents[20].banned {
		t.Error("expected agent to be unbanned")
	}

	// Step 3: Verify moderation log has both entries.
	logs, err := svc.ListModerationLog(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("list log: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("expected 2 log entries, got %d", len(logs))
	}
}
