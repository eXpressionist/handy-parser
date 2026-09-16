package extract

import (
	"testing"

	"github.com/eXpressionist/handy-parser/internal/model"
)

func TestNormalizePrice(t *testing.T) {
	tests := []struct{ raw, want string }{{"137.95₾", "13795"}, {"1 299,00 ₽", "129900"}, {"89,6", "8960"}, {"1,299.00", "129900"}, {"1.299,00", "129900"}, {"42", "4200"}}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := Normalize(tt.raw, model.ValuePrice, "GEL")
			if err != nil {
				t.Fatal(err)
			}
			if got.Normalized != tt.want {
				t.Fatalf("got %s, want %s", got.Normalized, tt.want)
			}
		})
	}
}

func TestNormalizeText(t *testing.T) {
	got, err := Normalize("  in\n stock  ", model.ValueText, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Normalized != "in stock" {
		t.Fatalf("got %q", got.Normalized)
	}
}

func TestNormalizeRejectsEmptyAndNonNumeric(t *testing.T) {
	for _, raw := range []string{"", "₾"} {
		if _, err := Normalize(raw, model.ValuePrice, "GEL"); err == nil {
			t.Fatalf("expected error for %q", raw)
		}
	}
}
