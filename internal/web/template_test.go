package web

import (
	"log/slog"
	"testing"
)

func TestTemplatesParse(t *testing.T) {
	if _, err := New(nil, nil, nil, nil, "test", "Europe/Moscow", slog.Default()); err != nil {
		t.Fatal(err)
	}
}
