package service

import (
	"context"
	"html/template"
	"strings"
	"testing"

	db "github.com/trishtzy/warren/db/generated"

	"github.com/jackc/pgx/v5/pgtype"
)

// mockCommentQuerier implements CommentQuerier for testing.
type mockCommentQuerier struct {
	comments      map[int64]db.Comment
	nextCommentID int64
}

func newMockCommentQuerier() *mockCommentQuerier {
	return &mockCommentQuerier{
		comments:      make(map[int64]db.Comment),
		nextCommentID: 1,
	}
}

func (m *mockCommentQuerier) CreateComment(_ context.Context, arg db.CreateCommentParams) (db.Comment, error) {
	c := db.Comment{
		ID:              m.nextCommentID,
		AgentID:         arg.AgentID,
		PostID:          arg.PostID,
		ParentCommentID: arg.ParentCommentID,
		Body:            arg.Body,
	}
	m.comments[c.ID] = c
	m.nextCommentID++
	return c, nil
}

func (m *mockCommentQuerier) GetCommentByID(_ context.Context, id int64) (db.GetCommentByIDRow, error) {
	c, ok := m.comments[id]
	if !ok {
		return db.GetCommentByIDRow{}, context.Canceled // stand-in for not found
	}
	return db.GetCommentByIDRow{
		ID:              c.ID,
		AgentID:         c.AgentID,
		PostID:          c.PostID,
		ParentCommentID: c.ParentCommentID,
		Body:            c.Body,
		AgentUsername:   "testuser",
	}, nil
}

func (m *mockCommentQuerier) ListAllCommentsByPost(_ context.Context, arg db.ListAllCommentsByPostParams) ([]db.ListAllCommentsByPostRow, error) {
	var rows []db.ListAllCommentsByPostRow
	for _, c := range m.comments {
		if c.PostID == arg.PostID {
			rows = append(rows, db.ListAllCommentsByPostRow{
				ID:              c.ID,
				AgentID:         c.AgentID,
				PostID:          c.PostID,
				ParentCommentID: c.ParentCommentID,
				Body:            c.Body,
				AgentUsername:   "testuser",
				CreatedAt:       pgtype.Timestamptz{Valid: true},
			})
		}
	}
	return rows, nil
}

func (m *mockCommentQuerier) CountCommentsByPost(_ context.Context, postID int64) (int64, error) {
	var count int64
	for _, c := range m.comments {
		if c.PostID == postID {
			count++
		}
	}
	return count, nil
}

func TestCreateComment_EmptyBody(t *testing.T) {
	svc := NewCommentService(newMockCommentQuerier())
	_, err := svc.CreateComment(context.Background(), 1, 1, nil, "")
	if err != ErrCommentBodyRequired {
		t.Errorf("expected ErrCommentBodyRequired, got %v", err)
	}
}

func TestCreateComment_WhitespaceBody(t *testing.T) {
	svc := NewCommentService(newMockCommentQuerier())
	_, err := svc.CreateComment(context.Background(), 1, 1, nil, "   \n\t  ")
	if err != ErrCommentBodyRequired {
		t.Errorf("expected ErrCommentBodyRequired, got %v", err)
	}
}

func TestCreateComment_TooLong(t *testing.T) {
	svc := NewCommentService(newMockCommentQuerier())
	body := strings.Repeat("a", 10001)
	_, err := svc.CreateComment(context.Background(), 1, 1, nil, body)
	if err != ErrCommentBodyTooLong {
		t.Errorf("expected ErrCommentBodyTooLong, got %v", err)
	}
}

func TestCreateComment_Success(t *testing.T) {
	svc := NewCommentService(newMockCommentQuerier())
	c, err := svc.CreateComment(context.Background(), 1, 1, nil, "hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Body != "hello world" {
		t.Errorf("expected body 'hello world', got %q", c.Body)
	}
	if c.ParentCommentID != nil {
		t.Errorf("expected nil parent, got %v", c.ParentCommentID)
	}
}

func TestCreateComment_WithParent(t *testing.T) {
	mock := newMockCommentQuerier()
	// Pre-populate a parent comment on post 1.
	mock.comments[1] = db.Comment{ID: 1, AgentID: 1, PostID: 1, Body: "parent"}
	mock.nextCommentID = 2

	svc := NewCommentService(mock)
	parentID := int64(1)
	c, err := svc.CreateComment(context.Background(), 1, 1, &parentID, "reply")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.ParentCommentID == nil || *c.ParentCommentID != 1 {
		t.Errorf("expected parent 1, got %v", c.ParentCommentID)
	}
}

