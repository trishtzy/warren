package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	db "github.com/trishtzy/warren/db/generated"
)

var (
	ErrTitleRequired = errors.New("title is required")
	ErrTitleTooLong  = errors.New("title must be at most 300 characters")
	ErrInvalidURL    = errors.New("url must start with http:// or https://")
	ErrURLAndBody    = errors.New("a post can have a url or a body, but not both")
	ErrBodyTooLong   = errors.New("body must be at most 10000 characters")
)

// PostQuerier defines the database methods required by PostService.
type PostQuerier interface {
	CreatePost(ctx context.Context, arg db.CreatePostParams) (db.Post, error)
	GetPostByID(ctx context.Context, id int64) (db.GetPostByIDRow, error)
	GetPostsByURL(ctx context.Context, url *string) ([]db.GetPostsByURLRow, error)
	ListPostsByNew(ctx context.Context, arg db.ListPostsByNewParams) ([]db.ListPostsByNewRow, error)
	CountPosts(ctx context.Context) (int64, error)
	CreateVote(ctx context.Context, arg db.CreateVoteParams) (db.Vote, error)
	UpdatePostScore(ctx context.Context, arg db.UpdatePostScoreParams) error
	CountVotesByPost(ctx context.Context, postID int64) (int64, error)
}

// PostService handles post submission and retrieval.
type PostService struct {
	queries PostQuerier
	client  *http.Client
}

// NewPostService creates a new PostService.
func NewPostService(queries PostQuerier) *PostService {
	return &PostService{
		queries: queries,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// SubmitResult contains the created post and any duplicate URL warnings.
type SubmitResult struct {
	Post       db.Post
	Duplicates []db.GetPostsByURLRow
}

// Submit validates input, creates a post, and auto-upvotes it.
// If force is false and the URL has duplicates, it returns duplicates without creating the post.
func (s *PostService) Submit(ctx context.Context, agentID int64, title, rawURL, body string, force bool) (*SubmitResult, error) {
	title = strings.TrimSpace(title)
	rawURL = strings.TrimSpace(rawURL)
	body = strings.TrimSpace(body)

	if title == "" {
		return nil, ErrTitleRequired
	}
	if len(title) > 300 {
		return nil, ErrTitleTooLong
	}
	if rawURL != "" && body != "" {
		return nil, ErrURLAndBody
	}
	if len(body) > 10000 {
		return nil, ErrBodyTooLong
	}

	var urlPtr, bodyPtr, domainPtr *string

	if rawURL != "" {
		if err := validateURL(rawURL); err != nil {
			return nil, err
		}
		urlPtr = &rawURL
		domain := ExtractDomain(rawURL)
		if domain != "" {
			domainPtr = &domain
		}

		// Check for duplicate URLs unless forced.
		if !force {
			dupes, err := s.queries.GetPostsByURL(ctx, urlPtr)
			if err != nil {
				return nil, fmt.Errorf("checking duplicate url: %w", err)
			}
			if len(dupes) > 0 {
				return &SubmitResult{Duplicates: dupes}, nil
			}
		}
	}

	if body != "" {
		bodyPtr = &body
	}

	post, err := s.queries.CreatePost(ctx, db.CreatePostParams{
		AgentID: agentID,
		Title:   title,
		Url:     urlPtr,
		Body:    bodyPtr,
		Domain:  domainPtr,
	})
	if err != nil {
		return nil, fmt.Errorf("creating post: %w", err)
	}

	// Auto-upvote: create vote and update score.
	_, err = s.queries.CreateVote(ctx, db.CreateVoteParams{
		AgentID: agentID,
		PostID:  post.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("auto-upvoting: %w", err)
	}

	count, err := s.queries.CountVotesByPost(ctx, post.ID)
	if err != nil {
		return nil, fmt.Errorf("counting votes: %w", err)
	}
	err = s.queries.UpdatePostScore(ctx, db.UpdatePostScoreParams{
		Score: int32(count),
		ID:    post.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("updating score: %w", err)
	}
	post.Score = int32(count)

	return &SubmitResult{Post: post}, nil
}

// FetchPageTitle fetches a URL and extracts the <title> tag content.
func (s *PostService) FetchPageTitle(rawURL string) (string, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "RabbitHole/1.0 (+https://rabbithole.dev)")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching url: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	// Read up to 1MB to find the title.
	limited := io.LimitReader(resp.Body, 1<<20)
	bodyBytes, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("reading body: %w", err)
	}

	return extractTitle(string(bodyBytes)), nil
}

// extractTitle finds the content between <title> and </title> tags.
func extractTitle(html string) string {
	lower := strings.ToLower(html)
	start := strings.Index(lower, "<title")
	if start == -1 {
		return ""
	}
	// Skip past the closing > of the opening tag.
	start = strings.Index(lower[start:], ">")
	if start == -1 {
		return ""
	}
	// Adjust start to be relative to html.
	startIdx := strings.Index(lower, "<title")
	start = startIdx + start + 1

	end := strings.Index(lower[start:], "</title>")
	if end == -1 {
		return ""
	}

	title := strings.TrimSpace(html[start : start+end])
	// Collapse whitespace.
	fields := strings.Fields(title)
	return strings.Join(fields, " ")
}

// ExtractDomain extracts the hostname from a URL, stripping the "www." prefix.
func ExtractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	host := u.Hostname()
	host = strings.TrimPrefix(host, "www.")
	return host
}

// validateURL checks that the URL starts with http:// or https://.
func validateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ErrInvalidURL
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ErrInvalidURL
	}
	if u.Host == "" {
		return ErrInvalidURL
	}
	return nil
}
