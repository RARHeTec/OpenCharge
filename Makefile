GO ?= go

.PHONY: sync build test lint tidy

sync:
	$(GO) work sync

build:
	$(GO) build ./...

test:
	$(GO) test ./...

tidy:
	$(GO) work sync
