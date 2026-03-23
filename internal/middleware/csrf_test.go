package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// dummyHandler is a simple handler that writes 200 OK.
var dummyHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
})

func TestCSRF_SetsCookieOnGET(t *testing.T) {
	handler := CSRF(dummyHandler)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	cookies := rec.Result().Cookies()
	var found bool
	for _, c := range cookies {
		if c.Name == csrfCookieName {
			found = true
			if !isValidTokenFormat(c.Value) {
				t.Errorf("cookie value is not a valid token: %q", c.Value)
			}
			if !c.HttpOnly {
				t.Error("cookie should be HttpOnly")
			}
			if !c.Secure {
				t.Error("cookie should be Secure")
			}
		}
	}
	if !found {
		t.Fatal("CSRF cookie not set on GET request")
	}
}

func TestCSRF_InjectsTokenInContext(t *testing.T) {
	var got string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = CSRFToken(r)
		w.WriteHeader(http.StatusOK)
	})

	handler := CSRF(inner)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if got == "" {
		t.Fatal("CSRFToken returned empty string")
	}
	// Masked token should be longer than the raw token (pad + masked).
	if len(got) != csrfTokenBytes*4 {
		t.Errorf("expected masked token length %d, got %d", csrfTokenBytes*4, len(got))
	}
}

func TestCSRF_POST_ValidToken(t *testing.T) {
	handler := CSRF(dummyHandler)

	// Step 1: GET to obtain cookie and masked token.
	var maskedToken string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		maskedToken = CSRFToken(r)
		w.WriteHeader(http.StatusOK)
	})
	getHandler := CSRF(inner)
	getReq := httptest.NewRequest(http.MethodGet, "/", nil)
	getRec := httptest.NewRecorder()
	getHandler.ServeHTTP(getRec, getReq)

	var cookieValue string
	for _, c := range getRec.Result().Cookies() {
		if c.Name == csrfCookieName {
			cookieValue = c.Value
		}
	}
	if cookieValue == "" {
		t.Fatal("no CSRF cookie from GET")
	}

	// Step 2: POST with valid token.
	form := url.Values{}
	form.Set(csrfFieldName, maskedToken)
	postReq := httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: cookieValue})
	postRec := httptest.NewRecorder()

	handler.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid token, got %d: %s", postRec.Code, postRec.Body.String())
	}
}

func TestCSRF_POST_MissingToken(t *testing.T) {
	handler := CSRF(dummyHandler)

	// GET to obtain cookie.
	getReq := httptest.NewRequest(http.MethodGet, "/", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	var cookieValue string
	for _, c := range getRec.Result().Cookies() {
		if c.Name == csrfCookieName {
			cookieValue = c.Value
		}
	}

	// POST without csrf_token form field.
	postReq := httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader("title=hello"))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: cookieValue})
	postRec := httptest.NewRecorder()

	handler.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for missing token, got %d", postRec.Code)
	}
}

func TestCSRF_POST_InvalidToken(t *testing.T) {
	handler := CSRF(dummyHandler)

	// GET to obtain cookie.
	getReq := httptest.NewRequest(http.MethodGet, "/", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	var cookieValue string
	for _, c := range getRec.Result().Cookies() {
		if c.Name == csrfCookieName {
			cookieValue = c.Value
		}
	}

	// POST with garbage token.
	form := url.Values{}
	form.Set(csrfFieldName, "not-a-valid-token")
	postReq := httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: cookieValue})
	postRec := httptest.NewRecorder()

	handler.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for invalid token, got %d", postRec.Code)
	}
}

func TestCSRF_POST_WrongToken(t *testing.T) {
	handler := CSRF(dummyHandler)

	// GET to obtain cookie.
	getReq := httptest.NewRequest(http.MethodGet, "/", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	var cookieValue string
	for _, c := range getRec.Result().Cookies() {
		if c.Name == csrfCookieName {
			cookieValue = c.Value
		}
	}

	// Generate a different valid-looking token (mask a different underlying token).
	differentToken, err := generateToken()
	if err != nil {
		t.Fatal(err)
	}
	maskedDifferent, err := maskToken(differentToken)
	if err != nil {
		t.Fatal(err)
	}

	form := url.Values{}
	form.Set(csrfFieldName, maskedDifferent)
	postReq := httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: cookieValue})
	postRec := httptest.NewRecorder()

	handler.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for wrong token, got %d", postRec.Code)
	}
}

func TestCSRF_POST_NoCookie(t *testing.T) {
	handler := CSRF(dummyHandler)

	// POST with a form token but no cookie — the middleware will generate a
	// new cookie, but the form token won't match the fresh cookie.
	form := url.Values{}
	form.Set(csrfFieldName, "aaaa")
	postReq := httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postRec := httptest.NewRecorder()

	handler.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for no-cookie POST, got %d", postRec.Code)
	}
}

func TestCSRF_GET_DoesNotValidate(t *testing.T) {
	handler := CSRF(dummyHandler)

	// GET request should pass even without any token.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestMaskUnmaskRoundTrip(t *testing.T) {
	token, err := generateToken()
	if err != nil {
		t.Fatal(err)
	}

	masked, err := maskToken(token)
	if err != nil {
		t.Fatal(err)
	}

	unmasked, ok := unmaskToken(masked)
	if !ok {
		t.Fatal("unmask failed")
	}

	if unmasked != token {
		t.Errorf("round-trip failed: got %q, want %q", unmasked, token)
	}
}

func TestMaskProducesDifferentValues(t *testing.T) {
	token, err := generateToken()
	if err != nil {
		t.Fatal(err)
	}

	masked1, err := maskToken(token)
	if err != nil {
		t.Fatal(err)
	}
	masked2, err := maskToken(token)
	if err != nil {
		t.Fatal(err)
	}

	if masked1 == masked2 {
		t.Error("masked tokens should differ due to random pad")
	}

	// Both should unmask to the same token.
	u1, ok1 := unmaskToken(masked1)
	u2, ok2 := unmaskToken(masked2)
	if !ok1 || !ok2 {
		t.Fatal("unmask failed")
	}
	if u1 != u2 {
		t.Error("unmasked tokens should be equal")
	}
}

func TestCSRF_ReusesCookieOnSubsequentGET(t *testing.T) {
	handler := CSRF(dummyHandler)

	// First GET — sets cookie.
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	var cookieValue string
	for _, c := range rec1.Result().Cookies() {
		if c.Name == csrfCookieName {
			cookieValue = c.Value
		}
	}
	if cookieValue == "" {
		t.Fatal("no cookie set")
	}

	// Second GET with existing cookie — should NOT set a new cookie.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.AddCookie(&http.Cookie{Name: csrfCookieName, Value: cookieValue})
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	for _, c := range rec2.Result().Cookies() {
		if c.Name == csrfCookieName {
			t.Error("should not re-set cookie when valid cookie already exists")
		}
	}
}
