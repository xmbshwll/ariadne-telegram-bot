# ariadne-telegram-bot

Telegram bot in Go. Uses [`github.com/go-telegram/bot`](https://github.com/go-telegram/bot) for Telegram transport and [`github.com/xmbshwll/ariadne`](https://github.com/xmbshwll/ariadne) for album resolution.

User sends album link from supported music service. Bot replies with clickable service-name links separated by ` | `.

Example reply:

```text
Apple Music | Bandcamp | Spotify | YouTube Music
```

## Supported input

Album links from services Ariadne can resolve at runtime today:

- Apple Music
- Bandcamp
- Deezer
- SoundCloud
- Spotify
- TIDAL
- YouTube Music

Bot currently resolves **albums only**.

## Telegram delivery mode

Telegram Bot API does **not** use WebSocket here.
This bot uses **long polling** only.

On startup bot clears any old webhook with `deleteWebhook`, then starts polling with `getUpdates`.

## Configuration

Required:

- `TELEGRAM_BOT_TOKEN`

Optional bot env vars:

- `LOG_LEVEL` — `debug`, `info`, `warn`, or `error`. Default: `info`

Optional Ariadne env vars:

- `SPOTIFY_CLIENT_ID`
- `SPOTIFY_CLIENT_SECRET`
- `APPLE_MUSIC_STOREFRONT`
- `APPLE_MUSIC_KEY_ID`
- `APPLE_MUSIC_TEAM_ID`
- `APPLE_MUSIC_PRIVATE_KEY_PATH`
- `APPLE_MUSIC_PRIVATE_KEY`
- `APPLE_MUSIC_PRIVATE_KEY_BASE64`
- `TIDAL_CLIENT_ID`
- `TIDAL_CLIENT_SECRET`
- `ARIADNE_HTTP_TIMEOUT`
- `ARIADNE_TARGET_SERVICES`

Use `.env.example` as template, then export vars into shell before start:

`LOG_LEVEL=debug` enables Telegram Bot API debug logging for requests and responses through `github.com/go-telegram/bot`.

```bash
cp .env.example .env
set -a
source .env
set +a
```

## Run

Build binary:

```bash
make build
```

Run from source:

```bash
go run ./cmd/ariadne-telegram-bot
```

## Docker

Build image:

```bash
docker build -t ariadne-telegram-bot .
```

Run container with env file:

```bash
docker run --rm --env-file .env ariadne-telegram-bot
```

No ports needed. Bot uses long polling.

Apple Music `.p8` handling in Docker:

Recommended: mount secret file read-only and point `APPLE_MUSIC_PRIVATE_KEY_PATH` at it.

```bash
docker run --rm \
  --env-file .env \
  -e APPLE_MUSIC_PRIVATE_KEY_PATH=/run/secrets/apple/AuthKey.p8 \
  -v "$PWD/secrets/AuthKey_ABC123XYZ.p8:/run/secrets/apple/AuthKey.p8:ro" \
  ariadne-telegram-bot
```

Supported fallback: pass key through env var. Bot will materialize temp `AuthKey.p8` file at startup and remove it on shutdown.

```bash
docker run --rm \
  --env-file .env \
  -e APPLE_MUSIC_PRIVATE_KEY_BASE64="$(base64 < ./secrets/AuthKey_ABC123XYZ.p8 | tr -d '\n')" \
  ariadne-telegram-bot
```

Use env-var fallback only when secret-file mount is not available. Env vars are easier to leak through container metadata, shell history, and process inspection.

## Test

```bash
go test ./...
```

## Notes

- Response uses Telegram HTML links with link previews disabled.
- `LOG_LEVEL=debug` enables Telegram request/response debug output in bot logs.
- Bot logs successful Telegram connection on startup with standard Go `log` output and `INFO` prefix.
- Spotify target search needs Spotify credentials.
- TIDAL input and target resolution need TIDAL credentials.
- Apple Music metadata search works without MusicKit credentials, but identifier search gets better when credentials are set.
- If `APPLE_MUSIC_PRIVATE_KEY_PATH` is empty and `APPLE_MUSIC_PRIVATE_KEY` or `APPLE_MUSIC_PRIVATE_KEY_BASE64` is set, bot writes temp `AuthKey.p8` file with `0600` permissions and points Ariadne at it automatically.
