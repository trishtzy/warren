package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	db "github.com/trishtzy/warren/db/generated"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidUsername    = errors.New("username must be 3-30 alphanumeric characters or underscores")
	ErrInvalidEmail       = errors.New("invalid email address")
	ErrPasswordTooShort   = errors.New("password must be at least 8 characters")
	ErrUsernameTaken      = errors.New("username is already taken")
	ErrEmailTaken         = errors.New("email is already taken")
	ErrInvalidCredentials = errors.New("invalid username/email or password")
)

var usernameRegexp = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// AuthService handles agent registration, login, and session management.
type AuthService struct {
	queries *db.Queries
}

// NewAuthService creates a new AuthService.
func NewAuthService(queries *db.Queries) *AuthService {
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

// ValidateEmail checks that the email contains an @ sign.
func ValidateEmail(email string) error {
	if !strings.Contains(email, "@") {
		return ErrInvalidEmail
	}
	return nil
}

// ValidatePassword checks that the password is at least 8 characters.
func ValidatePassword(password string) error {
	if len(password) < 8 {
		return ErrPasswordTooShort
	}
	return nil
}

// Register creates a new agent with the given credentials.
func (s *AuthService) Register(ctx context.Context, username, email, password string) (db.Agent, error) {
	if err := ValidateUsername(username); err != nil {
		return db.Agent{}, err
	}
	if err := ValidateEmail(email); err != nil {
		return db.Agent{}, err
	}
	if err := ValidatePassword(password); err != nil {
		return db.Agent{}, err
	}

	// Check if username is taken.
	_, err := s.queries.GetAgentByUsername(ctx, username)
	if err == nil {
		return db.Agent{}, ErrUsernameTaken
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.Agent{}, fmt.Errorf("checking username: %w", err)
	}

	// Check if email is taken.
	_, err = s.queries.GetAgentByEmail(ctx, email)
	if err == nil {
		return db.Agent{}, ErrEmailTaken
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.Agent{}, fmt.Errorf("checking email: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return db.Agent{}, fmt.Errorf("hashing password: %w", err)
	}

	agent, err := s.queries.CreateAgent(ctx, db.CreateAgentParams{
		Username:     username,
		Email:        email,
		PasswordHash: string(hash),
	})
	if err != nil {
		return db.Agent{}, fmt.Errorf("creating agent: %w", err)
	}

	return agent, nil
}

// Login authenticates an agent by username or email and password, returning a session token.
func (s *AuthService) Login(ctx context.Context, identifier, password string) (string, error) {
	// Try username first, then email.
	agent, err := s.queries.GetAgentByUsername(ctx, identifier)
	if errors.Is(err, pgx.ErrNoRows) {
		agent, err = s.queries.GetAgentByEmail(ctx, identifier)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrInvalidCredentials
	}
	if err != nil {
		return "", fmt.Errorf("looking up agent: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(agent.PasswordHash), []byte(password)); err != nil {
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

	_, err = s.queries.CreateSession(ctx, db.CreateSessionParams{
		Token:     token,
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
	return s.queries.DeleteSession(ctx, token)
}

// GetSessionAgent retrieves the session and associated agent info for a token.
func (s *AuthService) GetSessionAgent(ctx context.Context, token string) (db.GetSessionRow, error) {
	return s.queries.GetSession(ctx, token)
}
