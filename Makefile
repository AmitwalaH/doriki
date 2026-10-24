MODULE  := github.com/AmitwalaH/doriki
VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X $(MODULE)/internal/cli.version=$(VERSION) \
	-X $(MODULE)/internal/cli.commit=$(COMMIT) \
	-X $(MODULE)/internal/cli.date=$(DATE)

# doriki is a single static binary; no package may need cgo.
export CGO_ENABLED := 0

.PHONY: build test lint gate e2e

build:
	go build ./...
	go build -trimpath -ldflags '$(LDFLAGS)' -o bin/doriki ./cmd/doriki

# No -race: the race detector needs cgo.
test:
	go test ./...

lint:
	golangci-lint run ./...

gate: build lint test

# Red on purpose until M2 (PROJECT.md section 4).
e2e:
	@echo "e2e: not implemented until M2" >&2
	@exit 1
