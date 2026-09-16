package web

import (
	"log/slog"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestLoginDoesNotDependOnBrowserOriginHeader(t *testing.T) {
	server, err := New(nil, nil, nil, nil, "correct-password", "Europe/Moscow", slog.Default())
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

func TestSameOriginAcceptsDirectExternalAddress(t *testing.T) {
	r := httptest.NewRequest("POST", "http://5.181.187.148:8080/login", nil)
	r.Header.Set("Origin", "http://5.181.187.148:8080")
	if !sameOrigin(r) {
		t.Fatal("direct external origin was rejected")
	}
}

func TestSameOriginAcceptsBrowserFetchMetadata(t *testing.T) {
	r := httptest.NewRequest("POST", "http://internal:8080/login", nil)
	r.Header.Set("Origin", "null")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	if !sameOrigin(r) {
		t.Fatal("same-origin fetch metadata was rejected")
	}
}

func TestSameOriginAcceptsForwardedHost(t *testing.T) {
	r := httptest.NewRequest("POST", "http://internal:8080/login", nil)
	r.Header.Set("Origin", "https://parser.example.com")
	r.Header.Set("X-Forwarded-Host", "parser.example.com")
	if !sameOrigin(r) {
		t.Fatal("forwarded public host was rejected")
	}
}

func TestSameOriginRejectsCrossSitePost(t *testing.T) {
	r := httptest.NewRequest("POST", "http://5.181.187.148:8080/login", nil)
	r.Header.Set("Origin", "https://attacker.example")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	if sameOrigin(r) {
		t.Fatal("cross-site request was accepted")
	}
}
