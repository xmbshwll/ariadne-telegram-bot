package albumbot

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/xmbshwll/ariadne"
)

type stubResolver struct {
	resolutions map[string]*ariadne.Resolution
	errs        map[string]error
}

func (s stubResolver) ResolveAlbum(_ context.Context, inputURL string) (*ariadne.Resolution, error) {
	if err, ok := s.errs[inputURL]; ok {
		return nil, err
	}
	resolution, ok := s.resolutions[inputURL]
	if !ok {
		return nil, errors.New("unexpected url")
	}
	return resolution, nil
}

func TestBuildReplyNoURL(t *testing.T) {
	service := New(stubResolver{}, discardLogger())

	reply, ok := service.BuildReply(context.Background(), "hello world")
	if ok {
		t.Fatal("BuildReply() ok = true, want false")
	}
	if reply != "" {
		t.Fatalf("BuildReply() reply = %q, want empty", reply)
	}
}

func TestBuildReplyFormatsLinks(t *testing.T) {
	const url = "https://open.spotify.com/album/example"

	service := New(stubResolver{
		resolutions: map[string]*ariadne.Resolution{
			url: {
				InputURL: url,
				Parsed: ariadne.ParsedAlbumURL{
					Service:      ariadne.ServiceSpotify,
					CanonicalURL: url,
				},
				Source: ariadne.CanonicalAlbum{
					Title:     "Best & Loud",
					Artists:   []string{"Artist <One>"},
					SourceURL: url,
				},
				Matches: map[ariadne.ServiceName]ariadne.MatchResult{
					ariadne.ServiceBandcamp: {
						Best: &ariadne.ScoredMatch{URL: "https://artist.bandcamp.com/album/best-and-loud"},
					},
					ariadne.ServiceYouTubeMusic: {
						Best: &ariadne.ScoredMatch{URL: "https://music.youtube.com/playlist?list=PL123"},
					},
				},
			},
		},
	}, discardLogger())

	reply, ok := service.BuildReply(context.Background(), "check "+url)
	if !ok {
		t.Fatal("BuildReply() ok = false, want true")
	}

	want := strings.Join([]string{
		"<b>Artist &lt;One&gt; — Best &amp; Loud</b>",
		`<a href="https://artist.bandcamp.com/album/best-and-loud">Bandcamp</a> | <a href="https://open.spotify.com/album/example">Spotify</a> | <a href="https://music.youtube.com/playlist?list=PL123">YouTube Music</a>`,
	}, "\n")
	if reply != want {
		t.Fatalf("BuildReply() reply = %q, want %q", reply, want)
	}
}

func TestBuildReplyHandlesUnsupportedURL(t *testing.T) {
	const url = "https://example.com/not-supported"

	service := New(stubResolver{
		errs: map[string]error{url: ariadne.ErrUnsupportedURL},
	}, discardLogger())

	reply, ok := service.BuildReply(context.Background(), url)
	if !ok {
		t.Fatal("BuildReply() ok = false, want true")
	}

	want := "Could not resolve album link:\n<code>https://example.com/not-supported</code>"
	if reply != want {
		t.Fatalf("BuildReply() reply = %q, want %q", reply, want)
	}
}

func TestBuildReplyDeduplicatesURLsAndTrimsPunctuation(t *testing.T) {
	const url = "https://www.deezer.com/album/12047952"

	service := New(stubResolver{
		resolutions: map[string]*ariadne.Resolution{
			url: {
				InputURL: url,
				Parsed:   ariadne.ParsedAlbumURL{Service: ariadne.ServiceDeezer, CanonicalURL: url},
				Source:   ariadne.CanonicalAlbum{Title: "Album", SourceURL: url},
				Matches:  map[ariadne.ServiceName]ariadne.MatchResult{},
			},
		},
	}, discardLogger())

	reply, ok := service.BuildReply(context.Background(), url+". and again "+url)
	if !ok {
		t.Fatal("BuildReply() ok = false, want true")
	}

	want := strings.Join([]string{
		"<b>Album</b>",
		`<a href="https://www.deezer.com/album/12047952">Deezer</a>`,
	}, "\n")
	if reply != want {
		t.Fatalf("BuildReply() reply = %q, want %q", reply, want)
	}
}

