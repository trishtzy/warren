package db_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	db "github.com/trishtzy/warren/db/generated"
	"github.com/trishtzy/warren/migrations"
)

// testDBURL returns the database URL for integration tests.
// It uses WARREN_TEST_DATABASE_URL if set, otherwise falls back to a
// local default.
func testDBURL() string {
	if u := os.Getenv("WARREN_TEST_DATABASE_URL"); u != "" {
		return u
	}
	return "postgresql://rabbithole:rabbithole@127.0.0.1:5433/rabbithole_test?sslmode=disable"
}

// setup creates a fresh pgx connection and a db.Queries instance.
// It runs goose migrations to ensure the schema is up to date, then
// truncates all tables so each test starts clean.
func setup(t *testing.T) (context.Context, *pgx.Conn, *db.Queries) {
	t.Helper()
	ctx := context.Background()

	dbURL := testDBURL()

	// Run migrations via goose using the embedded SQL files.
	sqlDB, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Skipf("skipping: cannot open test database: %v", err)
	}
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		sqlDB.Close()
		t.Fatalf("goose set dialect: %v", err)
	}
	if err := goose.Up(sqlDB, "."); err != nil {
		sqlDB.Close()
		t.Skipf("skipping: goose migrations failed: %v", err)
	}
	sqlDB.Close()

	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Skipf("skipping: cannot connect to test database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })

	// Truncate in reverse-FK order.
	_, err = conn.Exec(ctx, `
		TRUNCATE sessions, flags, votes, comments, posts, agents RESTART IDENTITY CASCADE
	`)
	if err != nil {
		t.Fatalf("truncate tables: %v", err)
	}

	return ctx, conn, db.New(conn)
}

// createTestAgent is a helper that inserts an agent and returns it.
func createTestAgent(t *testing.T, ctx context.Context, q *db.Queries, username string) db.Agent {
	t.Helper()
	agent, err := q.CreateAgent(ctx, db.CreateAgentParams{
		Username:     username,
		Email:        username + "@example.com",
		PasswordHash: "hash_" + username,
	})
	if err != nil {
		t.Fatalf("create test agent %q: %v", username, err)
	}
	return agent
}

// createTestPost is a helper that inserts a post and returns it.
func createTestPost(t *testing.T, ctx context.Context, q *db.Queries, agentID int64, title string) db.Post {
	t.Helper()
	url := "https://example.com/" + title
	post, err := q.CreatePost(ctx, db.CreatePostParams{
		AgentID: agentID,
		Title:   title,
		Url:     &url,
		Domain:  strPtr("example.com"),
	})
	if err != nil {
		t.Fatalf("create test post %q: %v", title, err)
	}
	return post
}

func strPtr(s string) *string {
	return &s
}

// ---------------------------------------------------------------------------
// Agent tests
// ---------------------------------------------------------------------------

func TestCreateAndGetAgent(t *testing.T) {
	ctx, _, q := setup(t)

	agent, err := q.CreateAgent(ctx, db.CreateAgentParams{
		Username:     "alice",
		Email:        "alice@example.com",
		PasswordHash: "argon2id$hash",
	})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if agent.ID == 0 {
		t.Fatal("expected non-zero agent ID")
	}
	if agent.Username != "alice" {
		t.Errorf("username = %q, want %q", agent.Username, "alice")
	}
	if agent.IsAdmin {
		t.Error("expected is_admin = false for new agent")
	}

	// Read back by ID.
	got, err := q.GetAgentByID(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetAgentByID: %v", err)
	}
	if got.Email != "alice@example.com" {
		t.Errorf("email = %q, want %q", got.Email, "alice@example.com")
	}

	// Read back by username.
	got, err = q.GetAgentByUsername(ctx, "alice")
	if err != nil {
		t.Fatalf("GetAgentByUsername: %v", err)
	}
	if got.ID != agent.ID {
		t.Errorf("GetAgentByUsername ID = %d, want %d", got.ID, agent.ID)
	}

	// Read back by email.
	got, err = q.GetAgentByEmail(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("GetAgentByEmail: %v", err)
	}
	if got.ID != agent.ID {
		t.Errorf("GetAgentByEmail ID = %d, want %d", got.ID, agent.ID)
	}
}

// ---------------------------------------------------------------------------
// Post tests
// ---------------------------------------------------------------------------

