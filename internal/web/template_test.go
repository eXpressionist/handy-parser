package web

import (
	"log/slog"
	"testing"
)

func TestTemplatesParse(t *testing.T) {
	if _, err := New(nil, nil, nil, nil, "test", "Europe/Moscow", "00:00,06:00,12:00,18:00", slog.Default()); err != nil {
		t.Fatal(err)
	}
}
