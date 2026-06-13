# wireguard-ui build targets.
# The frontend is built into internal/web/dist, then embedded by `go build`,
# producing a single self-contained static binary.

BINARY := wireguard-ui
# Version stamped into the binary; defaults to the closest git tag.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all web build run dev clean

all: build

## web: build the Vue frontend into internal/web/dist
web:
	cd web && npm install && npm run build

## build: build the frontend then the embedded Go binary
build: web
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/wireguard-ui

## build-go: build only the Go binary (uses whatever is in internal/web/dist)
build-go:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/wireguard-ui

## run: build and run locally
run: build
	./$(BINARY)

## dev: run backend (:8000) and vite dev server (:5173) -- two terminals
dev:
	@echo "Terminal 1:  go run ./cmd/wireguard-ui"
	@echo "Terminal 2:  cd web && npm run dev   # open http://localhost:5173"

clean:
	rm -f $(BINARY)
	rm -rf web/node_modules internal/web/dist/assets
