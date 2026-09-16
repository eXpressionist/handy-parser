package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "modernc.org/sqlite"

	"github.com/eXpressionist/handy-parser/internal/model"
)

type Store struct{ db *sql.DB }

type OutboxItem struct {
	ID      int64
	Message string
	Tries   int
}

func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	s := &Store{db: db}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, pragma := range []string{"PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON", "PRAGMA busy_timeout=5000", "PRAGMA synchronous=NORMAL"} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			db.Close()
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) Close() error                   { return s.db.Close() }
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *Store) Migrate(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS watches (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 name TEXT NOT NULL,
 url TEXT NOT NULL,
 kind TEXT NOT NULL,
 selector TEXT NOT NULL DEFAULT '',
 attribute TEXT NOT NULL DEFAULT '',
 value_type TEXT NOT NULL,
 currency TEXT NOT NULL DEFAULT '',
 adapter_config TEXT NOT NULL DEFAULT '',
 rule TEXT NOT NULL DEFAULT 'any_change',
 threshold_minor INTEGER,
 enabled INTEGER NOT NULL DEFAULT 1,
 revision INTEGER NOT NULL DEFAULT 1,
 last_value TEXT NOT NULL DEFAULT '',
 last_display TEXT NOT NULL DEFAULT '',
 last_success_at TEXT,
 last_check_at TEXT,
 last_error TEXT NOT NULL DEFAULT '',
 consecutive_errors INTEGER NOT NULL DEFAULT 0,
 error_alerted INTEGER NOT NULL DEFAULT 0,
 created_at TEXT NOT NULL,
 updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS changes (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 watch_id INTEGER NOT NULL REFERENCES watches(id) ON DELETE CASCADE,
 old_value TEXT NOT NULL,
 new_value TEXT NOT NULL,
 old_display TEXT NOT NULL,
 new_display TEXT NOT NULL,
 observed_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_changes_watch_time ON changes(watch_id, observed_at DESC);
CREATE TABLE IF NOT EXISTS outbox (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 event_key TEXT NOT NULL UNIQUE,
 message TEXT NOT NULL,
 status TEXT NOT NULL DEFAULT 'pending',
 tries INTEGER NOT NULL DEFAULT 0,
 next_attempt_at TEXT NOT NULL,
 created_at TEXT NOT NULL,
 delivered_at TEXT,
 last_error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_outbox_pending ON outbox(status, next_attempt_at);
CREATE TABLE IF NOT EXISTS run_lock (
 name TEXT PRIMARY KEY,
 owner TEXT NOT NULL,
 expires_at TEXT NOT NULL
);`
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

func (s *Store) CreateWatch(ctx context.Context, w model.Watch) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	r, err := s.db.ExecContext(ctx, `INSERT INTO watches
(name,url,kind,selector,attribute,value_type,currency,adapter_config,rule,threshold_minor,enabled,created_at,updated_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, w.Name, w.URL, w.Kind, w.Selector, w.Attribute, w.ValueType, w.Currency, w.AdapterConfig, w.Rule, w.ThresholdMinor, boolInt(w.Enabled), now, now)
	if err != nil {
		return 0, err
	}
	return r.LastInsertId()
}

func (s *Store) ListWatches(ctx context.Context) ([]model.Watch, error) {
	rows, err := s.db.QueryContext(ctx, watchSelect+` ORDER BY enabled DESC, name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []model.Watch
	for rows.Next() {
		w, err := scanWatch(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, w)
	}
	return result, rows.Err()
}

func (s *Store) ListEnabled(ctx context.Context) ([]model.Watch, error) {
	rows, err := s.db.QueryContext(ctx, watchSelect+` WHERE enabled=1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []model.Watch
	for rows.Next() {
		w, err := scanWatch(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, w)
	}
	return result, rows.Err()
}

func (s *Store) GetWatch(ctx context.Context, id int64) (model.Watch, error) {
	return scanWatch(s.db.QueryRowContext(ctx, watchSelect+` WHERE id=?`, id))
}

func (s *Store) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	r, err := s.db.ExecContext(ctx, `UPDATE watches SET enabled=?, revision=revision+1, updated_at=? WHERE id=?`, boolInt(enabled), now(), id)
	if err != nil {
		return err
	}
	return requireRow(r)
}

func (s *Store) UpdateWatch(ctx context.Context, id int64, w model.Watch) error {
	r, err := s.db.ExecContext(ctx, `UPDATE watches SET name=?,url=?,kind=?,selector=?,attribute=?,value_type=?,currency=?,adapter_config=?,rule=?,threshold_minor=?,revision=revision+1,last_value='',last_display='',last_success_at=NULL,last_error='',consecutive_errors=0,error_alerted=0,updated_at=? WHERE id=?`, w.Name, w.URL, w.Kind, w.Selector, w.Attribute, w.ValueType, w.Currency, w.AdapterConfig, w.Rule, w.ThresholdMinor, now(), id)
	if err != nil {
		return err
	}
	return requireRow(r)
}

func (s *Store) DeleteWatch(ctx context.Context, id int64) error {
	r, err := s.db.ExecContext(ctx, `DELETE FROM watches WHERE id=?`, id)
	if err != nil {
		return err
	}
	return requireRow(r)
}

func (s *Store) RecordSuccess(ctx context.Context, expected model.Watch, obs model.Observation, notifications []string) (changed, stale bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, false, err
	}
	defer tx.Rollback()
	var revision int64
	var oldValue, oldDisplay string
	if err = tx.QueryRowContext(ctx, `SELECT revision,last_value,last_display FROM watches WHERE id=?`, expected.ID).Scan(&revision, &oldValue, &oldDisplay); err != nil {
		return false, false, err
	}
	if revision != expected.Revision {
		return false, true, nil
	}
	changed = oldValue != "" && oldValue != obs.Normalized
	timestamp := now()
	if _, err = tx.ExecContext(ctx, `UPDATE watches SET last_value=?,last_display=?,currency=?,last_success_at=?,last_check_at=?,last_error='',consecutive_errors=0,error_alerted=0,updated_at=? WHERE id=?`, obs.Normalized, obs.Display, obs.Currency, timestamp, timestamp, timestamp, expected.ID); err != nil {
		return false, false, err
	}
	if changed {
		r, e := tx.ExecContext(ctx, `INSERT INTO changes(watch_id,old_value,new_value,old_display,new_display,observed_at) VALUES(?,?,?,?,?,?)`, expected.ID, oldValue, obs.Normalized, oldDisplay, obs.Display, timestamp)
		if e != nil {
			return false, false, e
		}
		changeID, _ := r.LastInsertId()
		for i, notification := range notifications {
			if notification == "" {
				continue
			}
			key := "change:" + strconv.FormatInt(changeID, 10) + ":" + strconv.Itoa(i)
			if _, e = tx.ExecContext(ctx, `INSERT INTO outbox(event_key,message,next_attempt_at,created_at) VALUES(?,?,?,?)`, key, notification, timestamp, timestamp); e != nil {
				return false, false, e
			}
		}
	} else if expected.ErrorAlerted {
		for i, notification := range notifications {
			if notification == "" {
				continue
			}
			key := fmt.Sprintf("recovery:%d:%s:%d", expected.ID, timestamp, i)
			if _, err = tx.ExecContext(ctx, `INSERT INTO outbox(event_key,message,next_attempt_at,created_at) VALUES(?,?,?,?)`, key, notification, timestamp, timestamp); err != nil {
				return false, false, err
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return false, false, err
	}
	return changed, false, nil
}

func (s *Store) RecordFailure(ctx context.Context, expected model.Watch, message, notification string) (stale bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var revision int64
	var errorsCount, alerted int
	if err = tx.QueryRowContext(ctx, `SELECT revision,consecutive_errors,error_alerted FROM watches WHERE id=?`, expected.ID).Scan(&revision, &errorsCount, &alerted); err != nil {
		return false, err
	}
	if revision != expected.Revision {
		return true, nil
	}
	errorsCount++
	timestamp := now()
	newAlerted := alerted
	if errorsCount >= 3 && alerted == 0 && notification != "" {
		key := fmt.Sprintf("error:%d:%s", expected.ID, timestamp)
		if _, err = tx.ExecContext(ctx, `INSERT INTO outbox(event_key,message,next_attempt_at,created_at) VALUES(?,?,?,?)`, key, notification, timestamp, timestamp); err != nil {
			return false, err
		}
		newAlerted = 1
	}
	if _, err = tx.ExecContext(ctx, `UPDATE watches SET last_check_at=?,last_error=?,consecutive_errors=?,error_alerted=?,updated_at=? WHERE id=?`, timestamp, message, errorsCount, newAlerted, timestamp, expected.ID); err != nil {
		return false, err
	}
	return false, tx.Commit()
}

func (s *Store) RecentChanges(ctx context.Context, limit int) ([]model.Change, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT c.id,c.watch_id,w.name,c.old_display,c.new_display,c.observed_at FROM changes c JOIN watches w ON w.id=c.watch_id ORDER BY c.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Change
	for rows.Next() {
		var c model.Change
		var ts string
		if err := rows.Scan(&c.ID, &c.WatchID, &c.WatchName, &c.OldDisplay, &c.NewDisplay, &ts); err != nil {
			return nil, err
		}
		c.ObservedAt, _ = time.Parse(time.RFC3339Nano, ts)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) AcquireRun(ctx context.Context, owner string, ttl time.Duration) (bool, error) {
	n := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM run_lock WHERE name='check' AND expires_at < ?`, n.Format(time.RFC3339Nano)); err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO run_lock(name,owner,expires_at) VALUES('check',?,?)`, owner, n.Add(ttl).Format(time.RFC3339Nano))
	if err != nil {
		if isConstraint(err) {
			return false, nil
		}
		return false, err
	}
	return true, tx.Commit()
}

func (s *Store) ReleaseRun(ctx context.Context, owner string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM run_lock WHERE name='check' AND owner=?`, owner)
	return err
}

func (s *Store) PendingOutbox(ctx context.Context, limit int) ([]OutboxItem, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,message,tries FROM outbox WHERE status='pending' AND next_attempt_at<=? ORDER BY id LIMIT ?`, now(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []OutboxItem
	for rows.Next() {
		var i OutboxItem
		if err := rows.Scan(&i.ID, &i.Message, &i.Tries); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

func (s *Store) MarkDelivered(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE outbox SET status='delivered',delivered_at=?,last_error='' WHERE id=?`, now(), id)
	return err
}

func (s *Store) MarkOutboxFailure(ctx context.Context, id int64, message string, retry time.Duration) error {
	_, err := s.db.ExecContext(ctx, `UPDATE outbox SET tries=tries+1,last_error=?,next_attempt_at=? WHERE id=?`, message, time.Now().UTC().Add(retry).Format(time.RFC3339Nano), id)
	return err
}

const watchSelect = `SELECT id,name,url,kind,selector,attribute,value_type,currency,adapter_config,rule,threshold_minor,enabled,revision,last_value,last_display,last_success_at,last_check_at,last_error,consecutive_errors,error_alerted,created_at,updated_at FROM watches`

type scanner interface{ Scan(...any) error }

func scanWatch(row scanner) (model.Watch, error) {
	var w model.Watch
	var enabled int
	var threshold sql.NullInt64
	var success, check sql.NullString
	var created, updated string
	var errorAlerted int
	err := row.Scan(&w.ID, &w.Name, &w.URL, &w.Kind, &w.Selector, &w.Attribute, &w.ValueType, &w.Currency, &w.AdapterConfig, &w.Rule, &threshold, &enabled, &w.Revision, &w.LastValue, &w.LastDisplay, &success, &check, &w.LastError, &w.ConsecutiveErrors, &errorAlerted, &created, &updated)
	if err != nil {
		return w, err
	}
	w.Enabled = enabled != 0
	w.ErrorAlerted = errorAlerted != 0
	if threshold.Valid {
		v := threshold.Int64
		w.ThresholdMinor = &v
	}
	w.LastSuccessAt = parseNullTime(success)
	w.LastCheckAt = parseNullTime(check)
	w.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	w.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return w, nil
}

func parseNullTime(v sql.NullString) *time.Time {
	if !v.Valid {
		return nil
	}
	t, e := time.Parse(time.RFC3339Nano, v.String)
	if e != nil {
		return nil
	}
	return &t
}
func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func requireRow(r sql.Result) error {
	n, e := r.RowsAffected()
	if e != nil {
		return e
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
func isConstraint(err error) bool {
	return err != nil && (errors.Is(err, sql.ErrNoRows) || contains(err.Error(), "constraint"))
}
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
