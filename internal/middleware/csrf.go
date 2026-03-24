package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

const (
	csrfCookieName = "csrf_token"
	csrfFieldName  = "csrf_token"
	csrfTokenBytes = 32
)

// csrfContextKey is an unexported type for the CSRF context key.
type csrfContextKey struct{}

// csrfTokenKey is the context key for the masked CSRF token string.
var csrfTokenKey = csrfContextKey{}

// CSRFToken returns the masked CSRF token for the current request.
// Templates should render this value in a hidden form field.
func CSRFToken(r *http.Request) string {
	tok, _ := r.Context().Value(csrfTokenKey).(string)
	return tok
}

// generateToken returns a cryptographically random hex-encoded token.
func generateToken() (string, error) {
	b := make([]byte, csrfTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// tokensMatch performs a constant-time comparison of two token strings.
func tokensMatch(a, b string) bool {
	return hmac.Equal([]byte(a), []byte(b))
}

// isValidTokenFormat checks that a token is the expected hex length.
func isValidTokenFormat(token string) bool {
	if len(token) != csrfTokenBytes*2 {
		return false
	}
	_, err := hex.DecodeString(token)
	return err == nil
}

// maskToken XORs the token with a one-time pad and returns pad+masked as hex.
// This prevents BREACH-style compression attacks by ensuring the form value
// changes on every page load even though the underlying token stays the same.
func maskToken(token string) (string, error) {
	tokenBytes, err := hex.DecodeString(token)
	if err != nil {
		return "", err
	}
	pad := make([]byte, len(tokenBytes))
	if _, err := rand.Read(pad); err != nil {
		return "", err
	}
	masked := make([]byte, len(tokenBytes))
	for i := range tokenBytes {
		masked[i] = tokenBytes[i] ^ pad[i]
	}
	result := make([]byte, 0, len(pad)+len(masked))
	result = append(result, pad...)
	result = append(result, masked...)
	return hex.EncodeToString(result), nil
}

// unmaskToken reverses maskToken: splits pad+masked, XORs to recover original.
func unmaskToken(maskedHex string) (string, bool) {
	raw, err := hex.DecodeString(maskedHex)
	if err != nil || len(raw) != csrfTokenBytes*2 {
		return "", false
	}
	pad := raw[:csrfTokenBytes]
	masked := raw[csrfTokenBytes:]
	token := make([]byte, csrfTokenBytes)
	for i := range token {
		token[i] = pad[i] ^ masked[i]
	}
	return hex.EncodeToString(token), true
}

// CSRF returns middleware that protects against cross-site request forgery.
//
// On every request, it ensures a CSRF cookie exists and stores a masked token
// in the request context (accessible via CSRFToken). On state-changing methods
// (POST, PUT, PATCH, DELETE), it validates that the form field csrf_token
// unmasks to match the cookie value.
//
// The secureCookies parameter controls the Secure flag on the CSRF cookie.
// Set to false for local development over plain HTTP.
func CSRF(secureCookies bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var token string
		cookie, err := r.Cookie(csrfCookieName)
		if err == nil && isValidTokenFormat(cookie.Value) {
			token = cookie.Value
		} else {
			token, err = generateToken()
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name:     csrfCookieName,
				Value:    token,
				Path:     "/",
				HttpOnly: true,
				Secure:   secureCookies,
				SameSite: http.SameSiteLaxMode,
			})
		}

		// Validate on state-changing methods.
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			if parseErr := r.ParseForm(); parseErr != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			formToken := r.FormValue(csrfFieldName)
			unmasked, ok := unmaskToken(formToken)
			if !ok || !tokensMatch(token, unmasked) {
				http.Error(w, "forbidden - invalid CSRF token", http.StatusForbidden)
				return
			}
		}

		// Generate masked token for templates.
		maskedToken, maskErr := maskToken(token)
		if maskErr != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Vary", "Cookie")
		ctx := context.WithValue(r.Context(), csrfTokenKey, maskedToken)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
	}
}
