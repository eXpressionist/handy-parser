package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eXpressionist/handy-parser/internal/extract"
	"github.com/eXpressionist/handy-parser/internal/model"
	"github.com/eXpressionist/handy-parser/internal/notify"
	"github.com/eXpressionist/handy-parser/internal/runner"
	"github.com/eXpressionist/handy-parser/internal/store"
)

type session struct {
	csrf    string
	expires time.Time
}
type loginAttempt struct {
	count int
	reset time.Time
}
type contextKey string

const sessionKey contextKey = "session"

type Server struct {
	store     *store.Store
	extractor *extract.Extractor
	runner    *runner.Runner
	telegram  *notify.Telegram
	password  string
	schedule  string
	logger    *slog.Logger
	template  *template.Template
	mu        sync.Mutex
	sessions  map[string]session
	attempts  map[string]loginAttempt
}

func New(s *store.Store, e *extract.Extractor, r *runner.Runner, t *notify.Telegram, password, timezone, schedule string, logger *slog.Logger) (*Server, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("load display timezone %q: %w", timezone, err)
	}
	timefmt := func(t *time.Time) string {
		if t == nil {
			return "—"
		}
		return t.In(location).Format("02.01.2006 15:04")
	}
	timevalue := func(t time.Time) string { return t.In(location).Format("02.01.2006 15:04") }
	tmpl, err := template.New("page").Funcs(template.FuncMap{"timefmt": timefmt, "timevalue": timevalue, "selected": func(a, b string) bool { return a == b }, "threshold": func(v *int64) string {
		if v == nil {
			return ""
		}
		return fmt.Sprintf("%d.%02d", *v/100, *v%100)
	}, "checked": func(v bool) string {
		if v {
			return "checked"
		}
		return ""
	}}).Parse(pageTemplate)
	if err != nil {
		return nil, err
	}
	return &Server{store: s, extractor: e, runner: r, telegram: t, password: password, schedule: schedule, logger: logger, template: tmpl, sessions: map[string]session{}, attempts: map[string]loginAttempt{}}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /login", s.loginPage)
	mux.HandleFunc("POST /login", s.login)
	mux.Handle("POST /logout", s.auth(http.HandlerFunc(s.logout)))
	mux.Handle("GET /", s.auth(http.HandlerFunc(s.dashboard)))
	mux.Handle("POST /preview", s.auth(http.HandlerFunc(s.preview)))
	mux.Handle("POST /watches", s.auth(http.HandlerFunc(s.createWatch)))
	mux.Handle("GET /watches/{id}/edit", s.auth(http.HandlerFunc(s.editPage)))
	mux.Handle("POST /watches/{id}/preview", s.auth(http.HandlerFunc(s.preview)))
	mux.Handle("POST /watches/{id}", s.auth(http.HandlerFunc(s.updateWatch)))
	mux.Handle("POST /watches/{id}/toggle", s.auth(http.HandlerFunc(s.toggleWatch)))
	mux.Handle("POST /watches/{id}/delete", s.auth(http.HandlerFunc(s.deleteWatch)))
	mux.Handle("POST /check", s.auth(http.HandlerFunc(s.runCheck)))
	mux.Handle("POST /telegram/test", s.auth(http.HandlerFunc(s.testTelegram)))
	return securityHeaders(mux)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	if s.password == "" {
		http.Error(w, "HANDY_ADMIN_PASSWORD or HANDY_ADMIN_PASSWORD_FILE is required", http.StatusServiceUnavailable)
		return
	}
	s.render(w, pageData{Login: true, Message: r.URL.Query().Get("msg")})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	client := clientAddress(r)
	if !s.loginAllowed(client) {
		http.Error(w, "Слишком много попыток; повторите через 10 минут", http.StatusTooManyRequests)
		return
	}
	provided := []byte(r.FormValue("password"))
	expected := []byte(s.password)
	if len(provided) != len(expected) || subtle.ConstantTimeCompare(provided, expected) != 1 {
		s.recordLoginFailure(client)
		time.Sleep(250 * time.Millisecond)
		http.Redirect(w, r, "/login?msg="+url.QueryEscape("Неверный пароль"), http.StatusSeeOther)
		return
	}
	s.mu.Lock()
	delete(s.attempts, client)
	s.mu.Unlock()
	token, csrf := randomToken(), randomToken()
	s.mu.Lock()
	s.sessions[token] = session{csrf: csrf, expires: time.Now().Add(24 * time.Hour)}
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "handy_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 86400, Secure: isHTTPS(r)})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) loginAllowed(client string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.attempts[client]
	if !ok || time.Now().After(a.reset) {
		delete(s.attempts, client)
		return true
	}
	return a.count < 5
}
func (s *Server) recordLoginFailure(client string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.attempts[client]
	if time.Now().After(a.reset) {
		a = loginAttempt{reset: time.Now().Add(10 * time.Minute)}
	}
	a.count++
	s.attempts[client] = a
}
func clientAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if !s.validCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}
	if c, err := r.Cookie("handy_session"); err == nil {
		s.mu.Lock()
		delete(s.sessions, c.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: "handy_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("handy_session")
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		s.mu.Lock()
		sess, ok := s.sessions[c.Value]
		if ok && time.Now().After(sess.expires) {
			delete(s.sessions, c.Value)
			ok = false
		}
		s.mu.Unlock()
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if r.Method == http.MethodPost && (sess.csrf == "" || r.FormValue("csrf") != sess.csrf) {
			http.Error(w, "invalid CSRF token", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionKey, sess)))
	})
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	watches, err := s.store.ListWatches(r.Context())
	if err != nil {
		http.Error(w, "database error", 500)
		return
	}
	changes, err := s.store.RecentChanges(r.Context(), 20)
	if err != nil {
		http.Error(w, "database error", 500)
		return
	}
	var lastRun *model.RunRecord
	if run, runErr := s.store.LatestRun(r.Context()); runErr == nil {
		lastRun = &run
	} else if runErr != sql.ErrNoRows {
		s.logger.Warn("load latest run", "error", runErr)
	}
	pending, err := s.store.PendingOutboxCount(r.Context())
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	failed, err := s.store.FailedOutboxCount(r.Context())
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	sess, _ := r.Context().Value(sessionKey).(session)
	s.render(w, pageData{CSRF: sess.csrf, Watches: watches, Changes: changes, LastRun: lastRun, PendingOutbox: pending, FailedOutbox: failed, Schedule: s.schedule, Message: r.URL.Query().Get("msg"), TelegramConfigured: s.telegram.Configured()})
}

