package service

import "testing"

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
		{"valid", "alice@example.com", nil},
		{"valid simple", "a@b", nil},
		{"missing @", "aliceexample.com", ErrInvalidEmail},
		{"empty", "", ErrInvalidEmail},
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
