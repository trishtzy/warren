package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	db "github.com/trishtzy/warren/db/generated"

	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

// ---------------------------------------------------------------------------
// Mock
// ---------------------------------------------------------------------------

// mockQuerier implements AuthQuerier for testing.
type mockQuerier struct {
	agents   map[string]db.Agent // keyed by username
	sessions map[string]db.Session
}

func newMockQuerier() *mockQuerier {
	return &mockQuerier{
		agents:   make(map[string]db.Agent),
		sessions: make(map[string]db.Session),
	}
}

func (m *mockQuerier) CreateAgent(_ context.Context, arg db.CreateAgentParams) (db.Agent, error) {
	// Simulate PG unique constraint violations.
	for _, a := range m.agents {
		if a.Username == arg.Username {
			return db.Agent{}, &pgconn.PgError{
				Code:           "23505",
				ConstraintName: "agents_username_key",
			}
		}
		if a.Email == arg.Email {
			return db.Agent{}, &pgconn.PgError{
				Code:           "23505",
				ConstraintName: "agents_email_key",
			}
		}
	}
	agent := db.Agent{
		ID:           int64(len(m.agents) + 1),
		Username:     arg.Username,
		Email:        arg.Email,
		PasswordHash: arg.PasswordHash,
	}
	m.agents[arg.Username] = agent
	return agent, nil
}

var errMockNoRows = errors.New("no rows in result set")

func (m *mockQuerier) GetAgentByUsername(_ context.Context, username string) (db.Agent, error) {
	a, ok := m.agents[username]
	if !ok {
		return db.Agent{}, errMockNoRows
	}
	return a, nil
}

func (m *mockQuerier) GetAgentByEmail(_ context.Context, email string) (db.Agent, error) {
	for _, a := range m.agents {
		if a.Email == email {
			return a, nil
		}
	}
	return db.Agent{}, errMockNoRows
}

func (m *mockQuerier) CreateSession(_ context.Context, arg db.CreateSessionParams) (db.Session, error) {
	s := db.Session{
		Token:     arg.Token,
		AgentID:   arg.AgentID,
		ExpiresAt: arg.ExpiresAt,
	}
	m.sessions[arg.Token] = s
	return s, nil
}

func (m *mockQuerier) GetSession(_ context.Context, token string) (db.GetSessionRow, error) {
	s, ok := m.sessions[token]
	if !ok {
		return db.GetSessionRow{}, errMockNoRows
	}
	for _, a := range m.agents {
		if a.ID == s.AgentID {
			return db.GetSessionRow{
				Token:         s.Token,
				AgentID:       s.AgentID,
				AgentUsername: a.Username,
				IsAdmin:       a.IsAdmin,
				Banned:        a.Banned,
				ExpiresAt:     s.ExpiresAt,
			}, nil
		}
	}
	return db.GetSessionRow{}, errMockNoRows
}

func (m *mockQuerier) DeleteSession(_ context.Context, token string) error {
	delete(m.sessions, token)
	return nil
}

// addAgent inserts an agent with a bcrypt-hashed password into the mock store.
func (m *mockQuerier) addAgent(username, email, password string, banned bool) db.Agent {
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	a := db.Agent{
		ID:           int64(len(m.agents) + 1),
		Username:     username,
		Email:        email,
		PasswordHash: string(hash),
		Banned:       banned,
	}
	m.agents[username] = a
	return a
}

// ---------------------------------------------------------------------------
// Validation unit tests (existing + enhanced)
// ---------------------------------------------------------------------------