func (s *Server) preview(w http.ResponseWriter, r *http.Request) {
	var id int64
	if rawID := r.PathValue("id"); rawID != "" {
		var err error
		id, err = strconv.ParseInt(rawID, 10, 64)
		if err != nil || id <= 0 {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
	}
	watch, obs, cfg, err := s.formWatch(r)
	watch.ID = id
	if err != nil {
		s.previewResult(w, r, watch, model.Observation{}, "Ошибка: "+err.Error())
		return
	}
	if cfg != "" {
		watch.AdapterConfig = cfg
	}
	s.previewResult(w, r, watch, obs, "Найдено: "+obs.Display)
}

func (s *Server) createWatch(w http.ResponseWriter, r *http.Request) {
	watch, _, cfg, err := s.formWatch(r)
	if err != nil {
		s.redirect(w, r, "Ошибка: "+err.Error())
		return
	}
	watch.AdapterConfig = cfg
	watch.Enabled = true
	if _, err = s.store.CreateWatch(r.Context(), watch); err != nil {
		s.redirect(w, r, "Не удалось сохранить: "+err.Error())
		return
	}
	s.redirect(w, r, "Наблюдение добавлено. Первая проверка создаст исходное значение без алерта.")
}

func (s *Server) editPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	watch, err := s.store.GetWatch(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	sess, _ := r.Context().Value(sessionKey).(session)
	s.render(w, pageData{CSRF: sess.csrf, Edit: true, Form: watch})
}

func (s *Server) updateWatch(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	watch, _, cfg, err := s.formWatch(r)
	if err != nil {
		s.redirect(w, r, "Ошибка: "+err.Error())
		return
	}
	watch.AdapterConfig = cfg
	if err = s.store.UpdateWatch(r.Context(), id, watch); err != nil {
		s.redirect(w, r, "Не удалось обновить: "+err.Error())
		return
	}
	s.redirect(w, r, "Наблюдение обновлено; следующая проверка создаст новую исходную точку")
}

func (s *Server) formWatch(r *http.Request) (model.Watch, model.Observation, string, error) {
	if err := r.ParseForm(); err != nil {
		return model.Watch{}, model.Observation{}, "", err
	}
	w := model.Watch{Name: strings.TrimSpace(r.FormValue("name")), URL: strings.TrimSpace(r.FormValue("url")), Kind: r.FormValue("kind"), Selector: strings.TrimSpace(r.FormValue("selector")), Attribute: strings.TrimSpace(r.FormValue("attribute")), ValueType: r.FormValue("value_type"), Currency: strings.ToUpper(strings.TrimSpace(r.FormValue("currency"))), Rule: r.FormValue("rule"), Enabled: true}
	if w.Name == "" || w.URL == "" {
		return w, model.Observation{}, "", fmt.Errorf("название и URL обязательны")
	}
	if w.Rule == "" {
		w.Rule = model.RuleAnyChange
	}
	if w.Kind == model.KindPSP {
		cfg, obs, err := s.extractor.ResolvePSP(r.Context(), w.URL)
		if err != nil {
			return w, obs, "", err
		}
		b, _ := json.Marshal(cfg)
		w.ValueType = model.ValuePrice
		w.Currency = "GEL"
		if w.Rule == model.RuleBelow {
			if err := setThreshold(&w, r.FormValue("threshold")); err != nil {
				return w, obs, "", err
			}
		}
		return w, obs, string(b), nil
	}
	if w.Kind != model.KindHTML {
		return w, model.Observation{}, "", fmt.Errorf("неизвестный метод")
	}
	w = extract.ApplyKnownProfile(w)
	if w.Selector == "" {
		return w, model.Observation{}, "", fmt.Errorf("CSS-селектор обязателен")
	}
	if w.ValueType != model.ValueText {
		w.ValueType = model.ValuePrice
	}
	if w.Rule == model.RuleBelow {
		if err := setThreshold(&w, r.FormValue("threshold")); err != nil {
			return w, model.Observation{}, "", err
		}
	}
	obs, err := s.extractor.Observe(r.Context(), w)
	return w, obs, "", err
}

func setThreshold(w *model.Watch, raw string) error {
	threshold, err := extract.Normalize(raw, model.ValuePrice, w.Currency)
	if err != nil {
		return fmt.Errorf("порог: %w", err)
	}
	v, _ := strconv.ParseInt(threshold.Normalized, 10, 64)
	w.ThresholdMinor = &v
	return nil
}

func (s *Server) previewResult(w http.ResponseWriter, r *http.Request, watch model.Watch, obs model.Observation, message string) {
	sess, _ := r.Context().Value(sessionKey).(session)
	s.render(w, pageData{CSRF: sess.csrf, Preview: true, Edit: watch.ID > 0, Form: watch, Observation: obs, Message: message})
}

func (s *Server) toggleWatch(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", 400)
		return
	}
	watch, err := s.store.GetWatch(r.Context(), id)
	if err == nil {
		err = s.store.SetEnabled(r.Context(), id, !watch.Enabled)
	}
	if err != nil {
		s.redirect(w, r, "Не удалось изменить наблюдение")
		return
	}
	s.redirect(w, r, "Статус изменён")
}
func (s *Server) deleteWatch(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err == nil {
		err = s.store.DeleteWatch(r.Context(), id)
	}
	if err != nil {
		s.redirect(w, r, "Не удалось удалить наблюдение")
		return
	}
	s.redirect(w, r, "Наблюдение удалено")
}
func (s *Server) runCheck(w http.ResponseWriter, r *http.Request) {
	go func() {
		summary, err := s.runner.Run(context.Background())
		if err != nil {
			s.logger.Error("manual check", "error", err)
		} else {
			s.logger.Info("manual check finished", "checked", summary.Checked, "changed", summary.Changed, "errors", summary.Errors, "skipped", summary.Skipped)
		}
	}()
	s.redirect(w, r, "Проверка запущена")
}
func (s *Server) testTelegram(w http.ResponseWriter, r *http.Request) {
	retry, err := s.telegram.Send(r.Context(), "✅ Handy Parser: тестовое уведомление")
	_ = retry
	if err != nil {
		s.redirect(w, r, "Telegram: "+err.Error())
		return
	}
	s.redirect(w, r, "Тестовое сообщение отправлено")
}

func (s *Server) validCSRF(r *http.Request) bool {
	sess, _ := r.Context().Value(sessionKey).(session)
	return sess.csrf != "" && r.FormValue("csrf") == sess.csrf
}
func (s *Server) redirect(w http.ResponseWriter, r *http.Request, message string) {
	http.Redirect(w, r, "/?msg="+url.QueryEscape(message), http.StatusSeeOther)
}
func (s *Server) render(w http.ResponseWriter, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.template.ExecuteTemplate(w, "page", data); err != nil {
		s.logger.Error("render page", "error", err)
	}
}

func randomToken() string { b := make([]byte, 24); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

type pageData struct {
	Login, Preview, Edit, TelegramConfigured bool
	CSRF, Message                            string
	Schedule                                 string
	Watches                                  []model.Watch
	Changes                                  []model.Change
	Form                                     model.Watch
	Observation                              model.Observation
	LastRun                                  *model.RunRecord
	PendingOutbox                            int
	FailedOutbox                             int
}
