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

.PHONY: build test lint gate e2e e2e-up schema

build:
	go build ./...
	go build -trimpath -ldflags '$(LDFLAGS)' -o bin/doriki ./cmd/doriki

# No -race: the race detector needs cgo.
test:
	go test ./...

lint:
	golangci-lint run ./...

# .gate-ok is the proof .claude/hooks/git-gate.py checks before an agent commit. Remove it first
# and recreate it only after every check passes, so a failed gate never leaves a marker.
gate:
	@rm -f .gate-ok
	@$(MAKE) --no-print-directory build lint test
	@touch .gate-ok

# JSON Schemas from the Go types in internal/model; commit the result. The schema test fails
# when they are stale.
schema:
	go run ./schema/gen

# The live path end to end on a CPU kind cluster (DESIGN.md section 9.1): needs kind and a
# running Docker, and skips without them, saying so; DORIKI_E2E_REQUIRED=1 (CI) makes that a
# failure. Creates the kind cluster "doriki" when absent and leaves it for the next run.
e2e: build
	DORIKI_BIN=$(CURDIR)/bin/doriki go test -tags e2e -count=1 -timeout 15m -v ./test/e2e/

# Deploys the llama.cpp fixture's manifest to the kind cluster "doriki" (created when absent)
# and leaves it running for a manual check (PROJECT.md section 9). Uses your kubectl, with the
# cluster's own kubeconfig in bin/, so your kubeconfig is not touched.
E2E_MANIFEST   := packs/llamacpp/testdata/11889/llamacpp.yaml
E2E_KUBECONFIG := bin/kind-doriki.kubeconfig
e2e-up:
	@mkdir -p bin
	@kind get clusters 2>/dev/null | grep -qx doriki || kind create cluster --name doriki --kubeconfig $(E2E_KUBECONFIG) --wait 120s
	@kind get kubeconfig --name doriki > $(E2E_KUBECONFIG)
	kubectl --kubeconfig $(E2E_KUBECONFIG) apply -f $(E2E_MANIFEST)
	kubectl --kubeconfig $(E2E_KUBECONFIG) -n doriki rollout status deployment/llamacpp --timeout=10m
	@echo "llama.cpp is running; profile it with: go run ./cmd/doriki deploy/llamacpp -n doriki --kubeconfig $(E2E_KUBECONFIG)"
