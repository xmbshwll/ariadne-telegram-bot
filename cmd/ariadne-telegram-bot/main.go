package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/xmbshwll/ariadne"
	"github.com/xmbshwll/ariadne-telegram-bot/internal/albumbot"
	"github.com/xmbshwll/ariadne-telegram-bot/internal/config"
)

var allowedUpdates = tgbot.AllowedUpdates{
	models.AllowedUpdateMessage,
	models.AllowedUpdateChannelPost,
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	bootstrapLogger := newLogger(slog.LevelInfo)

	cfg, err := config.Load()
	if err != nil {
		bootstrapLogger.Error("load config failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := cfg.Cleanup(); err != nil {
			bootstrapLogger.Warn("config cleanup failed", "error", err)
		}
	}()

	logger := newLogger(cfg.LogLevel)
	logger.Debug("debug logging enabled")
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
		os.Exit(1)
	}

	b.RegisterHandler(tgbot.HandlerTypeMessageText, "start", tgbot.MatchTypeCommandStartOnly, service.HandleStart)
	b.RegisterHandler(tgbot.HandlerTypeMessageText, "help", tgbot.MatchTypeCommandStartOnly, service.HandleHelp)

	if _, err := b.DeleteWebhook(ctx, nil); err != nil {
		logger.Error("clear webhook failed", "error", fmt.Errorf("delete webhook: %w", err))
		os.Exit(1)
	}

	if infoEnabled(cfg.LogLevel) {
		log.Printf("INFO connected to telegram bot_id=%d delivery=polling log_level=%s", b.ID(), cfg.LogLevel.String())
	}
	b.Start(ctx)
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
