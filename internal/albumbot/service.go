package albumbot

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"regexp"
	"slices"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/xmbshwll/ariadne"
)

var urlPattern = regexp.MustCompile(`https?://[^\s<>()]+`)

var defaultServiceOrder = []ariadne.ServiceName{
	ariadne.ServiceAppleMusic,
	ariadne.ServiceBandcamp,
	ariadne.ServiceDeezer,
	ariadne.ServiceSoundCloud,
	ariadne.ServiceSpotify,
	ariadne.ServiceTIDAL,
	ariadne.ServiceYouTubeMusic,
	ariadne.ServiceAmazonMusic,
}

type albumResolver interface {
	ResolveAlbum(ctx context.Context, inputURL string) (*ariadne.Resolution, error)
}

type Service struct {
	resolver albumResolver
	logger   *slog.Logger
}

func New(resolver albumResolver, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{resolver: resolver, logger: logger}
}

func (s *Service) HandleDefault(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	message := messageFromUpdate(update)
	if message == nil {
		return
	}

	s.logIncomingRequest(update, message)

	text := messageText(message)
	if strings.TrimSpace(text) == "" {
		return
	}
	if len(extractURLs(text)) == 0 {
		return
	}

	_, _ = b.SendChatAction(ctx, &tgbot.SendChatActionParams{
		ChatID:          message.Chat.ID,
		MessageThreadID: message.MessageThreadID,
		Action:          models.ChatActionTyping,
	})

	reply, ok := s.BuildReply(ctx, text)
	if !ok {
		return
	}

	if _, err := b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:          message.Chat.ID,
		MessageThreadID: message.MessageThreadID,
		Text:            reply,
		ParseMode:       models.ParseModeHTML,
		ReplyParameters: &models.ReplyParameters{MessageID: message.ID},
		LinkPreviewOptions: &models.LinkPreviewOptions{
			IsDisabled: boolPtr(true),
		},
	}); err != nil {
		s.logger.Error("send reply failed", "error", err, "chat_id", message.Chat.ID, "message_id", message.ID)
	}
}

func (s *Service) HandleStart(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	message := messageFromUpdate(update)
	if message == nil {
		return
	}

	s.logIncomingRequest(update, message)

	if _, err := b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:          message.Chat.ID,
		MessageThreadID: message.MessageThreadID,
		Text:            startText,
		ReplyParameters: &models.ReplyParameters{MessageID: message.ID},
		LinkPreviewOptions: &models.LinkPreviewOptions{
			IsDisabled: boolPtr(true),
		},
	}); err != nil {
		s.logger.Error("send start failed", "error", err, "chat_id", message.Chat.ID, "message_id", message.ID)
	}
}

func (s *Service) HandleHelp(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	message := messageFromUpdate(update)
	if message == nil {
		return
	}

	s.logIncomingRequest(update, message)

	if _, err := b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:          message.Chat.ID,
		MessageThreadID: message.MessageThreadID,
		Text:            helpText,
		ReplyParameters: &models.ReplyParameters{MessageID: message.ID},
		LinkPreviewOptions: &models.LinkPreviewOptions{
			IsDisabled: boolPtr(true),
		},
	}); err != nil {
		s.logger.Error("send help failed", "error", err, "chat_id", message.Chat.ID, "message_id", message.ID)
	}
}

func (s *Service) BuildReply(ctx context.Context, text string) (string, bool) {
	urls := extractURLs(text)
	if len(urls) == 0 {
		return "", false
	}

	sections := make([]string, 0, len(urls))
	for _, rawURL := range urls {
		sections = append(sections, s.resolveSection(ctx, rawURL))
	}

	return strings.Join(sections, "\n\n"), true
}

func (s *Service) resolveSection(ctx context.Context, rawURL string) string {
	resolution, err := s.resolver.ResolveAlbum(ctx, rawURL)
	if err == nil {
		return formatResolution(resolution)
	}

	escapedURL := html.EscapeString(rawURL)

	switch {
	case errors.Is(err, ariadne.ErrUnsupportedURL):
		return fmt.Sprintf("Could not resolve album link:\n<code>%s</code>", escapedURL)
	case errors.Is(err, ariadne.ErrAmazonMusicDeferred):
		return fmt.Sprintf("Amazon Music album links not supported yet:\n<code>%s</code>", escapedURL)
	default:
		s.logger.Error("resolve album failed", "error", err, "url", rawURL)
		return fmt.Sprintf("Resolution failed right now:\n<code>%s</code>", escapedURL)
	}
}

func formatResolution(resolution *ariadne.Resolution) string {
	heading := albumHeading(resolution.Source)
	links := formatLinks(collectLinks(resolution))
	if heading == "" {
		return links
	}
	return heading + "\n" + links
}

func albumHeading(album ariadne.CanonicalAlbum) string {
	artist := strings.TrimSpace(strings.Join(album.Artists, ", "))
	title := strings.TrimSpace(album.Title)

	switch {
	case artist != "" && title != "":
		return "<b>" + html.EscapeString(artist+" — "+title) + "</b>"
	case title != "":
		return "<b>" + html.EscapeString(title) + "</b>"
	case artist != "":
		return "<b>" + html.EscapeString(artist) + "</b>"
	default:
		return ""
	}
}

