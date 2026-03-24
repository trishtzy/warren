package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestParsePage(t *testing.T) {
	tests := []struct {
		query string
		want  int
	}{
		{"", 1},
		{"?p=1", 1},
		{"?p=2", 2},
		{"?p=30", 30},
		{"?p=100", 100},
		{"?p=0", 1},
		{"?p=-1", 1},
		{"?p=abc", 1},
		{"?p=101", 1},  // exceeds maxPage
		{"?p=9999", 1}, // exceeds maxPage
	}
	for _, tt := range tests {
		r := httptest.NewRequest("GET", "/"+tt.query, nil)
		got := parsePage(r)
		if got != tt.want {
			t.Errorf("parsePage(%q) = %d, want %d", tt.query, got, tt.want)
		}
	}
}

func TestBuildPostItems(t *testing.T) {
	url1 := "https://example.com"
	domain1 := "example.com"
	now := pgtype.Timestamptz{Time: time.Now(), Valid: true}

	rows := []postRow{
		{ID: 1, Title: "First", Url: &url1, Domain: &domain1, Score: 10, AgentUsername: "alice", CreatedAt: now, CommentCount: 3},
		{ID: 2, Title: "Second", Url: nil, Domain: nil, Score: 5, AgentUsername: "bob", CreatedAt: now, CommentCount: 0},
	}
	votedSet := map[int64]bool{1: true}

	// Page 1, offset 0.
	items := buildPostItems(rows, 0, votedSet)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Rank != 1 {
		t.Errorf("item 0 rank = %d, want 1", items[0].Rank)
	}
	if items[1].Rank != 2 {
		t.Errorf("item 1 rank = %d, want 2", items[1].Rank)
	}
	if items[0].URL != url1 {
		t.Errorf("item 0 URL = %q, want %q", items[0].URL, url1)
	}
	if items[0].Domain != domain1 {
		t.Errorf("item 0 Domain = %q, want %q", items[0].Domain, domain1)
	}
	if items[1].URL != "" {
		t.Errorf("item 1 URL = %q, want empty", items[1].URL)
	}
	if !items[0].Voted {
		t.Error("item 0 should be voted")
	}
	if items[1].Voted {
		t.Error("item 1 should not be voted")
	}
	if items[0].CommentCount != 3 {
		t.Errorf("item 0 CommentCount = %d, want 3", items[0].CommentCount)
	}

	// Page 2, offset 30 — rank numbers should start at 31.
	items = buildPostItems(rows, 30, votedSet)
	if items[0].Rank != 31 {
		t.Errorf("page 2 item 0 rank = %d, want 31", items[0].Rank)
	}
	if items[1].Rank != 32 {
		t.Errorf("page 2 item 1 rank = %d, want 32", items[1].Rank)
	}
}

func TestBuildPostItems_NilVotedSet(t *testing.T) {
	now := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	rows := []postRow{
		{ID: 1, Title: "Test", Score: 1, AgentUsername: "alice", CreatedAt: now},
	}

	// nil votedSet should not panic.
	items := buildPostItems(rows, 0, nil)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Voted {
		t.Error("expected Voted=false with nil votedSet")
	}
}

func TestBuildPostItems_Empty(t *testing.T) {
	items := buildPostItems(nil, 0, nil)
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestPostHandlerRoutes(t *testing.T) {
	h := &PostHandler{tmpl: make(Templates), gravity: 1.5}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Verify /new pattern is registered by checking that the mux finds a handler.
	req := httptest.NewRequest("GET", "/new", nil)
	_, pattern := mux.Handler(req)
	if pattern == "" {
		t.Error("GET /new should be registered, got no matching pattern")
	}
}
