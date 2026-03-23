package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	db "github.com/trishtzy/warren/db/generated"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidUsername    = errors.New("username must be 3-30 alphanumeric characters or underscores")
	ErrInvalidEmail      = errors.New("invalid email address")
	ErrPasswordTooShort  = errors.New("password must be at least 8 characters")
	ErrPasswordTooLong   = errors.New("password must be at most 72 bytes")
	ErrUsernameTaken     = errors.New("username is already taken")
	ErrEmailTaken        = errors.New("email is already taken")
	ErrInvalidCredentials = errors.New("invalid username/email or password")
)

var usernameRegexp = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// AuthQuerier defines the database methods required by AuthService.
type AuthQuerier interface {
	CreateAgent(ctx context.Context, arg db.CreateAgentParams) (db.Agent, error)
	GetAgentByUsername(ctx context.Context, username string) (db.Agent, error)
	GetAgentByEmail(ctx context.Context, email string) (db.Agent, error)
	CreateSession(ctx context.Context, arg db.CreateSessionParams) (db.Session, error)
	GetSession(ctx context.Context, token string) (db.GetSessionRow, error)
	DeleteSession(ctx context.Context, token string) error
}

// AuthService handles agent registration, login, and session management.
type AuthService struct {
	queries AuthQuerier
}

// NewAuthService creates a new AuthService.
func NewAuthService(queries AuthQuerier) *AuthService {
	return &AuthService{queries: queries}
}

// ValidateUsername checks that the username is 3-30 alphanumeric/underscore characters.
func ValidateUsername(username string) error {
	if len(username) < 3 || len(username) > 30 {
		return ErrInvalidUsername
	}
	if !usernameRegexp.MatchString(username) {
		return ErrInvalidUsername
	}
	return nil
}

// ValidateEmail checks that the email has content before @, content after @,
// and a dot in the domain part.
func ValidateEmail(email string) error {
	at := strings.LastIndex(email, "@")
	if at < 1 {
		return ErrInvalidEmail
	}
	domain := email[at+1:]
	if domain == "" || !strings.Contains(domain, ".") {
		return ErrInvalidEmail
	}
	// Ensure there's content after the last dot.
	lastDot := strings.LastIndex(domain, ".")
	if lastDot == len(domain)-1 {
		return ErrInvalidEmail
	}
	return nil
}

// ValidatePassword checks that the password is at least 8 characters
// and at most 72 bytes (bcrypt's limit).
func ValidatePassword(password string) error {
	if len(password) < 8 {
		return ErrPasswordTooShort
	}
	if len(password) > 72 {
		return ErrPasswordTooLong
	}
	return nil
}

// hashToken returns the hex-encoded SHA-256 hash of a session token.
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// Register creates a new agent with the given credentials.
// Username is normalized to lowercase before storage.
func (s *AuthService) Register(ctx context.Context, username, email, password string) (db.Agent, error) {
	// M5: normalize username to lowercase.
	username = strings.ToLower(username)

	if err := ValidateUsername(username); err != nil {
		return db.Agent{}, err
	}
	if err := ValidateEmail(email); err != nil {
		return db.Agent{}, err
	}
	if err := ValidatePassword(password); err != nil {
		return db.Agent{}, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return db.Agent{}, fmt.Errorf("hashing password: %w", err)
	}

	// H1: Just INSERT and catch unique violation instead of SELECT-then-INSERT
	// (eliminates TOCTOU race and two unnecessary round-trips).
	agent, err := s.queries.CreateAgent(ctx, db.CreateAgentParams{
		Username:     username,
		Email:        email,
		PasswordHash: string(hash),
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			switch pgErr.ConstraintName {
			case "agents_username_key":
				return db.Agent{}, ErrUsernameTaken
			case "agents_email_key":
				return db.Agent{}, ErrEmailTaken
			default:
				return db.Agent{}, fmt.Errorf("creating agent: %w", err)
			}
		}
		return db.Agent{}, fmt.Errorf("creating agent: %w", err)
	}

	return agent, nil
}

// Login authenticates an agent by username or email and password, returning a session token.
func (s *AuthService) Login(ctx context.Context, identifier, password string) (string, error) {
	// M5: normalize identifier to lowercase for username lookup.
	lowerIdentifier := strings.ToLower(identifier)

	// Try username first, then email.
	agent, err := s.queries.GetAgentByUsername(ctx, lowerIdentifier)
	if err != nil {
		agent, err = s.queries.GetAgentByEmail(ctx, identifier)
	}
	if err != nil {
		return "", ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(agent.PasswordHash), []byte(password)); err != nil {
		return "", ErrInvalidCredentials
	}

	// H2: Reject banned users after password check.
	if agent.Banned {
		return "", ErrInvalidCredentials
	}

	// Generate a cryptographically secure session token.
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("generating token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)

	expiresAt := pgtype.Timestamptz{
		Time:  time.Now().Add(30 * 24 * time.Hour),
		Valid: true,
	}

	// H3: Store SHA-256(token) in the database, return the raw token to the caller.
	_, err = s.queries.CreateSession(ctx, db.CreateSessionParams{
		Token:     hashToken(token),
		AgentID:   agent.ID,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return "", fmt.Errorf("creating session: %w", err)
	}

	return token, nil
}

// Logout deletes the session identified by the given token.
func (s *AuthService) Logout(ctx context.Context, token string) error {
	// H3: Hash the token before looking it up in the database.
	return s.queries.DeleteSession(ctx, hashToken(token))
}

// GetSessionAgent retrieves the session and associated agent info for a token.
func (s *AuthService) GetSessionAgent(ctx context.Context, token string) (db.GetSessionRow, error) {
	// H3: Hash the token before looking it up in the database.
	return s.queries.GetSession(ctx, hashToken(token))
}
