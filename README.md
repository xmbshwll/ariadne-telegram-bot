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

Telegram Bot API delivery modes for standard bots are **long polling** via `getUpdates` or **HTTPS webhooks**.
On startup bot clears any old webhook with `deleteWebhook`, then receives updates through `getUpdates`.

## Configuration

Required:

- `TELEGRAM_BOT_TOKEN`

Optional bot env vars:

- `LOG_LEVEL` — `debug`, `info`, `warn`, or `error`. Default: `info`
- `PORT` — when set, enables HTTP health endpoints on that port. Cloud Run sets this automatically

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

Optional: expose health endpoints locally:

```bash
docker run --rm --env-file .env -e PORT=8080 -p 8080:8080 ariadne-telegram-bot
```

When `PORT` is set, container serves:

- `GET /livez` — liveness, always returns `200 OK` while process is alive
- `GET /healthz` — startup/health, returns `503 Service Unavailable` until bot startup completes, then `200 OK`
- `GET /` — simple `200 OK` response for manual checks

Bot still uses outbound HTTPS long polling for Telegram updates.

GitHub Actions workflow `.github/workflows/docker.yml` builds Docker image on pushes, pull requests, and manual dispatch. Non-PR runs publish image to `ghcr.io/<owner>/<repo>`.

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

## Cloud Run

Configure Cloud Run probes against exposed HTTP endpoints:

- startup/health probe: `/healthz`
- liveness probe: `/livez`

Cloud Run sets `PORT`, so bot enables tiny HTTP probe server there automatically while keeping Telegram long polling in same process. In environments without `PORT`, no probe server starts.

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
