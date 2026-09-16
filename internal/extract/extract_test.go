package extract

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eXpressionist/handy-parser/internal/model"
)

func TestHTMLAttributeExtraction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<meta name="product:price:amount" content="137.95">`))
	}))
	defer server.Close()
	e := New(NewClient(2*time.Second, 1024, true))
	obs, err := e.Observe(context.Background(), model.Watch{URL: server.URL, Kind: model.KindHTML, Selector: `meta[name="product:price:amount"]`, Attribute: "content", ValueType: model.ValuePrice, Currency: "GEL"})
	if err != nil {
		t.Fatal(err)
	}
	if obs.Normalized != "13795" {
		t.Fatalf("got %s", obs.Normalized)
	}
}

func TestHTMLRequiresExactlyOneMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<b class="price">1</b><b class="price">2</b>`))
	}))
	defer server.Close()
	e := New(NewClient(2*time.Second, 1024, true))
	_, err := e.Observe(context.Background(), model.Watch{URL: server.URL, Kind: model.KindHTML, Selector: ".price", ValueType: model.ValuePrice})
	if err == nil {
		t.Fatal("expected ambiguity error")
	}
}

func TestClientBlocksLocalTargets(t *testing.T) {
	c := NewClient(time.Second, 1024, false)
	_, _, err := c.Get(context.Background(), "http://127.0.0.1/", nil)
	if err == nil {
		t.Fatal("expected local target to be blocked")
	}
}

func TestClientLimitsDecompressedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("123456")) }))
	defer server.Close()
	c := NewClient(time.Second, 5, true)
	_, _, err := c.Get(context.Background(), server.URL, nil)
	if err == nil {
		t.Fatal("expected size error")
	}
}
