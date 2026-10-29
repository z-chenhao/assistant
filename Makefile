.PHONY: setup install run build test test-go test-node test-browser test-infra test-e2e test-full test-race lint

# Operations live in scripts; software tests live in tests. Model evals are external.
setup:
	python3 scripts/setup.py $(SETUP_FLAGS)

install:
	sh install.sh

build:
	go build -o bin/collie ./cmd/collie

run: build
	./bin/collie start

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

export PYTHONPATH := $(CURDIR)/tests/system:$(CURDIR)/scripts:$(PYTHONPATH)

test-infra:
	python3 -m unittest discover -s tests/harness -p 'test_*.py'

E2E_EVIDENCE ?= evidence/e2e

test-e2e: build
	python3 tests/system/runner.py --suite core --binary bin/collie --evidence "$(E2E_EVIDENCE)"

test-full: build
	python3 tests/system/runner.py --suite full --binary bin/collie --evidence "$(E2E_EVIDENCE)"

test-race:
	python3 tests/system/race.py

lint:
	python3 scripts/check_docs.py
	sh -n install.sh scripts/package.sh
	@files="$$(gofmt -l cmd internal)"; if [ -n "$$files" ]; then echo "$$files"; exit 1; fi
	go vet ./...
	go mod verify
	npm run lint --prefix provider-bridge
	npm run lint --prefix runtime/javascript
	npm run lint --prefix guest-browser
	npm run lint --prefix web