func TestHandleDefaultSendsReply(t *testing.T) {
	const url = "https://open.spotify.com/album/example"

	recorder := newTelegramRecorder(t)
	defer recorder.server.Close()

	botClient := newTestBot(t, recorder.server)
	service := New(stubResolver{
		resolutions: map[string]*ariadne.Resolution{
			url: {
				InputURL: url,
				Parsed:   ariadne.ParsedAlbumURL{Service: ariadne.ServiceSpotify, CanonicalURL: url},
				Source:   ariadne.CanonicalAlbum{Title: "Album", Artists: []string{"Artist"}, SourceURL: url},
				Matches: map[ariadne.ServiceName]ariadne.MatchResult{
					ariadne.ServiceBandcamp: {Best: &ariadne.ScoredMatch{URL: "https://artist.bandcamp.com/album/example"}},
				},
			},
		},
	}, discardLogger())

	service.HandleDefault(context.Background(), botClient, &models.Update{
		Message: &models.Message{
			ID:              42,
			MessageThreadID: 7,
			Chat:            models.Chat{ID: 99},
			Text:            url,
		},
	})

	if got := recorder.calls(); len(got) != 2 || got[0] != "sendChatAction" || got[1] != "sendMessage" {
		t.Fatalf("calls = %v, want [sendChatAction sendMessage]", got)
	}

	values := recorder.form("sendMessage")
	if got := values["chat_id"]; got != "99" {
		t.Fatalf("chat_id = %q, want 99", got)
	}
	if got := values["message_thread_id"]; got != "7" {
		t.Fatalf("message_thread_id = %q, want 7", got)
	}
	if got := values["parse_mode"]; got != "HTML" {
		t.Fatalf("parse_mode = %q, want HTML", got)
	}
	if got := values["reply_parameters"]; !strings.Contains(got, `"message_id":42`) {
		t.Fatalf("reply_parameters = %q, want message_id 42", got)
	}
	if got := values["text"]; !strings.Contains(got, `Bandcamp</a> | <a href="https://open.spotify.com/album/example">Spotify`) {
		t.Fatalf("text = %q, missing expected links", got)
	}
}

func TestHandleDefaultIgnoresMessagesWithoutURL(t *testing.T) {
	recorder := newTelegramRecorder(t)
	defer recorder.server.Close()

	botClient := newTestBot(t, recorder.server)
	service := New(stubResolver{}, discardLogger())

	service.HandleDefault(context.Background(), botClient, &models.Update{
		Message: &models.Message{Chat: models.Chat{ID: 99}, Text: "hello"},
	})

	if got := recorder.calls(); len(got) != 0 {
		t.Fatalf("calls = %v, want none", got)
	}
}

func TestHandleStartSendsStartTextAndLogsRequest(t *testing.T) {
	recorder := newTelegramRecorder(t)
	defer recorder.server.Close()

	botClient := newTestBot(t, recorder.server)
	logger, output := bufferLogger()
	service := New(stubResolver{}, logger)

	service.HandleStart(context.Background(), botClient, &models.Update{
		ID: 100,
		Message: &models.Message{
			ID:   13,
			Chat: models.Chat{ID: 77, Type: models.ChatTypePrivate},
			From: &models.User{ID: 88, Username: "starter"},
			Text: "/start",
		},
	})

	calls := recorder.calls()
	if len(calls) != 1 || calls[0] != "sendMessage" {
		t.Fatalf("calls = %v, want [sendMessage]", calls)
	}

	values := recorder.form("sendMessage")
	if got := values["text"]; got != startText {
		t.Fatalf("text = %q, want %q", got, startText)
	}
	if got := output.String(); !strings.Contains(got, "incoming request") || !strings.Contains(got, "/start") {
		t.Fatalf("log output = %q, want incoming request with /start", got)
	}
}

func TestHandleHelpSendsHelpText(t *testing.T) {
	recorder := newTelegramRecorder(t)
	defer recorder.server.Close()

	botClient := newTestBot(t, recorder.server)
	service := New(stubResolver{}, discardLogger())

	service.HandleHelp(context.Background(), botClient, &models.Update{
		ChannelPost: &models.Message{ID: 13, Chat: models.Chat{ID: 77}},
	})

	calls := recorder.calls()
	if len(calls) != 1 || calls[0] != "sendMessage" {
		t.Fatalf("calls = %v, want [sendMessage]", calls)
	}

	values := recorder.form("sendMessage")
	if got := values["text"]; got != helpText {
		t.Fatalf("text = %q, want %q", got, helpText)
	}
}

func TestResolveSectionHandlesAmazonDeferred(t *testing.T) {
	service := New(stubResolver{
		errs: map[string]error{"https://music.amazon.com/albums/example": ariadne.ErrAmazonMusicDeferred},
	}, discardLogger())

	got := service.resolveSection(context.Background(), "https://music.amazon.com/albums/example")
	want := "Amazon Music album links not supported yet:\n<code>https://music.amazon.com/albums/example</code>"
	if got != want {
		t.Fatalf("resolveSection() = %q, want %q", got, want)
	}
}

