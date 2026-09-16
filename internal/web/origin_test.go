package web

import (
	"net/http/httptest"
	"testing"
)

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
