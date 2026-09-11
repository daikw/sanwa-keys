.PHONY: build test vet check install
VERSION ?= dev
PREFIX ?= $(HOME)/.local
build:
	go build -ldflags '-X main.version=$(VERSION)' -o dist/sanwa-keys ./cmd/sanwa-keys
test:
	go test -race ./...
vet:
	go vet ./...
check: vet test
install: build
	install -d $(PREFIX)/bin
	install -m 755 dist/sanwa-keys $(PREFIX)/bin/sanwa-keys
