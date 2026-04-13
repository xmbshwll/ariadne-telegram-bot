# syntax=docker/dockerfile:1.7

FROM golang:1.26.2-bookworm AS build
WORKDIR /src

ENV CGO_ENABLED=0 \
    GOPROXY=direct \
    GONOSUMDB=github.com/xmbshwll/ariadne

ARG TARGETOS=linux
ARG TARGETARCH

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags='-s -w' -o /out/ariadne-telegram-bot ./cmd/ariadne-telegram-bot

FROM alpine:3.21
WORKDIR /app
COPY --from=build --chown=65532:65532 /out/ariadne-telegram-bot /app/ariadne-telegram-bot
USER 65532:65532
ENTRYPOINT ["/app/ariadne-telegram-bot"]