func TestMessageHelpers(t *testing.T) {
	message := &models.Message{Text: "text", From: &models.User{ID: 5, Username: "tester"}}
	channelPost := &models.Message{Caption: "caption"}

	if got := messageFromUpdate(&models.Update{Message: message}); got != message {
		t.Fatal("messageFromUpdate() did not return message")
	}
	if got := messageFromUpdate(&models.Update{ChannelPost: channelPost}); got != channelPost {
		t.Fatal("messageFromUpdate() did not return channel post")
	}
	if got := messageFromUpdate(nil); got != nil {
		t.Fatal("messageFromUpdate(nil) != nil")
	}
	if got := messageText(message); got != "text" {
		t.Fatalf("messageText(text) = %q, want text", got)
	}
	if got := messageText(channelPost); got != "caption" {
		t.Fatalf("messageText(caption) = %q, want caption", got)
	}
	if got := messageText(nil); got != "" {
		t.Fatalf("messageText(nil) = %q, want empty", got)
	}
	if got := updateID(&models.Update{ID: 9}); got != 9 {
		t.Fatalf("updateID() = %d, want 9", got)
	}
	if got := fromID(message); got != 5 {
		t.Fatalf("fromID() = %d, want 5", got)
	}
	if got := fromID(nil); got != 0 {
		t.Fatalf("fromID(nil) = %d, want 0", got)
	}
	if got := fromUsername(message); got != "tester" {
		t.Fatalf("fromUsername() = %q, want tester", got)
	}
	if got := fromUsername(nil); got != "" {
		t.Fatalf("fromUsername(nil) = %q, want empty", got)
	}
	if got := logText("line1\nline2"); got != "line1 line2" {
		t.Fatalf("logText() = %q, want line1 line2", got)
	}
	if got := logText(strings.Repeat("x", 301)); len(got) != 300 || !strings.HasSuffix(got, "...") {
		t.Fatalf("logText() = %q, want truncated ellipsis", got)
	}
}

func TestServiceLabel(t *testing.T) {
	tests := map[ariadne.ServiceName]string{
		ariadne.ServiceAppleMusic:   "Apple Music",
		ariadne.ServiceBandcamp:     "Bandcamp",
		ariadne.ServiceDeezer:       "Deezer",
		ariadne.ServiceSoundCloud:   "SoundCloud",
		ariadne.ServiceSpotify:      "Spotify",
		ariadne.ServiceTIDAL:        "TIDAL",
		ariadne.ServiceYouTubeMusic: "YouTube Music",
		ariadne.ServiceAmazonMusic:  "Amazon Music",
		"custom":                    "custom",
	}

	for service, want := range tests {
		if got := serviceLabel(service); got != want {
			t.Fatalf("serviceLabel(%q) = %q, want %q", service, got, want)
		}
	}
}

func TestNewUsesDefaultLoggerWhenNil(t *testing.T) {
	service := New(stubResolver{}, nil)
	if service.logger == nil {
		t.Fatal("New() logger = nil")
	}
}

func TestResolveSectionHandlesGenericError(t *testing.T) {
	service := New(stubResolver{
		errs: map[string]error{"https://broken.example/album": errors.New("boom")},
	}, discardLogger())

	got := service.resolveSection(context.Background(), "https://broken.example/album")
	want := "Resolution failed right now:\n<code>https://broken.example/album</code>"
	if got != want {
		t.Fatalf("resolveSection() = %q, want %q", got, want)
	}
}

