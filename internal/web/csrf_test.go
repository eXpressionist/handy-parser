package web

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestLoginDoesNotDependOnBrowserOriginHeader(t *testing.T) {
	server, err := New(nil, nil, nil, nil, "correct-password", "Europe/Moscow", "00:00,06:00,12:00,18:00", slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"password": {"wrong-password"}}
	r := httptest.NewRequest("POST", "http://parser.example/login", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "null")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, r)
	if w.Code != 303 || !strings.HasPrefix(w.Header().Get("Location"), "/login?msg=") {
		t.Fatalf("status=%d location=%q body=%q", w.Code, w.Header().Get("Location"), w.Body.String())
	}
}

func TestAuthenticatedPostUsesSessionCSRFTokenIndependentOfOrigin(t *testing.T) {
	server := &Server{sessions: map[string]session{
		"session-token": {csrf: "csrf-token", expires: time.Now().Add(time.Hour)},
	}}
	form := url.Values{"csrf": {"csrf-token"}}
	r := httptest.NewRequest(http.MethodPost, "http://parser.example/action", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "null")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	r.AddCookie(&http.Cookie{Name: "handy_session", Value: "session-token"})
	w := httptest.NewRecorder()

	server.auth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
	}
}

func TestAuthenticatedPostRejectsInvalidCSRFToken(t *testing.T) {
	server := &Server{sessions: map[string]session{
		"session-token": {csrf: "csrf-token", expires: time.Now().Add(time.Hour)},
	}}
	form := url.Values{"csrf": {"wrong-token"}}
	r := httptest.NewRequest(http.MethodPost, "http://parser.example/action", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: "handy_session", Value: "session-token"})
	w := httptest.NewRecorder()

	server.auth(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler must not be called with an invalid CSRF token")
	})).ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
	}
}
