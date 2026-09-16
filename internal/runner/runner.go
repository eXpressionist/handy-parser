package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/eXpressionist/handy-parser/internal/model"
	"github.com/eXpressionist/handy-parser/internal/notify"
	"github.com/eXpressionist/handy-parser/internal/store"
)

type Runner struct {
	store     *store.Store
	extractor Observer
	telegram  *notify.Telegram
	timeout   time.Duration
	logger    *slog.Logger
}

type Observer interface {
	Observe(context.Context, model.Watch) (model.Observation, error)
}

func New(s *store.Store, e Observer, t *notify.Telegram, timeout time.Duration, logger *slog.Logger) *Runner {
	return &Runner{store: s, extractor: e, telegram: t, timeout: timeout, logger: logger}
}

func (r *Runner) Run(parent context.Context) (summary model.RunSummary, runErr error) {
	ctx, cancel := context.WithTimeout(parent, r.timeout)
	defer cancel()
	runID, err := r.store.StartRun(ctx)
	if err != nil {
		return summary, err
	}
	defer func() {
		finishCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		if err := r.store.FinishRun(finishCtx, runID, summary, runErr); err != nil {
			r.logger.Error("finish run record", "error", err)
		}
	}()
	owner := randomOwner()
	locked, err := r.store.AcquireRun(ctx, owner, r.timeout+time.Minute)
	if err != nil {
		return summary, err
	}
	if !locked {
		summary.Skipped = true
		return summary, nil
	}
	defer func() {
		releaseCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = r.store.ReleaseRun(releaseCtx, owner)
	}()
	watches, err := r.store.ListEnabled(ctx)
	if err != nil {
		return summary, err
	}
	for index, w := range watches {
		if index > 0 {
			select {
			case <-ctx.Done():
				return summary, ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}
		summary.Checked++
		obs, observeErr := r.extractor.Observe(ctx, w)
		if observeErr != nil {
			summary.Errors++
			message := ""
			if w.ConsecutiveErrors+1 >= 3 && !w.ErrorAlerted {
				message = fmt.Sprintf("⚠️ %s: три проверки подряд завершились ошибкой.\n%s\n%s", w.Name, shortError(observeErr), w.URL)
			}
			_, saveErr := r.store.RecordFailure(ctx, w, shortError(observeErr), message)
			if saveErr != nil {
				r.logger.Error("save check failure", "watch_id", w.ID, "error", saveErr)
			}
			r.logger.Warn("watch failed", "watch_id", w.ID, "name", w.Name, "error", observeErr)
			continue
		}
		var messages []string
		if w.ErrorAlerted {
			messages = append(messages, fmt.Sprintf("✅ %s снова доступен.\nЗначение: %s\n%s", w.Name, obs.Display, w.URL))
		}
		changedExpected := w.LastValue != "" && w.LastValue != obs.Normalized
		if changedExpected && shouldNotify(w, obs) {
			messages = append(messages, changeMessage(w, obs))
		}
		changed, stale, saveErr := r.store.RecordSuccess(ctx, w, obs, messages)
		if saveErr != nil {
			summary.Errors++
			r.logger.Error("save observation", "watch_id", w.ID, "error", saveErr)
			continue
		}
		if stale {
			r.logger.Info("discarded result for edited watch", "watch_id", w.ID)
			continue
		}
		if changed {
			summary.Changed++
		}
		r.logger.Info("watch checked", "watch_id", w.ID, "name", w.Name, "value", obs.Display, "changed", changed)
	}
	if err := r.deliver(ctx); err != nil {
		summary.Errors++
		r.logger.Warn("Telegram outbox not fully delivered", "error", err)
	}
	if err := r.store.Cleanup(ctx); err != nil {
		r.logger.Warn("cleanup old records", "error", err)
	}
	return summary, nil
}

func (r *Runner) deliver(ctx context.Context) error {
	items, err := r.store.PendingOutbox(ctx, 50)
	if err != nil {
		return err
	}
	var lastErr error
	for _, item := range items {
		retry, sendErr := r.telegram.Send(ctx, item.Message)
		if sendErr == nil {
			if err = r.store.MarkDelivered(ctx, item.ID); err != nil {
				return err
			}
			continue
		}
		lastErr = sendErr
		if retry <= 0 {
			retry = time.Minute
		}
		if err = r.store.MarkOutboxFailure(ctx, item.ID, shortError(sendErr), retry); err != nil {
			return err
		}
		if ctx.Err() != nil {
			break
		}
	}
	return lastErr
}

func shouldNotify(w model.Watch, obs model.Observation) bool {
	if w.Rule == model.RuleAnyChange {
		return true
	}
	oldValue, oldErr := strconv.ParseInt(w.LastValue, 10, 64)
	newValue, newErr := strconv.ParseInt(obs.Normalized, 10, 64)
	if oldErr != nil || newErr != nil {
		return true
	}
	switch w.Rule {
	case model.RuleDecrease:
		return newValue < oldValue
	case model.RuleBelow:
		return w.ThresholdMinor != nil && oldValue >= *w.ThresholdMinor && newValue < *w.ThresholdMinor
	default:
		return true
	}
}

func changeMessage(w model.Watch, obs model.Observation) string {
	return fmt.Sprintf("🔔 %s\n%s → %s\n%s", w.Name, w.LastDisplay, obs.Display, w.URL)
}
func shortError(err error) string {
	s := err.Error()
	if len(s) > 500 {
		return s[:500]
	}
	return s
}
func randomOwner() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