func TestAlbumHeading(t *testing.T) {
	tests := []struct {
		name  string
		album ariadne.CanonicalAlbum
		want  string
	}{
		{
			name:  "artist and title",
			album: ariadne.CanonicalAlbum{Artists: []string{"Artist"}, Title: "Album"},
			want:  "<b>Artist — Album</b>",
		},
		{
			name:  "title only",
			album: ariadne.CanonicalAlbum{Title: "Album"},
			want:  "<b>Album</b>",
		},
		{
			name:  "artist only",
			album: ariadne.CanonicalAlbum{Artists: []string{"Artist"}},
			want:  "<b>Artist</b>",
		},
		{
			name:  "empty",
			album: ariadne.CanonicalAlbum{},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := albumHeading(tt.album); got != tt.want {
				t.Fatalf("albumHeading() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatResolutionWithoutHeading(t *testing.T) {
	resolution := &ariadne.Resolution{
		InputURL: "https://example.com/album",
		Parsed:   ariadne.ParsedAlbumURL{Service: ariadne.ServiceSpotify, CanonicalURL: "https://example.com/album"},
		Matches: map[ariadne.ServiceName]ariadne.MatchResult{
			ariadne.ServiceBandcamp: {Best: &ariadne.ScoredMatch{URL: "https://artist.bandcamp.com/album/example"}},
		},
	}

	got := formatResolution(resolution)
	want := `<a href="https://artist.bandcamp.com/album/example">Bandcamp</a> | <a href="https://example.com/album">Spotify</a>`
	if got != want {
		t.Fatalf("formatResolution() = %q, want %q", got, want)
	}
}

func TestCollectLinksFallsBackToInputURL(t *testing.T) {
	links := collectLinks(&ariadne.Resolution{
		InputURL: "https://example.com/album",
		Parsed:   ariadne.ParsedAlbumURL{Service: ariadne.ServiceSpotify},
		Matches:  map[ariadne.ServiceName]ariadne.MatchResult{},
	})

	if got := links[ariadne.ServiceSpotify]; got != "https://example.com/album" {
		t.Fatalf("collectLinks() source = %q, want input url", got)
	}
}

func TestHandleDefaultLogsRequest(t *testing.T) {
	const url = "https://open.spotify.com/album/example"

	recorder := newTelegramRecorder(t)
	defer recorder.server.Close()

	botClient := newTestBot(t, recorder.server)
	logger, output := bufferLogger()
	service := New(stubResolver{
		resolutions: map[string]*ariadne.Resolution{
			url: {
				InputURL: url,
				Parsed:   ariadne.ParsedAlbumURL{Service: ariadne.ServiceSpotify, CanonicalURL: url},
				Source:   ariadne.CanonicalAlbum{Title: "Album", SourceURL: url},
				Matches:  map[ariadne.ServiceName]ariadne.MatchResult{},
			},
		},
	}, logger)

	service.HandleDefault(context.Background(), botClient, &models.Update{
		ID: 101,
		Message: &models.Message{
			ID:   42,
			Chat: models.Chat{ID: 99, Type: models.ChatTypePrivate},
			From: &models.User{ID: 123, Username: "listener"},
			Text: url,
		},
	})

	if got := output.String(); !strings.Contains(got, "incoming request") || !strings.Contains(got, url) {
		t.Fatalf("log output = %q, want incoming request with url", got)
	}
}

func TestHandleDefaultIgnoresNilUpdate(t *testing.T) {
	recorder := newTelegramRecorder(t)
	defer recorder.server.Close()

	botClient := newTestBot(t, recorder.server)
	service := New(stubResolver{}, discardLogger())

	service.HandleDefault(context.Background(), botClient, nil)
	if got := recorder.calls(); len(got) != 0 {
		t.Fatalf("calls = %v, want none", got)
	}
}

func TestHandleHelpIgnoresNilUpdate(t *testing.T) {
	recorder := newTelegramRecorder(t)
	defer recorder.server.Close()

	botClient := newTestBot(t, recorder.server)
	service := New(stubResolver{}, discardLogger())

	service.HandleHelp(context.Background(), botClient, nil)
	if got := recorder.calls(); len(got) != 0 {
		t.Fatalf("calls = %v, want none", got)
	}
}

func TestBoolPtr(t *testing.T) {
	if got := boolPtr(true); got == nil || !*got {
		t.Fatal("boolPtr(true) did not return true pointer")
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func bufferLogger() (*slog.Logger, *bytes.Buffer) {
	var output bytes.Buffer
	return slog.New(slog.NewTextHandler(&output, nil)), &output
}

type telegramRecorder struct {
	mu     sync.Mutex
	callsV []string
	forms  map[string]map[string]string
	server *httptest.Server
}

func newTelegramRecorder(t *testing.T) *telegramRecorder {
	t.Helper()

	recorder := &telegramRecorder{forms: make(map[string]map[string]string)}
	recorder.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm() error = %v", err)
		}

		method := pathMethod(r.URL.Path)
		values := make(map[string]string, len(r.MultipartForm.Value))
		for key, value := range r.MultipartForm.Value {
			if len(value) == 0 {
				continue
			}
			values[key] = value[0]
		}

		recorder.mu.Lock()
		recorder.callsV = append(recorder.callsV, method)
		recorder.forms[method] = values
		recorder.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch method {
		case "sendChatAction":
			_, _ = io.WriteString(w, `{"ok":true,"result":true}`)
		case "sendMessage":
			_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":1,"type":"private"}}}`)
		default:
			_, _ = io.WriteString(w, `{"ok":true,"result":{}}`)
		}
	}))
	return recorder
}

func (r *telegramRecorder) calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.callsV...)
}

func (r *telegramRecorder) form(method string) map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	clone := make(map[string]string, len(r.forms[method]))
	for key, value := range r.forms[method] {
		clone[key] = value
	}
	return clone
}

func newTestBot(t *testing.T, server *httptest.Server) *tgbot.Bot {
	t.Helper()

	botClient, err := tgbot.New(
		"123:test-token",
		tgbot.WithSkipGetMe(),
		tgbot.WithServerURL(server.URL),
	)
	if err != nil {
		t.Fatalf("bot.New() error = %v", err)
	}
	return botClient
}

func pathMethod(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}
