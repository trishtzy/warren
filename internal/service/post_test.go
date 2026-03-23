package service

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	db "github.com/trishtzy/warren/db/generated"

	"github.com/jackc/pgx/v5"
)

// mockPostStore implements PostStore for testing.
type mockPostStore struct {
	posts      map[int64]db.Post
	votes      map[voteKeyT]db.Vote
	nextPostID int64
	nextVoteID int64
}

type voteKeyT struct{ agentID, postID int64 }

func newMockPostStore() *mockPostStore {
	return &mockPostStore{
		posts:      make(map[int64]db.Post),
		votes:      make(map[voteKeyT]db.Vote),
		nextPostID: 1,
		nextVoteID: 1,
	}
}

func (m *mockPostStore) CreatePost(_ context.Context, arg db.CreatePostParams) (db.Post, error) {
	post := db.Post{
		ID:      m.nextPostID,
		AgentID: arg.AgentID,
		Title:   arg.Title,
		Url:     arg.Url,
		Body:    arg.Body,
		Domain:  arg.Domain,
		Score:   1,
	}
	m.posts[post.ID] = post
	m.nextPostID++
	return post, nil
}

func (m *mockPostStore) GetPostByID(_ context.Context, id int64) (db.GetPostByIDRow, error) {
	p, ok := m.posts[id]
	if !ok {
		return db.GetPostByIDRow{}, pgx.ErrNoRows
	}
	return db.GetPostByIDRow{
		ID:      p.ID,
		AgentID: p.AgentID,
		Title:   p.Title,
		Url:     p.Url,
		Body:    p.Body,
		Domain:  p.Domain,
		Score:   p.Score,
	}, nil
}

func (m *mockPostStore) GetPostsByURL(_ context.Context, url *string) ([]db.GetPostsByURLRow, error) {
	var results []db.GetPostsByURLRow
	if url == nil {
		return results, nil
	}
	for _, p := range m.posts {
		if p.Url != nil && *p.Url == *url {
			results = append(results, db.GetPostsByURLRow{
				ID:    p.ID,
				Title: p.Title,
				Url:   p.Url,
			})
		}
	}
	return results, nil
}

func (m *mockPostStore) ListPostsByNew(_ context.Context, _ db.ListPostsByNewParams) ([]db.ListPostsByNewRow, error) {
	return nil, nil
}

func (m *mockPostStore) CountPosts(_ context.Context) (int64, error) {
	return int64(len(m.posts)), nil
}

func (m *mockPostStore) CreateVote(_ context.Context, arg db.CreateVoteParams) (db.Vote, error) {
	vote := db.Vote{
		ID:      m.nextVoteID,
		AgentID: arg.AgentID,
		PostID:  arg.PostID,
	}
	m.votes[voteKeyT{arg.AgentID, arg.PostID}] = vote
	m.nextVoteID++
	return vote, nil
}

func (m *mockPostStore) UpdatePostScore(_ context.Context, arg db.UpdatePostScoreParams) error {
	if p, ok := m.posts[arg.ID]; ok {
		p.Score = arg.Score
		m.posts[arg.ID] = p
	}
	return nil
}

func (m *mockPostStore) DeleteVote(_ context.Context, arg db.DeleteVoteParams) error {
	delete(m.votes, voteKeyT{arg.AgentID, arg.PostID})
	return nil
}

func (m *mockPostStore) GetVote(_ context.Context, arg db.GetVoteParams) (db.Vote, error) {
	v, ok := m.votes[voteKeyT{arg.AgentID, arg.PostID}]
	if !ok {
		return db.Vote{}, pgx.ErrNoRows
	}
	return v, nil
}

func (m *mockPostStore) CountVotesByPost(_ context.Context, postID int64) (int64, error) {
	var count int64
	for _, v := range m.votes {
		if v.PostID == postID {
			count++
		}
	}
	return count, nil
}

func (m *mockPostStore) ListVotedPostIDsByAgent(_ context.Context, agentID int64) ([]int64, error) {
	var ids []int64
	for k := range m.votes {
		if k.agentID == agentID {
			ids = append(ids, k.postID)
		}
	}
	return ids, nil
}

