BINARY := ariadne-telegram-bot
CMD := ./cmd/ariadne-telegram-bot

.DEFAULT_GOAL := build

.PHONY: build
build:
	go build -o $(BINARY) $(CMD)