func TestCreateComment_CrossPostReply(t *testing.T) {
	mock := newMockCommentQuerier()
	// Parent comment belongs to post 1.
	mock.comments[1] = db.Comment{ID: 1, AgentID: 1, PostID: 1, Body: "parent on post 1"}
	mock.nextCommentID = 2

	svc := NewCommentService(mock)
	parentID := int64(1)
	// Try to reply on post 2 with a parent from post 1 — should be rejected.
	_, err := svc.CreateComment(context.Background(), 1, 2, &parentID, "cross-post reply")
	if err != ErrParentCommentWrongPost {
		t.Errorf("expected ErrParentCommentWrongPost, got %v", err)
	}
}

func TestCreateComment_NonexistentParent(t *testing.T) {
	svc := NewCommentService(newMockCommentQuerier())
	parentID := int64(999)
	_, err := svc.CreateComment(context.Background(), 1, 1, &parentID, "orphan reply")
	if err != ErrParentCommentWrongPost {
		t.Errorf("expected ErrParentCommentWrongPost, got %v", err)
	}
}

func TestRenderMarkdown_Bold(t *testing.T) {
	svc := NewCommentService(newMockCommentQuerier())
	html := svc.RenderMarkdown("**bold**")
	if !strings.Contains(string(html), "<strong>bold</strong>") {
		t.Errorf("expected bold rendering, got %q", html)
	}
}

func TestRenderMarkdown_Link(t *testing.T) {
	svc := NewCommentService(newMockCommentQuerier())
	html := svc.RenderMarkdown("[example](https://example.com)")
	if !strings.Contains(string(html), `href="https://example.com"`) {
		t.Errorf("expected link rendering, got %q", html)
	}
}

func TestRenderMarkdown_CodeBlock(t *testing.T) {
	svc := NewCommentService(newMockCommentQuerier())
	html := svc.RenderMarkdown("```\ncode\n```")
	if !strings.Contains(string(html), "<code>") {
		t.Errorf("expected code block rendering, got %q", html)
	}
}

func TestRenderMarkdown_LinkRelAttributes(t *testing.T) {
	svc := NewCommentService(newMockCommentQuerier())
	html := string(svc.RenderMarkdown("[example](https://example.com)"))
	if !strings.Contains(html, `rel="nofollow noreferrer noopener"`) &&
		!strings.Contains(html, `rel="noreferrer nofollow noopener"`) &&
		!strings.Contains(html, `rel="noopener noreferrer nofollow"`) {
		t.Errorf("expected rel with noreferrer and noopener, got %q", html)
	}
}

func TestRenderMarkdown_XSSSanitization(t *testing.T) {
	svc := NewCommentService(newMockCommentQuerier())
	html := svc.RenderMarkdown(`<script>alert("xss")</script>`)
	if strings.Contains(string(html), "<script>") {
		t.Errorf("expected script tag to be sanitized, got %q", html)
	}
}

func TestBuildCommentTree_Empty(t *testing.T) {
	svc := NewCommentService(newMockCommentQuerier())
	roots, count, err := svc.BuildCommentTree(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roots) != 0 {
		t.Errorf("expected 0 roots, got %d", len(roots))
	}
	if count != 0 {
		t.Errorf("expected 0 count, got %d", count)
	}
}

func TestBuildCommentTree_Nested(t *testing.T) {
	mock := newMockCommentQuerier()
	// Create a parent comment and a reply.
	mock.comments[1] = db.Comment{ID: 1, AgentID: 1, PostID: 1, Body: "parent"}
	parentID := int64(1)
	mock.comments[2] = db.Comment{ID: 2, AgentID: 2, PostID: 1, ParentCommentID: &parentID, Body: "child"}
	mock.nextCommentID = 3

	svc := NewCommentService(mock)
	roots, count, err := svc.BuildCommentTree(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 count, got %d", count)
	}
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	if len(roots[0].Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(roots[0].Children))
	}
	if roots[0].Children[0].Depth != 1 {
		t.Errorf("expected child depth 1, got %d", roots[0].Children[0].Depth)
	}
}

func TestFlattenTree_IndentCap(t *testing.T) {
	// Build a deeply nested tree manually.
	root := &CommentTree{ID: 1, Depth: 0, BodyHTML: template.HTML("a")}
	current := root
	for i := 2; i <= 8; i++ {
		child := &CommentTree{ID: int64(i), Depth: i - 1, BodyHTML: template.HTML("a")}
		current.Children = []*CommentTree{child}
		current = child
	}

	flat := FlattenTree([]*CommentTree{root})
	if len(flat) != 8 {
		t.Fatalf("expected 8 flat comments, got %d", len(flat))
	}
	// Depth 5 (index 5) should be capped at 5*20=100px.
	if flat[5].IndentPx != 100 {
		t.Errorf("expected 100px at depth 5, got %d", flat[5].IndentPx)
	}
	// Depth 7 (index 7) should also be capped at 100px.
	if flat[7].IndentPx != 100 {
		t.Errorf("expected 100px at depth 7, got %d", flat[7].IndentPx)
	}
}
