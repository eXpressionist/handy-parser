package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/eXpressionist/handy-parser/internal/config"
	"github.com/eXpressionist/handy-parser/internal/extract"
	"github.com/eXpressionist/handy-parser/internal/notify"
	"github.com/eXpressionist/handy-parser/internal/runner"
	"github.com/eXpressionist/handy-parser/internal/store"
	webapp "github.com/eXpressionist/handy-parser/internal/web"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("command failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if command == "healthcheck" {
		return healthcheck(cfg)
	}
	s, err := store.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err = s.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	if command == "migrate" {
		logger.Info("database migrations applied")
		return nil
	}
	client := extract.NewClient(cfg.HTTPTimeout, cfg.MaxResponseBytes, cfg.AllowPrivateTarget)
	extractor := extract.New(client)
	telegram := notify.NewTelegram(cfg.TelegramToken, cfg.TelegramChatID, cfg.HTTPTimeout)
	checkRunner := runner.New(s, extractor, telegram, cfg.CheckTimeout, logger)
	switch command {
	case "check":
		runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		summary, err := checkRunner.Run(runCtx)
		if err != nil {
			return err
		}
		logger.Info("check finished", "checked", summary.Checked, "changed", summary.Changed, "errors", summary.Errors, "skipped", summary.Skipped)
		if summary.Errors > 0 {
			return fmt.Errorf("check completed with %d error(s)", summary.Errors)
		}
		return nil
	case "serve":
		if cfg.AdminPassword == "" {
			return fmt.Errorf("admin password is required for serve")
		}
		webServer, err := webapp.New(s, extractor, checkRunner, telegram, cfg.AdminPassword, cfg.DisplayTimezone, logger)
		if err != nil {
			return err
		}
		httpServer := &http.Server{Addr: cfg.Listen, Handler: webServer.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
		serverErr := make(chan error, 1)
		go func() {
			logger.Info("web server listening", "address", cfg.Listen)
			serverErr <- httpServer.ListenAndServe()
		}()
		serveCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		select {
		case <-serveCtx.Done():
			shutdownCtx, c := context.WithTimeout(context.Background(), 10*time.Second)
			defer c()
			return httpServer.Shutdown(shutdownCtx)
		case err := <-serverErr:
			if err == http.ErrServerClosed {
				return nil
			}
			return err
		}
	default:
		return fmt.Errorf("unknown command %q (use serve, check, migrate, healthcheck)", command)
	}
}

func healthcheck(cfg config.Config) error {
	address := cfg.Listen
	if strings.HasPrefix(address, "0.0.0.0:") {
		address = "127.0.0.1:" + strings.TrimPrefix(address, "0.0.0.0:")
	}
	if strings.HasPrefix(address, "[") {
	} else if strings.HasPrefix(address, ":") {
		address = "127.0.0.1" + address
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+address+"/healthz", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health endpoint returned %d", resp.StatusCode)
	}
	return nil
}