func TestCreateAndGetPost(t *testing.T) {
	ctx, _, q := setup(t)
	agent := createTestAgent(t, ctx, q, "poster")

	url := "https://example.com/article"
	domain := "example.com"
	post, err := q.CreatePost(ctx, db.CreatePostParams{
		AgentID: agent.ID,
		Title:   "Test Article",
		Url:     &url,
		Domain:  &domain,
	})
	if err != nil {
		t.Fatalf("CreatePost: %v", err)
	}
	if post.ID == 0 {
		t.Fatal("expected non-zero post ID")
	}
	if post.Title != "Test Article" {
		t.Errorf("title = %q, want %q", post.Title, "Test Article")
	}
	if post.Score != 1 {
		t.Errorf("score = %d, want 1 (default)", post.Score)
	}

	// Read back by ID (includes agent username via JOIN).
	got, err := q.GetPostByID(ctx, post.ID)
	if err != nil {
		t.Fatalf("GetPostByID: %v", err)
	}
	if got.AgentUsername != "poster" {
		t.Errorf("agent_username = %q, want %q", got.AgentUsername, "poster")
	}

	// List by new — should return the post.
	posts, err := q.ListPostsByNew(ctx, db.ListPostsByNewParams{
		RowLimit:  10,
		RowOffset: 0,
	})
	if err != nil {
		t.Fatalf("ListPostsByNew: %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("ListPostsByNew returned %d posts, want 1", len(posts))
	}
	if posts[0].ID != post.ID {
		t.Errorf("ListPostsByNew[0].ID = %d, want %d", posts[0].ID, post.ID)
	}
}

// ---------------------------------------------------------------------------
// Comment tests
// ---------------------------------------------------------------------------

func TestCreateAndGetComment(t *testing.T) {
	ctx, _, q := setup(t)
	agent := createTestAgent(t, ctx, q, "commenter")
	post := createTestPost(t, ctx, q, agent.ID, "comment-target")

	comment, err := q.CreateComment(ctx, db.CreateCommentParams{
		AgentID: agent.ID,
		PostID:  post.ID,
		Body:    "Great post!",
	})
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	if comment.ID == 0 {
		t.Fatal("expected non-zero comment ID")
	}
	if comment.Body != "Great post!" {
		t.Errorf("body = %q, want %q", comment.Body, "Great post!")
	}

	// Read back by ID.
	got, err := q.GetCommentByID(ctx, comment.ID)
	if err != nil {
		t.Fatalf("GetCommentByID: %v", err)
	}
	if got.AgentUsername != "commenter" {
		t.Errorf("agent_username = %q, want %q", got.AgentUsername, "commenter")
	}

	// List by post (now requires limit/offset).
	comments, err := q.ListCommentsByPost(ctx, db.ListCommentsByPostParams{
		PostID:    post.ID,
		RowLimit:  10,
		RowOffset: 0,
	})
	if err != nil {
		t.Fatalf("ListCommentsByPost: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("ListCommentsByPost returned %d, want 1", len(comments))
	}
	if comments[0].Body != "Great post!" {
		t.Errorf("comment body = %q, want %q", comments[0].Body, "Great post!")
	}
}

func TestListCommentsByPostPagination(t *testing.T) {
	ctx, _, q := setup(t)
	agent := createTestAgent(t, ctx, q, "paginator")
	post := createTestPost(t, ctx, q, agent.ID, "paginated-post")

	// Create 5 comments.
	for i := 0; i < 5; i++ {
		_, err := q.CreateComment(ctx, db.CreateCommentParams{
			AgentID: agent.ID,
			PostID:  post.ID,
			Body:    fmt.Sprintf("Comment %d", i),
		})
		if err != nil {
			t.Fatalf("CreateComment %d: %v", i, err)
		}
	}

	// Fetch first page (limit 2).
	page1, err := q.ListCommentsByPost(ctx, db.ListCommentsByPostParams{
		PostID:    post.ID,
		RowLimit:  2,
		RowOffset: 0,
	})
	if err != nil {
		t.Fatalf("ListCommentsByPost page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1 len = %d, want 2", len(page1))
	}

	// Fetch second page (limit 2, offset 2).
	page2, err := q.ListCommentsByPost(ctx, db.ListCommentsByPostParams{
		PostID:    post.ID,
		RowLimit:  2,
		RowOffset: 2,
	})
	if err != nil {
		t.Fatalf("ListCommentsByPost page2: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("page2 len = %d, want 2", len(page2))
	}

	// Pages should not overlap.
	if page1[0].ID == page2[0].ID {
		t.Error("page1 and page2 returned the same first comment")
	}
}

// ---------------------------------------------------------------------------
// Vote tests
// ---------------------------------------------------------------------------

func TestCreateVoteAndDuplicate(t *testing.T) {
	ctx, _, q := setup(t)
	agent := createTestAgent(t, ctx, q, "voter")
	post := createTestPost(t, ctx, q, agent.ID, "voteable")

	vote, err := q.CreateVote(ctx, db.CreateVoteParams{
		AgentID: agent.ID,
		PostID:  post.ID,
	})
	if err != nil {
		t.Fatalf("CreateVote: %v", err)
	}
	if vote.ID == 0 {
		t.Fatal("expected non-zero vote ID")
	}

	// Duplicate vote should NOT error — it returns the existing row
	// thanks to ON CONFLICT DO UPDATE.
	vote2, err := q.CreateVote(ctx, db.CreateVoteParams{
		AgentID: agent.ID,
		PostID:  post.ID,
	})
	if err != nil {
		t.Fatalf("CreateVote (duplicate): %v", err)
	}
	if vote2.ID != vote.ID {
		t.Errorf("duplicate vote ID = %d, want original %d", vote2.ID, vote.ID)
	}

	// Read back via GetVote.
	got, err := q.GetVote(ctx, db.GetVoteParams{
		AgentID: agent.ID,
		PostID:  post.ID,
	})
	if err != nil {
		t.Fatalf("GetVote: %v", err)
	}
	if got.ID != vote.ID {
		t.Errorf("GetVote ID = %d, want %d", got.ID, vote.ID)
	}

	// Count should be 1 (not 2).
	count, err := q.CountVotesByPost(ctx, post.ID)
	if err != nil {
		t.Fatalf("CountVotesByPost: %v", err)
	}
	if count != 1 {
		t.Errorf("vote count = %d, want 1", count)
	}
}

// ---------------------------------------------------------------------------
// Flag tests
// ---------------------------------------------------------------------------

func TestCreateFlagAndDuplicate(t *testing.T) {
	ctx, _, q := setup(t)
	agent := createTestAgent(t, ctx, q, "flagger")
	post := createTestPost(t, ctx, q, agent.ID, "flaggable")

	reason := "spam"
	flag, err := q.CreateFlag(ctx, db.CreateFlagParams{
		AgentID:    agent.ID,
		TargetType: db.FlagTargetTypePost,
		TargetID:   post.ID,
		Reason:     &reason,
	})
	if err != nil {
		t.Fatalf("CreateFlag: %v", err)
	}
	if flag.ID == 0 {
		t.Fatal("expected non-zero flag ID")
	}

	// Duplicate flag should NOT error — returns the row with updated reason.
	newReason := "offensive"
	flag2, err := q.CreateFlag(ctx, db.CreateFlagParams{
		AgentID:    agent.ID,
		TargetType: db.FlagTargetTypePost,
		TargetID:   post.ID,
		Reason:     &newReason,
	})
	if err != nil {
		t.Fatalf("CreateFlag (duplicate): %v", err)
	}
	if flag2.ID != flag.ID {
		t.Errorf("duplicate flag ID = %d, want original %d", flag2.ID, flag.ID)
	}
	// Reason should be updated.
	if flag2.Reason == nil || *flag2.Reason != "offensive" {
		t.Errorf("flag reason = %v, want %q", flag2.Reason, "offensive")
	}

	// List flags (now requires limit/offset).
	flags, err := q.ListFlagsByTarget(ctx, db.ListFlagsByTargetParams{
		TargetType: db.FlagTargetTypePost,
		TargetID:   post.ID,
		RowLimit:   10,
		RowOffset:  0,
	})
	if err != nil {
		t.Fatalf("ListFlagsByTarget: %v", err)
	}
	if len(flags) != 1 {
		t.Fatalf("ListFlagsByTarget returned %d, want 1", len(flags))
	}

	// Count should be 1.
	count, err := q.CountFlagsByTarget(ctx, db.CountFlagsByTargetParams{
		TargetType: db.FlagTargetTypePost,
		TargetID:   post.ID,
	})
	if err != nil {
		t.Fatalf("CountFlagsByTarget: %v", err)
	}
	if count != 1 {
		t.Errorf("flag count = %d, want 1", count)
	}
}

// ---------------------------------------------------------------------------
// Session tests
// ---------------------------------------------------------------------------

func TestCreateAndGetSession(t *testing.T) {
	ctx, _, q := setup(t)
	agent := createTestAgent(t, ctx, q, "sessioner")

	expiresAt := pgtype.Timestamptz{
		Time:  time.Now().Add(24 * time.Hour),
		Valid: true,
	}
	session, err := q.CreateSession(ctx, db.CreateSessionParams{
		Token:     "test-token-abc",
		AgentID:   agent.ID,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if session.Token != "test-token-abc" {
		t.Errorf("token = %q, want %q", session.Token, "test-token-abc")
	}

	// Read back via GetSession (includes agent info via JOIN).
	got, err := q.GetSession(ctx, "test-token-abc")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.AgentUsername != "sessioner" {
		t.Errorf("agent_username = %q, want %q", got.AgentUsername, "sessioner")
	}
	if got.AgentID != agent.ID {
		t.Errorf("agent_id = %d, want %d", got.AgentID, agent.ID)
	}

	// Delete and verify it's gone.
	err = q.DeleteSession(ctx, "test-token-abc")
	if err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	_, err = q.GetSession(ctx, "test-token-abc")
	if err == nil {
		t.Fatal("expected error after deleting session, got nil")
	}
}
