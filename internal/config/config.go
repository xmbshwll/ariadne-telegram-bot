package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const (
	envTelegramBotToken             = "TELEGRAM_BOT_TOKEN"
	envLogLevel                     = "LOG_LEVEL"
	envAppleMusicPrivateKeyPath     = "APPLE_MUSIC_PRIVATE_KEY_PATH"
	envAppleMusicPrivateKey         = "APPLE_MUSIC_PRIVATE_KEY"
	envAppleMusicPrivateKeyBase64   = "APPLE_MUSIC_PRIVATE_KEY_BASE64"
	appleMusicPrivateKeyFilePattern = "ariadne-apple-key-*"
	appleMusicPrivateKeyFileName    = "AuthKey.p8"
)

var (
	ErrMissingTelegramBotToken           = errors.New("TELEGRAM_BOT_TOKEN is required")
	ErrInvalidLogLevel                   = errors.New("LOG_LEVEL must be one of: debug, info, warn, error")
	ErrAppleMusicPrivateKeyConflict      = errors.New("set only one of APPLE_MUSIC_PRIVATE_KEY or APPLE_MUSIC_PRIVATE_KEY_BASE64")
	ErrInvalidAppleMusicPrivateKeyBase64 = errors.New("APPLE_MUSIC_PRIVATE_KEY_BASE64 is not valid base64")
)

type Config struct {
	TelegramBotToken string
	LogLevel         slog.Level
	cleanup          func() error
}

func Load() (Config, error) {
	cleanup, err := prepareAppleMusicPrivateKey()
	if err != nil {
		return Config{}, err
	}

	logLevel, err := loadLogLevel(os.Getenv(envLogLevel))
	if err != nil {
		if cleanup != nil {
			_ = cleanup()
		}
		return Config{}, err
	}

	cfg := Config{
		TelegramBotToken: strings.TrimSpace(os.Getenv(envTelegramBotToken)),
		LogLevel:         logLevel,
		cleanup:          cleanup,
	}
	if err := cfg.Validate(); err != nil {
		_ = cfg.Cleanup()
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.TelegramBotToken) == "" {
		return ErrMissingTelegramBotToken
	}
	return nil
}

func (c Config) Cleanup() error {
	if c.cleanup == nil {
		return nil
	}
	return c.cleanup()
}

func loadLogLevel(raw string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("%w: %q", ErrInvalidLogLevel, raw)
	}
}

func prepareAppleMusicPrivateKey() (func() error, error) {
	if strings.TrimSpace(os.Getenv(envAppleMusicPrivateKeyPath)) != "" {
		return nil, nil
	}

	raw := os.Getenv(envAppleMusicPrivateKey)
	encoded := strings.TrimSpace(os.Getenv(envAppleMusicPrivateKeyBase64))
	if strings.TrimSpace(raw) == "" && encoded == "" {
		return nil, nil
	}
	if strings.TrimSpace(raw) != "" && encoded != "" {
		return nil, ErrAppleMusicPrivateKeyConflict
	}

	keyData, err := resolveAppleMusicPrivateKeyData(raw, encoded)
	if err != nil {
		return nil, err
	}

	dir, err := createPrivateTempDir(appleMusicPrivateKeyFilePattern)
	if err != nil {
		return nil, err
	}

	path := filepath.Join(dir, appleMusicPrivateKeyFileName)
	if err := os.WriteFile(path, keyData, 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("write Apple Music private key file: %w", err)
	}
	if err := os.Setenv(envAppleMusicPrivateKeyPath, path); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("set APPLE_MUSIC_PRIVATE_KEY_PATH: %w", err)
	}

	_ = os.Unsetenv(envAppleMusicPrivateKey)
	_ = os.Unsetenv(envAppleMusicPrivateKeyBase64)

	return func() error {
		_ = os.Unsetenv(envAppleMusicPrivateKeyPath)
		if err := os.RemoveAll(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove Apple Music private key temp dir: %w", err)
		}
		return nil
	}, nil
}

func createPrivateTempDir(pattern string) (string, error) {
	dir, err := os.MkdirTemp("", pattern)
	if err == nil {
		return dir, nil
	}

	cwd, cwdErr := os.Getwd()
	if cwdErr == nil {
		dir, cwdErr = os.MkdirTemp(cwd, "."+pattern)
		if cwdErr == nil {
			return dir, nil
		}
	}

	if cwdErr != nil {
		return "", fmt.Errorf("create temp dir for Apple Music private key: primary error: %w; fallback error: %v", err, cwdErr)
	}
	return "", fmt.Errorf("create temp dir for Apple Music private key: %w", err)
}

func resolveAppleMusicPrivateKeyData(raw, encoded string) ([]byte, error) {
	if encoded != "" {
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInvalidAppleMusicPrivateKeyBase64, err)
		}
		return normalizeAppleMusicPrivateKey(string(decoded)), nil
	}
	return normalizeAppleMusicPrivateKey(raw), nil
}

func normalizeAppleMusicPrivateKey(value string) []byte {
	value = strings.TrimSpace(value)
	if strings.Contains(value, `\n`) && !strings.Contains(value, "\n") {
		value = strings.ReplaceAll(value, `\n`, "\n")
	}
	value = strings.ReplaceAll(value, "\r\n", "\n")
	if value != "" && !strings.HasSuffix(value, "\n") {
		value += "\n"
	}
	return []byte(value)
}
