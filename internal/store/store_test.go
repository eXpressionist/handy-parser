package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/eXpressionist/handy-parser/internal/model"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestRecordSuccessBaselineAndChangeAreAtomic(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	id, err := s.CreateWatch(ctx, model.Watch{Name: "Price", URL: "https://example.com", Kind: model.KindHTML, Selector: ".price", ValueType: model.ValuePrice, Currency: "GEL", Rule: model.RuleAnyChange, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.GetWatch(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	changed, stale, err := s.RecordSuccess(ctx, w, model.Observation{Normalized: "1000", Display: "10.00 GEL", Currency: "GEL"}, nil)
	if err != nil || changed || stale {
		t.Fatalf("baseline: changed=%v stale=%v err=%v", changed, stale, err)
	}
	w, _ = s.GetWatch(ctx, id)
	changed, stale, err = s.RecordSuccess(ctx, w, model.Observation{Normalized: "900", Display: "9.00 GEL", Currency: "GEL"}, []string{"price changed"})
	if err != nil || !changed || stale {
		t.Fatalf("change: changed=%v stale=%v err=%v", changed, stale, err)
	}
	changes, err := s.RecentChanges(ctx, 10)
	if err != nil || len(changes) != 1 {
		t.Fatalf("changes=%d err=%v", len(changes), err)
	}
	outbox, err := s.PendingOutbox(ctx, 10)
	if err != nil || len(outbox) != 1 || outbox[0].Message != "price changed" {
		t.Fatalf("outbox=%v err=%v", outbox, err)
	}
	if err = s.MarkOutboxFailed(ctx, outbox[0].ID, "permanent delivery error"); err != nil {
		t.Fatal(err)
	}
	pending, _ := s.PendingOutboxCount(ctx)
	failed, _ := s.FailedOutboxCount(ctx)
	if pending != 0 || failed != 1 {
		t.Fatalf("pending=%d failed=%d", pending, failed)
	}
}

func TestRecordSuccessRefreshesAdapterConfig(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	id, err := s.CreateWatch(ctx, model.Watch{Name: "PSP", URL: "https://psp.ge/product", Kind: model.KindPSP, ValueType: model.ValuePrice, Currency: "GEL", AdapterConfig: `{"product_id":1}`, Rule: model.RuleAnyChange, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	w, _ := s.GetWatch(ctx, id)
	_, _, err = s.RecordSuccess(ctx, w, model.Observation{Normalized: "100", Display: "1.00 GEL", Currency: "GEL", Metadata: map[string]string{"adapter_config": `{"product_id":2}`}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := s.GetWatch(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if updated.AdapterConfig != `{"product_id":2}` {
		t.Fatalf("adapter config=%s", updated.AdapterConfig)
	}
}

func TestEditedWatchRejectsStaleResult(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	id, err := s.CreateWatch(ctx, model.Watch{Name: "Text", URL: "https://example.com", Kind: model.KindHTML, Selector: "h1", ValueType: model.ValueText, Rule: model.RuleAnyChange, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	w, _ := s.GetWatch(ctx, id)
	if err = s.SetEnabled(ctx, id, false); err != nil {
		t.Fatal(err)
	}
	_, stale, err := s.RecordSuccess(ctx, w, model.Observation{Normalized: "old", Display: "old"}, nil)
	if err != nil || !stale {
		t.Fatalf("stale=%v err=%v", stale, err)
	}
}

func TestUpdateWatchResetsBaselineAndIncrementsRevision(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	id, err := s.CreateWatch(ctx, model.Watch{Name: "Old", URL: "https://example.com/a", Kind: model.KindHTML, Selector: "h1", ValueType: model.ValueText, Rule: model.RuleAnyChange, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	w, _ := s.GetWatch(ctx, id)
	if _, _, err = s.RecordSuccess(ctx, w, model.Observation{Normalized: "a", Display: "a"}, nil); err != nil {
		t.Fatal(err)
	}
	if err = s.UpdateWatch(ctx, id, model.Watch{Name: "New", URL: "https://example.com/b", Kind: model.KindHTML, Selector: "h2", ValueType: model.ValueText, Rule: model.RuleAnyChange}); err != nil {
		t.Fatal(err)
	}
	updated, err := s.GetWatch(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "New" || updated.LastValue != "" || updated.Revision != w.Revision+1 {
		t.Fatalf("unexpected update: %+v", updated)
	}
}

func TestRunLeasePreventsOverlapAndExpires(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ok, err := s.AcquireRun(ctx, "one", 30*time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("first lock: %v %v", ok, err)
	}
	ok, err = s.AcquireRun(ctx, "two", time.Minute)
	if err != nil || ok {
		t.Fatalf("overlap: %v %v", ok, err)
	}
	time.Sleep(40 * time.Millisecond)
	ok, err = s.AcquireRun(ctx, "two", time.Minute)
	if err != nil || !ok {
		t.Fatalf("expired: %v %v", ok, err)
	}
}