func TestValidateUsername(t *testing.T) {
	tests := []struct {
		name     string
		username string
		wantErr  error
	}{
		{"valid simple", "alice", nil},
		{"valid with underscore", "alice_bob", nil},
		{"valid with numbers", "agent007", nil},
		{"valid min length", "abc", nil},
		{"valid max length", "abcdefghijklmnopqrstuvwxyz1234", nil},
		{"too short", "ab", ErrInvalidUsername},
		{"too long", "abcdefghijklmnopqrstuvwxyz12345", ErrInvalidUsername},
		{"empty", "", ErrInvalidUsername},
		{"has space", "alice bob", ErrInvalidUsername},
		{"has dash", "alice-bob", ErrInvalidUsername},
		{"has dot", "alice.bob", ErrInvalidUsername},
		{"has special char", "alice@bob", ErrInvalidUsername},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUsername(tt.username)
			if err != tt.wantErr {
				t.Errorf("ValidateUsername(%q) = %v, want %v", tt.username, err, tt.wantErr)
			}
		})
	}
}

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		wantErr error
	}{
		{"valid full", "alice@example.com", nil},
		{"valid simple", "a@b.co", nil},
		{"valid with plus", "test+tag@domain.org", nil},
		// M2: stricter validation — must have content before @, after @, and dot in domain.
		{"missing @", "aliceexample.com", ErrInvalidEmail},
		{"empty", "", ErrInvalidEmail},
		{"only @", "@", ErrInvalidEmail},
		{"nothing before @", "@example.com", ErrInvalidEmail},
		{"nothing after @", "foo@", ErrInvalidEmail},
		{"no dot in domain", "foo@bar", ErrInvalidEmail},
		{"trailing dot", "foo@bar.", ErrInvalidEmail},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEmail(tt.email)
			if err != tt.wantErr {
				t.Errorf("ValidateEmail(%q) = %v, want %v", tt.email, err, tt.wantErr)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  error
	}{
		{"valid", "password123", nil},
		{"valid min length", "12345678", nil},
		{"too short", "1234567", ErrPasswordTooShort},
		{"empty", "", ErrPasswordTooShort},
		// M4: max length (72 bytes = bcrypt limit).
		{"valid max length", strings.Repeat("a", 72), nil},
		{"too long", strings.Repeat("a", 73), ErrPasswordTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.password)
			if err != tt.wantErr {
				t.Errorf("ValidatePassword(%q) = %v, want %v", tt.password, err, tt.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Register tests (H5)
// ---------------------------------------------------------------------------

func TestRegister_Success(t *testing.T) {
	mq := newMockQuerier()
	svc := NewAuthService(mq)

	agent, err := svc.Register(context.Background(), "Alice", "alice@example.com", "password123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// M5: username should be lowercased.
	if agent.Username != "alice" {
		t.Errorf("username = %q, want %q", agent.Username, "alice")
	}
	if agent.Email != "alice@example.com" {
		t.Errorf("email = %q, want %q", agent.Email, "alice@example.com")
	}
}

func TestRegister_DuplicateUsername(t *testing.T) {
	mq := newMockQuerier()
	svc := NewAuthService(mq)

	_, err := svc.Register(context.Background(), "alice", "alice@example.com", "password123")
	if err != nil {
		t.Fatalf("first register: %v", err)
	}

	_, err = svc.Register(context.Background(), "alice", "bob@example.com", "password456")
	if !errors.Is(err, ErrUsernameTaken) {
		t.Errorf("expected ErrUsernameTaken, got: %v", err)
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	mq := newMockQuerier()
	svc := NewAuthService(mq)

	_, err := svc.Register(context.Background(), "alice", "shared@example.com", "password123")
	if err != nil {
		t.Fatalf("first register: %v", err)
	}

	_, err = svc.Register(context.Background(), "bob", "shared@example.com", "password456")
	if !errors.Is(err, ErrEmailTaken) {
		t.Errorf("expected ErrEmailTaken, got: %v", err)
	}
}

func TestRegister_ValidationFailures(t *testing.T) {
	mq := newMockQuerier()
	svc := NewAuthService(mq)

	tests := []struct {
		name     string
		username string
		email    string
		password string
		wantErr  error
	}{
		{"short username", "ab", "a@b.com", "password123", ErrInvalidUsername},
		{"invalid username chars", "ab!cd", "a@b.com", "password123", ErrInvalidUsername},
		{"no @ in email", "alice", "aliceexample.com", "password123", ErrInvalidEmail},
		{"no domain dot", "alice", "alice@example", "password123", ErrInvalidEmail},
		{"nothing before @", "alice", "@example.com", "password123", ErrInvalidEmail},
		{"nothing after dot", "alice", "alice@example.", "password123", ErrInvalidEmail},
		{"short password", "alice", "alice@example.com", "short", ErrPasswordTooShort},
		{"long password", "alice", "alice@example.com", strings.Repeat("a", 73), ErrPasswordTooLong},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Register(context.Background(), tc.username, tc.email, tc.password)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("got %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestRegister_NormalizesUsernameToLowercase(t *testing.T) {
	mq := newMockQuerier()
	svc := NewAuthService(mq)

	agent, err := svc.Register(context.Background(), "AlIcE", "alice@example.com", "password123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agent.Username != "alice" {
		t.Errorf("username = %q, want %q", agent.Username, "alice")
	}
}

// ---------------------------------------------------------------------------
// Login tests (H5)
// ---------------------------------------------------------------------------

func TestLogin_SuccessByUsername(t *testing.T) {
	mq := newMockQuerier()
	mq.addAgent("alice", "alice@example.com", "password123", false)
	svc := NewAuthService(mq)

	token, err := svc.Login(context.Background(), "alice", "password123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty token")
	}
	if len(mq.sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(mq.sessions))
	}
}

func TestLogin_SuccessByEmail(t *testing.T) {
	mq := newMockQuerier()
	mq.addAgent("alice", "alice@example.com", "password123", false)
	svc := NewAuthService(mq)

	token, err := svc.Login(context.Background(), "alice@example.com", "password123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty token")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	mq := newMockQuerier()
	mq.addAgent("alice", "alice@example.com", "password123", false)
	svc := NewAuthService(mq)

	_, err := svc.Login(context.Background(), "alice", "wrongpassword")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got: %v", err)
	}
}

func TestLogin_NonexistentUser(t *testing.T) {
	mq := newMockQuerier()
	svc := NewAuthService(mq)

	_, err := svc.Login(context.Background(), "nobody", "password123")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got: %v", err)
	}
}

func TestLogin_BannedUser(t *testing.T) {
	mq := newMockQuerier()
	mq.addAgent("banned_user", "banned@example.com", "password123", true)
	svc := NewAuthService(mq)

	_, err := svc.Login(context.Background(), "banned_user", "password123")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials for banned user, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Logout tests (H5)
// ---------------------------------------------------------------------------

func TestLogout_Success(t *testing.T) {
	mq := newMockQuerier()
	mq.addAgent("alice", "alice@example.com", "password123", false)
	svc := NewAuthService(mq)

	token, err := svc.Login(context.Background(), "alice", "password123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if len(mq.sessions) != 1 {
		t.Fatalf("expected 1 session before logout, got %d", len(mq.sessions))
	}

	err = svc.Logout(context.Background(), token)
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	if len(mq.sessions) != 0 {
		t.Errorf("expected 0 sessions after logout, got %d", len(mq.sessions))
	}
}

func TestLogout_InvalidSession(t *testing.T) {
	mq := newMockQuerier()
	svc := NewAuthService(mq)

	// Logging out a nonexistent token should not error (DELETE is idempotent).
	err := svc.Logout(context.Background(), "nonexistent-token")
	if err != nil {
		t.Errorf("expected no error for invalid session logout, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Session token hashing (H3)
// ---------------------------------------------------------------------------

func TestSessionTokenIsHashed(t *testing.T) {
	mq := newMockQuerier()
	mq.addAgent("alice", "alice@example.com", "password123", false)
	svc := NewAuthService(mq)

	token, err := svc.Login(context.Background(), "alice", "password123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	// The raw token should NOT be stored in the sessions map.
	if _, exists := mq.sessions[token]; exists {
		t.Error("raw token should not be stored in the database; expected hashed token")
	}

	// The hashed token SHOULD be stored.
	hashed := hashToken(token)
	if _, exists := mq.sessions[hashed]; !exists {
		t.Error("hashed token should be stored in the database")
	}
}

func TestGetSessionAgent_UsesHashedToken(t *testing.T) {
	mq := newMockQuerier()
	mq.addAgent("alice", "alice@example.com", "password123", false)
	svc := NewAuthService(mq)

	token, err := svc.Login(context.Background(), "alice", "password123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	row, err := svc.GetSessionAgent(context.Background(), token)
	if err != nil {
		t.Fatalf("GetSessionAgent: %v", err)
	}
	if row.AgentUsername != "alice" {
		t.Errorf("agent_username = %q, want %q", row.AgentUsername, "alice")
	}
}
