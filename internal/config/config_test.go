package config

import (
	"encoding/base64"
	"errors"
	"log/slog"
	"os"
	"testing"
)

func TestLoadLogLevel(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    slog.Level
		wantErr error
	}{
		{name: "default", raw: "", want: slog.LevelInfo},
		{name: "debug", raw: "debug", want: slog.LevelDebug},
		{name: "info", raw: "INFO", want: slog.LevelInfo},
		{name: "warn", raw: "warn", want: slog.LevelWarn},
		{name: "warning", raw: "warning", want: slog.LevelWarn},
		{name: "error", raw: "error", want: slog.LevelError},
		{name: "invalid", raw: "trace", wantErr: ErrInvalidLogLevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := loadLogLevel(tt.raw)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("loadLogLevel() error = %v, want %v", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Fatalf("loadLogLevel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr error
	}{
		{
			name:    "missing token",
			config:  Config{},
			wantErr: ErrMissingTelegramBotToken,
		},
		{
			name:    "whitespace token",
			config:  Config{TelegramBotToken: "   \n\t"},
			wantErr: ErrMissingTelegramBotToken,
		},
		{
			name:   "valid token",
			config: Config{TelegramBotToken: "token"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	t.Run("missing token", func(t *testing.T) {
		t.Setenv(envTelegramBotToken, "")

		_, err := Load()
		if !errors.Is(err, ErrMissingTelegramBotToken) {
			t.Fatalf("Load() error = %v, want %v", err, ErrMissingTelegramBotToken)
		}
	})

	t.Run("trim token", func(t *testing.T) {
		t.Setenv(envTelegramBotToken, "  token  ")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		defer func() {
			if err := cfg.Cleanup(); err != nil {
				t.Fatalf("Cleanup() error = %v", err)
			}
		}()

		if cfg.TelegramBotToken != "token" {
			t.Fatalf("TelegramBotToken = %q, want token", cfg.TelegramBotToken)
		}
		if cfg.LogLevel != slog.LevelInfo {
			t.Fatalf("LogLevel = %v, want %v", cfg.LogLevel, slog.LevelInfo)
		}
	})

	t.Run("debug log level", func(t *testing.T) {
		t.Setenv(envTelegramBotToken, "token")
		t.Setenv(envLogLevel, "debug")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		defer func() {
			if err := cfg.Cleanup(); err != nil {
				t.Fatalf("Cleanup() error = %v", err)
			}
		}()

		if cfg.LogLevel != slog.LevelDebug {
			t.Fatalf("LogLevel = %v, want %v", cfg.LogLevel, slog.LevelDebug)
		}
	})

	t.Run("invalid log level", func(t *testing.T) {
		t.Setenv(envTelegramBotToken, "token")
		t.Setenv(envLogLevel, "trace")

		_, err := Load()
		if !errors.Is(err, ErrInvalidLogLevel) {
			t.Fatalf("Load() error = %v, want %v", err, ErrInvalidLogLevel)
		}
	})

	t.Run("materialize raw Apple key", func(t *testing.T) {
		t.Setenv(envTelegramBotToken, "token")
		t.Setenv(envAppleMusicPrivateKey, `-----BEGIN PRIVATE KEY-----\nabc123\n-----END PRIVATE KEY-----`)
		t.Setenv(envAppleMusicPrivateKeyBase64, "")
		t.Setenv(envAppleMusicPrivateKeyPath, "")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		path := os.Getenv(envAppleMusicPrivateKeyPath)
		if path == "" {
			t.Fatal("APPLE_MUSIC_PRIVATE_KEY_PATH not set")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}
		want := "-----BEGIN PRIVATE KEY-----\nabc123\n-----END PRIVATE KEY-----\n"
		if string(data) != want {
			t.Fatalf("key file = %q, want %q", string(data), want)
		}
		if got := os.Getenv(envAppleMusicPrivateKey); got != "" {
			t.Fatalf("APPLE_MUSIC_PRIVATE_KEY = %q, want empty", got)
		}

		if err := cfg.Cleanup(); err != nil {
			t.Fatalf("Cleanup() error = %v", err)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("key file still exists after cleanup, stat error = %v", err)
		}
		if got := os.Getenv(envAppleMusicPrivateKeyPath); got != "" {
			t.Fatalf("APPLE_MUSIC_PRIVATE_KEY_PATH = %q after cleanup, want empty", got)
		}
	})

	t.Run("materialize base64 Apple key", func(t *testing.T) {
		t.Setenv(envTelegramBotToken, "token")
		t.Setenv(envAppleMusicPrivateKey, "")
		t.Setenv(envAppleMusicPrivateKeyPath, "")
		t.Setenv(envAppleMusicPrivateKeyBase64, base64.StdEncoding.EncodeToString([]byte("-----BEGIN PRIVATE KEY-----\nabc123\n-----END PRIVATE KEY-----\n")))

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		defer func() {
			if err := cfg.Cleanup(); err != nil {
				t.Fatalf("Cleanup() error = %v", err)
			}
		}()

		path := os.Getenv(envAppleMusicPrivateKeyPath)
		if path == "" {
			t.Fatal("APPLE_MUSIC_PRIVATE_KEY_PATH not set")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}
		want := "-----BEGIN PRIVATE KEY-----\nabc123\n-----END PRIVATE KEY-----\n"
		if string(data) != want {
			t.Fatalf("key file = %q, want %q", string(data), want)
		}
	})

	t.Run("existing path wins", func(t *testing.T) {
		t.Setenv(envTelegramBotToken, "token")
		t.Setenv(envAppleMusicPrivateKeyPath, "/run/secrets/apple-music.p8")
		t.Setenv(envAppleMusicPrivateKey, `-----BEGIN PRIVATE KEY-----\nabc123\n-----END PRIVATE KEY-----`)
		t.Setenv(envAppleMusicPrivateKeyBase64, "")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		defer func() {
			if err := cfg.Cleanup(); err != nil {
				t.Fatalf("Cleanup() error = %v", err)
			}
		}()

		if got := os.Getenv(envAppleMusicPrivateKeyPath); got != "/run/secrets/apple-music.p8" {
			t.Fatalf("APPLE_MUSIC_PRIVATE_KEY_PATH = %q, want existing path", got)
		}
		if got := os.Getenv(envAppleMusicPrivateKey); got == "" {
			t.Fatal("APPLE_MUSIC_PRIVATE_KEY unexpectedly cleared")
		}
	})

	t.Run("conflicting Apple key env vars", func(t *testing.T) {
		t.Setenv(envTelegramBotToken, "token")
		t.Setenv(envAppleMusicPrivateKeyPath, "")
		t.Setenv(envAppleMusicPrivateKey, "raw")
		t.Setenv(envAppleMusicPrivateKeyBase64, base64.StdEncoding.EncodeToString([]byte("raw")))

		_, err := Load()
		if !errors.Is(err, ErrAppleMusicPrivateKeyConflict) {
			t.Fatalf("Load() error = %v, want %v", err, ErrAppleMusicPrivateKeyConflict)
		}
	})

	t.Run("invalid base64 Apple key", func(t *testing.T) {
		t.Setenv(envTelegramBotToken, "token")
		t.Setenv(envAppleMusicPrivateKeyPath, "")
		t.Setenv(envAppleMusicPrivateKey, "")
		t.Setenv(envAppleMusicPrivateKeyBase64, "not-base64")

		_, err := Load()
		if !errors.Is(err, ErrInvalidAppleMusicPrivateKeyBase64) {
			t.Fatalf("Load() error = %v, want %v", err, ErrInvalidAppleMusicPrivateKeyBase64)
		}
	})
}
