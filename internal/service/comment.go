package service

import (
	"bytes"
	"context"
	"errors"
	"html/template"
	"strings"
	"unicode/utf8"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"

	db "github.com/trishtzy/warren/db/generated"
	"github.com/trishtzy/warren/internal/timeutil"
)

var (
	ErrCommentBodyRequired    = errors.New("comment body is required")
	ErrCommentBodyTooLong     = errors.New("comment body must be at most 10000 characters")
	ErrParentCommentWrongPost = errors.New("parent comment does not belong to this post")
)

// MaxCommentsPerPost is the maximum number of comments loaded for tree building.
const MaxCommentsPerPost = 2000

// CommentQuerier defines the database methods required by CommentService.
type CommentQuerier interface {
	CreateComment(ctx context.Context, arg db.CreateCommentParams) (db.Comment, error)
	GetCommentByID(ctx context.Context, id int64) (db.GetCommentByIDRow, error)
	ListAllCommentsByPost(ctx context.Context, arg db.ListAllCommentsByPostParams) ([]db.ListAllCommentsByPostRow, error)
	CountCommentsByPost(ctx context.Context, postID int64) (int64, error)
}

// CommentService handles comment creation and retrieval.
type CommentService struct {
	queries  CommentQuerier
	md       goldmark.Markdown
	sanitize *bluemonday.Policy
}

// NewCommentService creates a new CommentService.
func NewCommentService(queries CommentQuerier) *CommentService {
	policy := bluemonday.UGCPolicy()
	policy.RequireNoReferrerOnLinks(true)
	policy.AddTargetBlankToFullyQualifiedLinks(true)
	return &CommentService{
		queries:  queries,
		md:       goldmark.New(),
		sanitize: policy,
	}
}

// CreateComment validates and creates a comment.
func (s *CommentService) CreateComment(ctx context.Context, agentID, postID int64, parentCommentID *int64, body string) (db.Comment, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return db.Comment{}, ErrCommentBodyRequired
	}
	if utf8.RuneCountInString(body) > 10000 {
		return db.Comment{}, ErrCommentBodyTooLong
	}

	// Validate that parent comment belongs to the same post.
	if parentCommentID != nil {
		parent, err := s.queries.GetCommentByID(ctx, *parentCommentID)
		if err != nil {
			return db.Comment{}, ErrParentCommentWrongPost
		}
		if parent.PostID != postID {
			return db.Comment{}, ErrParentCommentWrongPost
		}
	}

	return s.queries.CreateComment(ctx, db.CreateCommentParams{
		AgentID:         agentID,
		PostID:          postID,
		ParentCommentID: parentCommentID,
		Body:            body,
	})
}

// CommentTree represents a comment with its nested replies.
type CommentTree struct {
	ID              int64
	AgentID         int64
	PostID          int64
	ParentCommentID *int64
	Body            string
	BodyHTML        template.HTML
	Hidden          bool
	AgentUsername   string
	TimeAgo         string
	Depth           int
	Children        []*CommentTree
}

// BuildCommentTree fetches comments for a post (up to MaxCommentsPerPost) and builds a nested tree.
func (s *CommentService) BuildCommentTree(ctx context.Context, postID int64) ([]*CommentTree, int, error) {
	rows, err := s.queries.ListAllCommentsByPost(ctx, db.ListAllCommentsByPostParams{
		PostID:      postID,
		MaxComments: MaxCommentsPerPost,
	})
	if err != nil {
		return nil, 0, err
	}

	// Build lookup map and tree.
	nodeMap := make(map[int64]*CommentTree, len(rows))
	var roots []*CommentTree

	for _, r := range rows {
		node := &CommentTree{
			ID:              r.ID,
			AgentID:         r.AgentID,
			PostID:          r.PostID,
			ParentCommentID: r.ParentCommentID,
			Hidden:          r.Hidden,
			AgentUsername:   r.AgentUsername,
			TimeAgo:         timeutil.Ago(r.CreatedAt.Time),
		}
		if r.Hidden {
			node.Body = "[hidden]"
			node.BodyHTML = template.HTML("<em>[hidden]</em>")
		} else {
			node.Body = r.Body
			node.BodyHTML = s.RenderMarkdown(r.Body)
		}
		nodeMap[r.ID] = node
	}

	// Link children to parents.
	for _, node := range nodeMap {
		if node.ParentCommentID != nil {
			parent, ok := nodeMap[*node.ParentCommentID]
			if ok {
				node.Depth = parent.Depth + 1
				parent.Children = append(parent.Children, node)
			} else {
				// Orphaned comment — treat as root.
				roots = append(roots, node)
			}
		} else {
			roots = append(roots, node)
		}
	}

	// Sort children by insertion order (they come from DB sorted by created_at ASC,
	// and we iterate nodeMap which doesn't preserve order). Re-sort using original order.
	orderIndex := make(map[int64]int, len(rows))
	for i, r := range rows {
		orderIndex[r.ID] = i
	}
	sortTree(roots, orderIndex)

	return roots, len(rows), nil
}

// sortTree recursively sorts children by their original DB order.
func sortTree(nodes []*CommentTree, orderIndex map[int64]int) {
	sortByIndex(nodes, orderIndex)
	for _, n := range nodes {
		if len(n.Children) > 0 {
			sortTree(n.Children, orderIndex)
		}
	}
}

func sortByIndex(nodes []*CommentTree, orderIndex map[int64]int) {
	// Simple insertion sort — comment counts are small.
	for i := 1; i < len(nodes); i++ {
		key := nodes[i]
		j := i - 1
		for j >= 0 && orderIndex[nodes[j].ID] > orderIndex[key.ID] {
			nodes[j+1] = nodes[j]
			j--
		}
		nodes[j+1] = key
	}
}

// FlatComment is a pre-computed comment for template rendering.
type FlatComment struct {
	ID            int64
	PostID        int64
	AgentUsername string
	TimeAgo       string
	BodyHTML      template.HTML
	Hidden        bool
	IndentPx      int
}

// FlattenTree converts a comment tree into a flat slice with pre-computed indent pixels.
// The visual indent is capped at 5 levels (100px).
func FlattenTree(roots []*CommentTree) []FlatComment {
	var result []FlatComment
	var walk func(nodes []*CommentTree)
	walk = func(nodes []*CommentTree) {
		for _, n := range nodes {
			depth := n.Depth
			if depth > 5 {
				depth = 5
			}
			result = append(result, FlatComment{
				ID:            n.ID,
				PostID:        n.PostID,
				AgentUsername: n.AgentUsername,
				TimeAgo:       n.TimeAgo,
				BodyHTML:      n.BodyHTML,
				Hidden:        n.Hidden,
				IndentPx:      depth * 20,
			})
			walk(n.Children)
		}
	}
	walk(roots)
	return result
}

// RenderMarkdown converts markdown to sanitized HTML.
func (s *CommentService) RenderMarkdown(src string) template.HTML {
	var buf bytes.Buffer
	if err := s.md.Convert([]byte(src), &buf); err != nil {
		// On error, return escaped text.
		return template.HTML(template.HTMLEscapeString(src))
	}
	safe := s.sanitize.SanitizeBytes(buf.Bytes())
	return template.HTML(safe)
}