// ExecTx runs fn directly against the mock store (no real transaction needed in tests).
func (m *mockPostStore) ExecTx(_ context.Context, fn func(PostQuerier) error) error {
	return fn(m)
}

func newTestService(store PostStore) *PostService {
	return &PostService{store: store, client: &http.Client{}}
}

// --- Tests ---

func TestSubmit_URLPost(t *testing.T) {
	store := newMockPostStore()
	svc := newTestService(store)

	url := "https://example.com/article"
	result, err := svc.Submit(context.Background(), 1, "Test Title", url, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Post.Title != "Test Title" {
		t.Errorf("got title %q, want %q", result.Post.Title, "Test Title")
	}
	if result.Post.Url == nil || *result.Post.Url != url {
		t.Errorf("got url %v, want %q", result.Post.Url, url)
	}
	if result.Post.Domain == nil || *result.Post.Domain != "example.com" {
		t.Errorf("got domain %v, want %q", result.Post.Domain, "example.com")
	}
	if result.Post.Body != nil {
		t.Errorf("got body %v, want nil", result.Post.Body)
	}
}

func TestSubmit_TextPost(t *testing.T) {
	store := newMockPostStore()
	svc := newTestService(store)

	result, err := svc.Submit(context.Background(), 1, "Ask RH: Question?", "", "Some body text", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Post.Body == nil || *result.Post.Body != "Some body text" {
		t.Errorf("got body %v, want %q", result.Post.Body, "Some body text")
	}
	if result.Post.Url != nil {
		t.Errorf("got url %v, want nil", result.Post.Url)
	}
}

func TestSubmit_TitleRequired(t *testing.T) {
	svc := newTestService(newMockPostStore())
	_, err := svc.Submit(context.Background(), 1, "", "", "body", false)
	if err != ErrTitleRequired {
		t.Errorf("got %v, want ErrTitleRequired", err)
	}
}

func TestSubmit_TitleTooLong(t *testing.T) {
	svc := newTestService(newMockPostStore())
	longTitle := strings.Repeat("a", 301)
	_, err := svc.Submit(context.Background(), 1, longTitle, "", "", false)
	if err != ErrTitleTooLong {
		t.Errorf("got %v, want ErrTitleTooLong", err)
	}
}

func TestSubmit_URLAndBody(t *testing.T) {
	svc := newTestService(newMockPostStore())
	_, err := svc.Submit(context.Background(), 1, "Title", "https://example.com", "body", false)
	if err != ErrURLAndBody {
		t.Errorf("got %v, want ErrURLAndBody", err)
	}
}

func TestSubmit_InvalidURL(t *testing.T) {
	svc := newTestService(newMockPostStore())
	_, err := svc.Submit(context.Background(), 1, "Title", "ftp://example.com", "", false)
	if err != ErrInvalidURL {
		t.Errorf("got %v, want ErrInvalidURL", err)
	}
}

func TestSubmit_BodyTooLong(t *testing.T) {
	svc := newTestService(newMockPostStore())
	longBody := strings.Repeat("a", 10001)
	_, err := svc.Submit(context.Background(), 1, "Title", "", longBody, false)
	if err != ErrBodyTooLong {
		t.Errorf("got %v, want ErrBodyTooLong", err)
	}
}

func TestSubmit_DuplicateURLDetection(t *testing.T) {
	store := newMockPostStore()
	svc := newTestService(store)

	// First submission succeeds.
	_, err := svc.Submit(context.Background(), 1, "First", "https://example.com/dup", "", false)
	if err != nil {
		t.Fatalf("first submit failed: %v", err)
	}

	// Second submission with same URL returns duplicates.
	result, err := svc.Submit(context.Background(), 2, "Second", "https://example.com/dup", "", false)
	if err != nil {
		t.Fatalf("second submit failed: %v", err)
	}
	if len(result.Duplicates) == 0 {
		t.Error("expected duplicates, got none")
	}

	// Force resubmission succeeds.
	result, err = svc.Submit(context.Background(), 2, "Second", "https://example.com/dup", "", true)
	if err != nil {
		t.Fatalf("force submit failed: %v", err)
	}
	if len(result.Duplicates) != 0 {
		t.Errorf("expected no duplicates on force, got %d", len(result.Duplicates))
	}
	if result.Post.Title != "Second" {
		t.Errorf("got title %q, want %q", result.Post.Title, "Second")
	}
}

func TestSubmit_TitleTrimmed(t *testing.T) {
	svc := newTestService(newMockPostStore())
	result, err := svc.Submit(context.Background(), 1, "  Hello World  ", "", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Post.Title != "Hello World" {
		t.Errorf("got title %q, want %q", result.Post.Title, "Hello World")
	}
}

func TestSubmit_AutoUpvote(t *testing.T) {
	store := newMockPostStore()
	svc := newTestService(store)

	result, err := svc.Submit(context.Background(), 1, "Title", "", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Post.Score != 1 {
		t.Errorf("got score %d, want 1 (auto-upvote)", result.Post.Score)
	}
	// Verify the vote was created.
	if len(store.votes) != 1 {
		t.Errorf("got %d votes, want 1", len(store.votes))
	}
}

func TestSubmit_RuneCount(t *testing.T) {
	svc := newTestService(newMockPostStore())

	// 300 multi-byte characters should be accepted.
	title := strings.Repeat("\u00e9", 300) // é is 2 bytes each, 600 bytes total
	result, err := svc.Submit(context.Background(), 1, title, "", "", false)
	if err != nil {
		t.Fatalf("unexpected error for 300-rune title: %v", err)
	}
	if result.Post.Title != title {
		t.Error("title should be preserved")
	}

	// 301 multi-byte characters should be rejected.
	title301 := strings.Repeat("\u00e9", 301)
	_, err = svc.Submit(context.Background(), 1, title301, "", "", false)
	if err != ErrTitleTooLong {
		t.Errorf("got %v, want ErrTitleTooLong for 301-rune title", err)
	}
}

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		url    string
		domain string
	}{
		{"https://example.com/path", "example.com"},
		{"https://www.example.com/path", "example.com"},
		{"https://sub.example.com", "sub.example.com"},
		{"https://www.sub.example.com", "sub.example.com"},
		{"not-a-url", ""},
	}
	for _, tt := range tests {
		got := ExtractDomain(tt.url)
		if got != tt.domain {
			t.Errorf("ExtractDomain(%q) = %q, want %q", tt.url, got, tt.domain)
		}
	}
}

func TestExtractTitle(t *testing.T) {
	tests := []struct {
		html  string
		title string
	}{
		{`<html><head><title>Hello World</title></head></html>`, "Hello World"},
		{`<html><head><TITLE>Case Test</TITLE></head></html>`, "Case Test"},
		{`<html><head><title>  Spaces   Here  </title></head></html>`, "Spaces Here"},
		{`<html><head><title lang="en">With Attr</title></head></html>`, "With Attr"},
		{`<html><body>no title</body></html>`, ""},
		{``, ""},
	}
	for _, tt := range tests {
		got := extractTitle(tt.html)
		if got != tt.title {
			t.Errorf("extractTitle(%q) = %q, want %q", tt.html, got, tt.title)
		}
	}
}

func TestFetchPageTitle(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("expected User-Agent header")
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>Test Page</title></head></html>`))
	}))
	defer ts.Close()

	svc := &PostService{client: ts.Client()}

	title, err := svc.FetchPageTitle(ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "Test Page" {
		t.Errorf("got title %q, want %q", title, "Test Page")
	}
}

func TestFetchPageTitle_NonOK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	svc := &PostService{client: ts.Client()}

	_, err := svc.FetchPageTitle(ts.URL)
	if err == nil {
		t.Error("expected error for non-200 status")
	}
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		url   string
		valid bool
	}{
		{"https://example.com", true},
		{"http://example.com/path", true},
		{"ftp://example.com", false},
		{"javascript:alert(1)", false},
		{"not-a-url", false},
		{"://missing-scheme", false},
	}
	for _, tt := range tests {
		err := validateURL(tt.url)
		if (err == nil) != tt.valid {
			t.Errorf("validateURL(%q): got err=%v, want valid=%v", tt.url, err, tt.valid)
		}
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip      string
		private bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"192.168.1.1", true},
		{"172.16.0.1", true},
		{"169.254.1.1", true},
		{"0.0.0.0", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"::1", true},
		{"fe80::1", true},
	}
	for _, tt := range tests {
		ip := net.ParseIP(tt.ip)
		got := isPrivateIP(ip)
		if got != tt.private {
			t.Errorf("isPrivateIP(%q) = %v, want %v", tt.ip, got, tt.private)
		}
	}
}

func TestUpvote(t *testing.T) {
	store := newMockPostStore()
	svc := newTestService(store)

	// Create a post first (auto-upvotes with agent 1).
	result, err := svc.Submit(context.Background(), 1, "Title", "", "", false)
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}
	postID := result.Post.ID

	// Agent 2 upvotes.
	voted, err := svc.Upvote(context.Background(), 2, postID)
	if err != nil {
		t.Fatalf("upvote failed: %v", err)
	}
	if !voted {
		t.Error("expected voted=true for new vote")
	}

	// Verify score updated.
	count, _ := store.CountVotesByPost(context.Background(), postID)
	if count != 2 {
		t.Errorf("got %d votes, want 2", count)
	}

	// Agent 2 upvotes again — should be idempotent.
	voted, err = svc.Upvote(context.Background(), 2, postID)
	if err != nil {
		t.Fatalf("second upvote failed: %v", err)
	}
	if voted {
		t.Error("expected voted=false for duplicate vote")
	}
}

func TestUnvote(t *testing.T) {
	store := newMockPostStore()
	svc := newTestService(store)

	// Create a post (auto-upvotes with agent 1).
	result, err := svc.Submit(context.Background(), 1, "Title", "", "", false)
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}
	postID := result.Post.ID

	// Agent 2 upvotes then unvotes.
	_, _ = svc.Upvote(context.Background(), 2, postID)
	removed, err := svc.Unvote(context.Background(), 2, postID)
	if err != nil {
		t.Fatalf("unvote failed: %v", err)
	}
	if !removed {
		t.Error("expected removed=true")
	}

	// Verify score updated.
	count, _ := store.CountVotesByPost(context.Background(), postID)
	if count != 1 {
		t.Errorf("got %d votes, want 1", count)
	}

	// Unvote again — should be idempotent.
	removed, err = svc.Unvote(context.Background(), 2, postID)
	if err != nil {
		t.Fatalf("second unvote failed: %v", err)
	}
	if removed {
		t.Error("expected removed=false for non-existent vote")
	}
}

func TestVotedPostIDs(t *testing.T) {
	store := newMockPostStore()
	svc := newTestService(store)

	// Create two posts.
	r1, _ := svc.Submit(context.Background(), 1, "Post 1", "", "", false)
	r2, _ := svc.Submit(context.Background(), 1, "Post 2", "", "", false)

	// Agent 1 auto-voted on both. Check VotedPostIDs.
	voted, err := svc.VotedPostIDs(context.Background(), 1)
	if err != nil {
		t.Fatalf("VotedPostIDs failed: %v", err)
	}
	if !voted[r1.Post.ID] || !voted[r2.Post.ID] {
		t.Errorf("expected both posts voted, got %v", voted)
	}

	// Agent 2 has no votes.
	voted, err = svc.VotedPostIDs(context.Background(), 2)
	if err != nil {
		t.Fatalf("VotedPostIDs failed: %v", err)
	}
	if len(voted) != 0 {
		t.Errorf("expected empty voted set, got %v", voted)
	}
}

// Verify that ExecTx on the mock store passes through the same querier.
func TestMockExecTx(t *testing.T) {
	store := newMockPostStore()
	called := false
	err := store.ExecTx(context.Background(), func(q PostQuerier) error {
		called = true
		// Verify the querier is the store itself.
		post, txErr := q.CreatePost(context.Background(), db.CreatePostParams{
			AgentID: 1,
			Title:   "tx test",
		})
		if txErr != nil {
			return txErr
		}
		if post.Title != "tx test" {
			t.Errorf("got title %q, want %q", post.Title, "tx test")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ExecTx failed: %v", err)
	}
	if !called {
		t.Error("ExecTx callback was not called")
	}
}
