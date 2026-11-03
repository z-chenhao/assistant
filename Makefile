.PHONY: help setup install run build test test-go test-node test-browser test-infra test-e2e test-full test-race lint lint-actions audit

.DEFAULT_GOAL := setup

help:
	@echo 'make setup       Install locked development dependencies'
	@echo 'make build/run   Build / start Steward'
	@echo 'make test lint   Run software tests and local quality checks'
	@echo 'make test-e2e    Run core flows (set E2E_EVIDENCE to a fresh path)'
	@echo 'make test-full   Run all deterministic system flows'
	@echo 'make test-race   Run the complete Go race inventory'
	@echo 'make audit       Check current Go/npm vulnerability advisories (network required)'
	@echo 'See CONTRIBUTING.md for prerequisites and focused checks.'

# Operations live in scripts; software tests live in tests. Model evals are external.
setup:
	python3 scripts/setup.py $(SETUP_FLAGS)

install:
	sh install.sh

build:
	go build -o bin/steward ./cmd/steward

run: build
	./bin/steward start

# Go tests currently contain both unit and component integration tests.
# Keep that explicit rather than relabeling the entire package suite as UT.
test: test-go test-node test-browser test-infra

test-go:
	go test ./...

test-node:
	npm run check --prefix web
	npm test --prefix provider-bridge
	npm test --prefix runtime/javascript

test-browser:
	npm test --prefix guest-browser
	node web/artwork/icons.mjs --check

export PYTHONPATH := $(CURDIR)/tests/system:$(CURDIR)/scripts:$(PYTHONPATH)

test-infra:
	python3 -m unittest discover -s tests/harness -p 'test_*.py'

E2E_EVIDENCE ?= evidence/e2e

test-e2e: build
	python3 tests/system/runner.py --suite core --binary bin/steward --evidence "$(E2E_EVIDENCE)"

test-full: build
	python3 tests/system/runner.py --suite full --binary bin/steward --evidence "$(E2E_EVIDENCE)"

test-race:
	python3 tests/system/race.py

lint: lint-actions
	python3 scripts/check_docs.py
	sh -n install.sh scripts/package.sh
	@files="$$(gofmt -l cmd internal)"; if [ -n "$$files" ]; then echo "$$files"; exit 1; fi
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...
	go mod verify
	npm run lint --prefix provider-bridge
	npm run lint --prefix runtime/javascript
	npm run lint --prefix guest-browser
	npm run lint --prefix web

# Tool versions are explicit and do not alter the application's go.mod.
lint-actions:
	go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -color

# Advisory availability is separate from deterministic software regressions.
audit:
	go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
	npm audit --prefix provider-bridge --package-lock-only --audit-level=high
	npm audit --prefix runtime/javascript --package-lock-only --audit-level=high
	npm audit --prefix guest-browser --package-lock-only --audit-level=high
	npm audit --prefix web --package-lock-only --audit-level=high
