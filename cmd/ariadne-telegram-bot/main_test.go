package main

import (
	"log/slog"
	"testing"
)

func TestAllowedUpdates(t *testing.T) {
	if len(allowedUpdates) != 2 {
		t.Fatalf("len(allowedUpdates) = %d, want 2", len(allowedUpdates))
	}
	if allowedUpdates[0] != "message" {
		t.Fatalf("allowedUpdates[0] = %q, want message", allowedUpdates[0])
	}
	if allowedUpdates[1] != "channel_post" {
		t.Fatalf("allowedUpdates[1] = %q, want channel_post", allowedUpdates[1])
	}
}

func TestDebugEnabled(t *testing.T) {
	if !debugEnabled(slog.LevelDebug) {
		t.Fatal("debugEnabled(debug) = false, want true")
	}
	if debugEnabled(slog.LevelInfo) {
		t.Fatal("debugEnabled(info) = true, want false")
	}
}

func TestInfoEnabled(t *testing.T) {
	if !infoEnabled(slog.LevelDebug) {
		t.Fatal("infoEnabled(debug) = false, want true")
	}
	if !infoEnabled(slog.LevelInfo) {
		t.Fatal("infoEnabled(info) = false, want true")
	}
	if infoEnabled(slog.LevelWarn) {
		t.Fatal("infoEnabled(warn) = true, want false")
	}
}
