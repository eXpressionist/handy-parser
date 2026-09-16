package web

import (
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eXpressionist/handy-parser/internal/model"
)

func TestTemplatesParse(t *testing.T) {
	if _, err := New(nil, nil, nil, nil, "test", "Europe/Moscow", "00:00,06:00,12:00,18:00", slog.Default()); err != nil {
		t.Fatal(err)
	}
}

func TestEditFormIncludesPreviewForExistingWatch(t *testing.T) {
	server, err := New(nil, nil, nil, nil, "test", "Europe/Moscow", "00:00,06:00,12:00,18:00", slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	server.render(w, pageData{Preview: true, Edit: true, CSRF: "token", Form: model.Watch{ID: 42, Name: "Product"}})
	body := w.Body.String()
	if !strings.Contains(body, `formaction="/watches/42/preview"`) {
		t.Fatalf("edit preview action is missing: %q", body)
	}
	if !strings.Contains(body, `action="/watches/42"`) {
		t.Fatalf("edit update action is missing: %q", body)
	}
}
