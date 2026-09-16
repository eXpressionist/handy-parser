package runner

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/eXpressionist/handy-parser/internal/model"
	"github.com/eXpressionist/handy-parser/internal/notify"
	"github.com/eXpressionist/handy-parser/internal/store"
)

type fakeObserver struct {
	observation model.Observation
	err         error
}

func (f *fakeObserver) Observe(context.Context, model.Watch) (model.Observation, error) {
	return f.observation, f.err
}

func TestRunCreatesBaselineThenChangeEvent(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "runner.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	id, err := s.CreateWatch(ctx, model.Watch{Name: "Price", URL: "https://example.com", Kind: model.KindHTML, Selector: ".price", ValueType: model.ValuePrice, Currency: "GEL", Rule: model.RuleAnyChange, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeObserver{observation: model.Observation{Normalized: "1000", Display: "10.00 GEL", Currency: "GEL"}}
	r := New(s, fake, notify.NewTelegram("", "", time.Second), time.Second, slog.Default())
	first, err := r.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.Checked != 1 || first.Changed != 0 || first.Errors != 0 {
		t.Fatalf("unexpected baseline summary: %+v", first)
	}
	w, err := s.GetWatch(ctx, id)
	if err != nil || w.LastValue != "1000" {
		t.Fatalf("baseline state: %+v err=%v", w, err)
	}
	fake.observation = model.Observation{Normalized: "900", Display: "9.00 GEL", Currency: "GEL"}
	second, err := r.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if second.Changed != 1 || second.Errors != 1 {
		t.Fatalf("unexpected change summary: %+v", second)
	}
	changes, err := s.RecentChanges(ctx, 10)
	if err != nil || len(changes) != 1 {
		t.Fatalf("changes=%d err=%v", len(changes), err)
	}
	outbox, err := s.PendingOutbox(ctx, 10)
	if err != nil || len(outbox) != 0 {
		t.Fatalf("pending should be delayed after failed delivery: %v err=%v", outbox, err)
	}
}

func TestRunSkipsWhenLeaseHeld(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "lock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	ok, err := s.AcquireRun(ctx, "other", time.Minute)
	if err != nil || !ok {
		t.Fatalf("lock: %v %v", ok, err)
	}
	r := New(s, &fakeObserver{}, notify.NewTelegram("", "", time.Second), time.Second, slog.Default())
	summary, err := r.Run(ctx)
	if err != nil || !summary.Skipped {
		t.Fatalf("summary=%+v err=%v", summary, err)
	}
}
