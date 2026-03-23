package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	db "github.com/trishtzy/warren/db/generated"
)

var (
	ErrTitleRequired = errors.New("title is required")
	ErrTitleTooLong  = errors.New("title must be at most 300 characters")
	ErrInvalidURL    = errors.New("url must start with http:// or https://")
	ErrURLAndBody    = errors.New("a post can have a url or a body, but not both")
	ErrBodyTooLong   = errors.New("body must be at most 10000 characters")
	ErrPrivateURL    = errors.New("url must not point to a private or internal address")
)

// PostQuerier defines the database methods required by PostService.
type PostQuerier interface {
	CreatePost(ctx context.Context, arg db.CreatePostParams) (db.Post, error)
	GetPostByID(ctx context.Context, id int64) (db.GetPostByIDRow, error)
	GetPostsByURL(ctx context.Context, url *string) ([]db.GetPostsByURLRow, error)
	ListPostsByNew(ctx context.Context, arg db.ListPostsByNewParams) ([]db.ListPostsByNewRow, error)
	CountPosts(ctx context.Context) (int64, error)
	CreateVote(ctx context.Context, arg db.CreateVoteParams) (db.Vote, error)
	DeleteVote(ctx context.Context, arg db.DeleteVoteParams) error
	GetVote(ctx context.Context, arg db.GetVoteParams) (db.Vote, error)
	UpdatePostScore(ctx context.Context, arg db.UpdatePostScoreParams) error
	CountVotesByPost(ctx context.Context, postID int64) (int64, error)
	ListVotedPostIDsByAgent(ctx context.Context, agentID int64) ([]int64, error)
}

// PostStore extends PostQuerier with transaction support.
type PostStore interface {
	PostQuerier
	// ExecTx runs fn within a database transaction, passing a transactional PostQuerier.
	// The transaction is committed if fn returns nil, rolled back otherwise.
	ExecTx(ctx context.Context, fn func(PostQuerier) error) error
}

// PgPostStore wraps a pgxpool.Pool and db.Queries to implement PostStore.
type PgPostStore struct {
	*db.Queries
	pool interface {
		Begin(ctx context.Context) (pgx.Tx, error)
	}
}

// NewPgPostStore creates a PgPostStore from a pool.
func NewPgPostStore(queries *db.Queries, pool interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}) *PgPostStore {
	return &PgPostStore{Queries: queries, pool: pool}
}

// ExecTx begins a transaction, calls fn with a transactional Queries, and commits or rolls back.
func (s *PgPostStore) ExecTx(ctx context.Context, fn func(PostQuerier) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.WithTx(tx)
	if err := fn(qtx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// PostService handles post submission and retrieval.
type PostService struct {
	store  PostStore
	client *http.Client
}

// NewPostService creates a new PostService.
func NewPostService(store PostStore) *PostService {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, ErrPrivateURL
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("resolving host: %w", err)
			}
			for _, ip := range ips {
				if isPrivateIP(ip.IP) {
					return nil, ErrPrivateURL
				}
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
		},
	}
	return &PostService{
		store: store,
		client: &http.Client{
			Timeout:   5 * time.Second,
			Transport: transport,
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
	if utf8.RuneCountInString(title) > 300 {
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
			dupes, err := s.store.GetPostsByURL(ctx, urlPtr)
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

	// Use a transaction to atomically create the post, vote, and update score.
	var post db.Post
	var count int64
	err := s.store.ExecTx(ctx, func(q PostQuerier) error {
		var txErr error
		post, txErr = q.CreatePost(ctx, db.CreatePostParams{
			AgentID: agentID,
			Title:   title,
			Url:     urlPtr,
			Body:    bodyPtr,
			Domain:  domainPtr,
		})
		if txErr != nil {
			return fmt.Errorf("creating post: %w", txErr)
		}

		// Auto-upvote: create vote and update score.
		_, txErr = q.CreateVote(ctx, db.CreateVoteParams{
			AgentID: agentID,
			PostID:  post.ID,
		})
		if txErr != nil {
			return fmt.Errorf("auto-upvoting: %w", txErr)
		}

		count, txErr = q.CountVotesByPost(ctx, post.ID)
		if txErr != nil {
			return fmt.Errorf("counting votes: %w", txErr)
		}
		txErr = q.UpdatePostScore(ctx, db.UpdatePostScoreParams{
			Score: int32(count),
			ID:    post.ID,
		})
		if txErr != nil {
			return fmt.Errorf("updating score: %w", txErr)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	post.Score = int32(count)
	return &SubmitResult{Post: post}, nil
}

// Upvote creates a vote for the given agent on the given post and updates the score.
// It is idempotent — voting on an already-voted post is a no-op and returns false.
func (s *PostService) Upvote(ctx context.Context, agentID, postID int64) (voted bool, err error) {
	err = s.store.ExecTx(ctx, func(q PostQuerier) error {
		// Check if already voted.
		_, txErr := q.GetVote(ctx, db.GetVoteParams{AgentID: agentID, PostID: postID})
		if txErr == nil {
			// Already voted — no-op.
			voted = false
			return nil
		}

		_, txErr = q.CreateVote(ctx, db.CreateVoteParams{AgentID: agentID, PostID: postID})
		if txErr != nil {
			return fmt.Errorf("creating vote: %w", txErr)
		}

		count, txErr := q.CountVotesByPost(ctx, postID)
		if txErr != nil {
			return fmt.Errorf("counting votes: %w", txErr)
		}
		txErr = q.UpdatePostScore(ctx, db.UpdatePostScoreParams{Score: int32(count), ID: postID})
		if txErr != nil {
			return fmt.Errorf("updating score: %w", txErr)
		}

		voted = true
		return nil
	})
	return voted, err
}

// Unvote removes a vote for the given agent on the given post and updates the score.
// It is idempotent — unvoting when not voted is a no-op and returns false.
func (s *PostService) Unvote(ctx context.Context, agentID, postID int64) (removed bool, err error) {
	err = s.store.ExecTx(ctx, func(q PostQuerier) error {
		// Check if vote exists.
		_, txErr := q.GetVote(ctx, db.GetVoteParams{AgentID: agentID, PostID: postID})
		if txErr != nil {
			// Not voted — no-op.
			removed = false
			return nil
		}

		txErr = q.DeleteVote(ctx, db.DeleteVoteParams{AgentID: agentID, PostID: postID})
		if txErr != nil {
			return fmt.Errorf("deleting vote: %w", txErr)
		}

		count, txErr := q.CountVotesByPost(ctx, postID)
		if txErr != nil {
			return fmt.Errorf("counting votes: %w", txErr)
		}
		txErr = q.UpdatePostScore(ctx, db.UpdatePostScoreParams{Score: int32(count), ID: postID})
		if txErr != nil {
			return fmt.Errorf("updating score: %w", txErr)
		}

		removed = true
		return nil
	})
	return removed, err
}

// VotedPostIDs returns the set of post IDs the agent has voted on.
func (s *PostService) VotedPostIDs(ctx context.Context, agentID int64) (map[int64]bool, error) {
	ids, err := s.store.ListVotedPostIDsByAgent(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("listing voted post ids: %w", err)
	}
	m := make(map[int64]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m, nil
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

// isPrivateIP returns true if the IP is loopback, private, link-local, or unspecified.
func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified()
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
