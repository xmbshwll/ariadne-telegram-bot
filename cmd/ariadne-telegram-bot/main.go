package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/xmbshwll/ariadne"
	"github.com/xmbshwll/ariadne-telegram-bot/internal/albumbot"
	"github.com/xmbshwll/ariadne-telegram-bot/internal/config"
)

const (
	envPort            = "PORT"
	healthPath         = "/healthz"
	livenessPath       = "/livez"
	healthOKBody       = "ok\n"
	healthStartingBody = "starting\n"
)

var allowedUpdates = tgbot.AllowedUpdates{
	models.AllowedUpdateMessage,
	models.AllowedUpdateChannelPost,
}

type healthState struct {
	ready atomic.Bool
}

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	bootstrapLogger := newLogger(slog.LevelInfo)

	cfg, err := config.Load()
	if err != nil {
		bootstrapLogger.Error("load config failed", "error", err)
		return err
	}
	defer func() {
		if err := cfg.Cleanup(); err != nil {
			bootstrapLogger.Warn("config cleanup failed", "error", err)
		}
	}()

	logger := newLogger(cfg.LogLevel)
	logger.Debug("debug logging enabled")

	health := &healthState{}
	var healthErrCh <-chan error
	if addr, ok := healthListenAddr(); ok {
		healthErrCh, err = startHealthServer(ctx, logger, addr, health, stop)
		if err != nil {
			logger.Error("start health server failed", "error", err)
			return err
		}
	} else {
		logger.Info("health server disabled", "reason", envPort+" not set")
	}

	resolver := ariadne.New(ariadne.LoadConfig())
	service := albumbot.New(resolver, logger)

	opts := []tgbot.Option{
		tgbot.WithDefaultHandler(service.HandleDefault),
		tgbot.WithAllowedUpdates(allowedUpdates),
		tgbot.WithErrorsHandler(func(err error) {
			logger.Error("telegram bot error", "error", err)
		}),
		tgbot.WithWorkers(4),
	}
	if debugEnabled(cfg.LogLevel) {
		opts = append(opts,
			tgbot.WithDebug(),
			tgbot.WithDebugHandler(func(format string, args ...any) {
				logger.Debug(fmt.Sprintf(format, args...))
			}),
		)
	}

	b, err := tgbot.New(cfg.TelegramBotToken, opts...)
	if err != nil {
		logger.Error("create telegram bot failed", "error", err)
		return err
	}

	b.RegisterHandler(tgbot.HandlerTypeMessageText, "start", tgbot.MatchTypeCommandStartOnly, service.HandleStart)
	b.RegisterHandler(tgbot.HandlerTypeMessageText, "help", tgbot.MatchTypeCommandStartOnly, service.HandleHelp)

	if _, err := b.DeleteWebhook(ctx, nil); err != nil {
		logger.Error("clear webhook failed", "error", fmt.Errorf("delete webhook: %w", err))
		return err
	}

	health.ready.Store(true)

	if infoEnabled(cfg.LogLevel) {
		log.Printf(
			"INFO connected to telegram bot_id=%d delivery=polling log_level=%s",
			b.ID(),
			cfg.LogLevel.String(),
		)
	}
	b.Start(ctx)

	select {
	case err := <-healthErrCh:
		logger.Error("health server failed", "error", err)
		return err
	default:
	}

	return nil
}

func startHealthServer(
	ctx context.Context,
	logger *slog.Logger,
	addr string,
	health *healthState,
	stop context.CancelFunc,
) (<-chan error, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen health server on %s: %w", addr, err)
	}

	server := &http.Server{
		Handler:           newHealthHandler(health),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	errCh := make(chan error, 1)

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Warn("shutdown health server failed", "error", err)
		}
	}()

	go func() {
		logger.Info(
			"health server listening",
			"addr", listener.Addr().String(),
			"health_path", healthPath,
			"liveness_path", livenessPath,
		)

		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case errCh <- fmt.Errorf("serve health server: %w", err):
			default:
			}
			stop()
		}
	}()

	return errCh, nil
}

func healthListenAddr() (string, bool) {
	port := strings.TrimSpace(os.Getenv(envPort))
	if port == "" {
		return "", false
	}
	return net.JoinHostPort("", port), true
}

func newHealthHandler(health *healthState) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead:
		default:
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		switch r.URL.Path {
		case "/", livenessPath:
			writeHealthResponse(w, r, http.StatusOK, healthOKBody)
		case healthPath:
			if health != nil && health.ready.Load() {
				writeHealthResponse(w, r, http.StatusOK, healthOKBody)
				return
			}
			writeHealthResponse(w, r, http.StatusServiceUnavailable, healthStartingBody)
		default:
			http.NotFound(w, r)
		}
	})
}

func writeHealthResponse(w http.ResponseWriter, r *http.Request, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write([]byte(body))
}

func newLogger(level slog.Level) *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

func debugEnabled(level slog.Level) bool {
	return level <= slog.LevelDebug
}

func infoEnabled(level slog.Level) bool {
	return level <= slog.LevelInfo
}
