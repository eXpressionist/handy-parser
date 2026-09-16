package extract

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/eXpressionist/handy-parser/internal/model"
)

var nonNumber = regexp.MustCompile(`[^0-9,\.\-]+`)

func Normalize(raw, valueType, currency string) (model.Observation, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return model.Observation{}, fmt.Errorf("empty extracted value")
	}
	if valueType == model.ValueText {
		text := strings.Join(strings.FieldsFunc(raw, unicode.IsSpace), " ")
		return model.Observation{Normalized: text, Display: text, Raw: raw, Currency: currency}, nil
	}
	minor, err := parseMinor(raw)
	if err != nil {
		return model.Observation{}, err
	}
	display := formatMinor(minor)
	if currency != "" {
		display += " " + currency
	}
	return model.Observation{Normalized: strconv.FormatInt(minor, 10), Display: display, Raw: raw, Currency: currency}, nil
}

func parseMinor(raw string) (int64, error) {
	s := nonNumber.ReplaceAllString(raw, "")
	negative := strings.HasPrefix(s, "-")
	s = strings.ReplaceAll(s, "-", "")
	if s == "" {
		return 0, fmt.Errorf("value %q has no number", raw)
	}
	lastComma, lastDot := strings.LastIndex(s, ","), strings.LastIndex(s, ".")
	sep := lastComma
	if lastDot > sep {
		sep = lastDot
	}
	whole, frac := s, ""
	if sep >= 0 {
		candidate := s[sep+1:]
		if len(candidate) == 1 || len(candidate) == 2 {
			whole, frac = s[:sep], candidate
		}
	}
	whole = strings.NewReplacer(",", "", ".", "").Replace(whole)
	if whole == "" {
		whole = "0"
	}
	if len(frac) == 1 {
		frac += "0"
	}
	if frac == "" {
		frac = "00"
	}
	if len(frac) > 2 {
		return 0, fmt.Errorf("value %q has unsupported precision", raw)
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse price %q: %w", raw, err)
	}
	f, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse price %q: %w", raw, err)
	}
	value := w*100 + f
	if negative {
		value = -value
	}
	return value, nil
}

func formatMinor(v int64) string {
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	return fmt.Sprintf("%s%d.%02d", sign, v/100, v%100)
}
