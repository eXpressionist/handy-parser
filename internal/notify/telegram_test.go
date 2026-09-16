package notify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSendHonorsTelegramRetryAfter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"ok":false,"description":"Too Many Requests","parameters":{"retry_after":7}}`))
	}))
	defer server.Close()
	tg := NewTelegram("secret-token", "42", time.Second)
	tg.endpoint = server.URL
	retry, err := tg.Send(context.Background(), "hello")
	if err == nil || retry != 7*time.Second || !IsRateLimited(err) || IsPermanent(err) {
		t.Fatalf("retry=%v err=%v permanent=%v limited=%v", retry, err, IsPermanent(err), IsRateLimited(err))
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Fatal("error contains Telegram token")
	}
}

func TestSendMarksAuthenticationFailurePermanent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"ok":false,"description":"Unauthorized"}`))
	}))
	defer server.Close()
	tg := NewTelegram("secret-token", "42", time.Second)
	tg.endpoint = server.URL
	_, err := tg.Send(context.Background(), "hello")
	if err == nil || !IsPermanent(err) {
		t.Fatalf("err=%v permanent=%v", err, IsPermanent(err))
	}
}

func TestSendSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || r.Form.Get("chat_id") != "42" || r.Form.Get("text") != "hello" {
			t.Errorf("unexpected form: %v err=%v", r.Form, err)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer server.Close()
	tg := NewTelegram("secret-token", "42", time.Second)
	tg.endpoint = server.URL
	if _, err := tg.Send(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
}
