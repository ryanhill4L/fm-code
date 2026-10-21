VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
PREFIX  ?= $(HOME)/.local

.PHONY: build test install uninstall

build:
	go build -ldflags "-X main.version=$(VERSION)" -o fm-code .

test:
	go vet ./...
	go test ./...

install: build
	install -d $(PREFIX)/bin
	install -m 755 fm-code $(PREFIX)/bin/fm-code
	@echo "installed $(PREFIX)/bin/fm-code (make sure $(PREFIX)/bin is on your PATH)"

uninstall:
	rm -f $(PREFIX)/bin/fm-code
