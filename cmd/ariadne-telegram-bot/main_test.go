package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
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

func TestHealthListenAddr(t *testing.T) {
	tests := []struct {
		name        string
		port        string
		wantAddr    string
		wantEnabled bool
	}{
		{name: "disabled by default", port: "", wantAddr: "", wantEnabled: false},
		{name: "disabled whitespace", port: "  \t", wantAddr: "", wantEnabled: false},
		{name: "enabled custom", port: "9090", wantAddr: ":9090", wantEnabled: true},
		{name: "enabled trims whitespace", port: "  7070\t", wantAddr: ":7070", wantEnabled: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envPort, tt.port)

			gotAddr, gotEnabled := healthListenAddr()
			if gotAddr != tt.wantAddr {
				t.Fatalf("healthListenAddr() addr = %q, want %q", gotAddr, tt.wantAddr)
			}
			if gotEnabled != tt.wantEnabled {
				t.Fatalf("healthListenAddr() enabled = %t, want %t", gotEnabled, tt.wantEnabled)
			}
		})
	}
}

func TestHealthHandler(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		method     string
		ready      bool
		wantStatus int
		wantBody   string
	}{
		{name: "root ok", path: "/", method: http.MethodGet, wantStatus: http.StatusOK, wantBody: healthOKBody},
		{name: "liveness ok", path: livenessPath, method: http.MethodGet, wantStatus: http.StatusOK, wantBody: healthOKBody},
		{name: "health starting", path: healthPath, method: http.MethodGet, wantStatus: http.StatusServiceUnavailable, wantBody: healthStartingBody},
		{name: "health ready", path: healthPath, method: http.MethodGet, ready: true, wantStatus: http.StatusOK, wantBody: healthOKBody},
		{name: "health head", path: healthPath, method: http.MethodHead, ready: true, wantStatus: http.StatusOK, wantBody: ""},
		{name: "method not allowed", path: healthPath, method: http.MethodPost, wantStatus: http.StatusMethodNotAllowed, wantBody: "Method Not Allowed\n"},
		{name: "not found", path: "/missing", method: http.MethodGet, wantStatus: http.StatusNotFound, wantBody: "404 page not found\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			health := &healthState{}
			if tt.ready {
				health.ready.Store(true)
			}

			req := httptest.NewRequest(tt.method, tt.path, nil)
			rr := httptest.NewRecorder()

			newHealthHandler(health).ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
			if rr.Body.String() != tt.wantBody {
				t.Fatalf("body = %q, want %q", rr.Body.String(), tt.wantBody)
			}
		})
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