func collectLinks(resolution *ariadne.Resolution) map[ariadne.ServiceName]string {
	links := make(map[ariadne.ServiceName]string, len(resolution.Matches)+1)

	sourceURL := firstNonEmpty(
		resolution.Source.SourceURL,
		resolution.Parsed.CanonicalURL,
		resolution.InputURL,
	)
	if sourceURL != "" {
		links[resolution.Parsed.Service] = sourceURL
	}

	for service, match := range resolution.Matches {
		if match.Best == nil {
			continue
		}
		url := strings.TrimSpace(match.Best.URL)
		if url == "" {
			continue
		}
		links[service] = url
	}

	return links
}

func formatLinks(links map[ariadne.ServiceName]string) string {
	services := orderedServices(links)
	parts := make([]string, 0, len(services))
	for _, service := range services {
		parts = append(parts, fmt.Sprintf(
			`<a href="%s">%s</a>`,
			html.EscapeString(links[service]),
			html.EscapeString(serviceLabel(service)),
		))
	}
	return strings.Join(parts, " | ")
}

func orderedServices(links map[ariadne.ServiceName]string) []ariadne.ServiceName {
	services := make([]ariadne.ServiceName, 0, len(links))
	seen := make(map[ariadne.ServiceName]struct{}, len(defaultServiceOrder))

	for _, service := range defaultServiceOrder {
		if _, ok := links[service]; !ok {
			continue
		}
		services = append(services, service)
		seen[service] = struct{}{}
	}

	extra := make([]ariadne.ServiceName, 0, len(links)-len(services))
	for service := range links {
		if _, ok := seen[service]; ok {
			continue
		}
		extra = append(extra, service)
	}

	slices.SortFunc(extra, func(a, b ariadne.ServiceName) int {
		return strings.Compare(serviceLabel(a), serviceLabel(b))
	})

	return append(services, extra...)
}

func extractURLs(text string) []string {
	matches := urlPattern.FindAllString(text, -1)
	if len(matches) == 0 {
		return nil
	}

	urls := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		match = strings.TrimRight(match, ".,!?;:)]}")
		if match == "" {
			continue
		}
		if _, ok := seen[match]; ok {
			continue
		}
		seen[match] = struct{}{}
		urls = append(urls, match)
	}

	return urls
}

func (s *Service) logIncomingRequest(update *models.Update, message *models.Message) {
	s.logger.Info(
		"incoming request",
		"update_id", updateID(update),
		"chat_id", message.Chat.ID,
		"chat_type", message.Chat.Type,
		"message_id", message.ID,
		"message_thread_id", message.MessageThreadID,
		"from_id", fromID(message),
		"from_username", fromUsername(message),
		"text", logText(messageText(message)),
	)
}

func messageFromUpdate(update *models.Update) *models.Message {
	if update == nil {
		return nil
	}
	if update.Message != nil {
		return update.Message
	}
	if update.ChannelPost != nil {
		return update.ChannelPost
	}
	return nil
}

func messageText(message *models.Message) string {
	if message == nil {
		return ""
	}
	if strings.TrimSpace(message.Text) != "" {
		return message.Text
	}
	return message.Caption
}

func updateID(update *models.Update) int64 {
	if update == nil {
		return 0
	}
	return update.ID
}

func fromID(message *models.Message) int64 {
	if message == nil || message.From == nil {
		return 0
	}
	return message.From.ID
}

func fromUsername(message *models.Message) string {
	if message == nil || message.From == nil {
		return ""
	}
	return message.From.Username
}

func logText(text string) string {
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "\n", " ")
	if len(text) <= 300 {
		return text
	}
	return text[:297] + "..."
}

func serviceLabel(service ariadne.ServiceName) string {
	switch service {
	case ariadne.ServiceAppleMusic:
		return "Apple Music"
	case ariadne.ServiceBandcamp:
		return "Bandcamp"
	case ariadne.ServiceDeezer:
		return "Deezer"
	case ariadne.ServiceSoundCloud:
		return "SoundCloud"
	case ariadne.ServiceSpotify:
		return "Spotify"
	case ariadne.ServiceTIDAL:
		return "TIDAL"
	case ariadne.ServiceYouTubeMusic:
		return "YouTube Music"
	case ariadne.ServiceAmazonMusic:
		return "Amazon Music"
	default:
		return string(service)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func boolPtr(value bool) *bool {
	return &value
}

const (
	startText = "Hi! I can find matching album links across music services.\n\nSend album link from Apple Music, Bandcamp, Deezer, SoundCloud, Spotify, TIDAL, or YouTube Music."
	helpText  = "Send album link from Apple Music, Bandcamp, Deezer, SoundCloud, Spotify, TIDAL, or YouTube Music.\n\nI will reply with matching links like Apple Music | Bandcamp | Spotify | YouTube Music."
)
