.PHONY: setup install run build test test-race lint verify release-audit

setup:
	npm ci --prefix provider-bridge --ignore-scripts
	npm ci --prefix runtime/javascript --ignore-scripts
	npm ci --prefix guest-browser --ignore-scripts
	npm ci --prefix web --ignore-scripts
	node guest-browser/node_modules/playwright-core/cli.js install chromium

install:
	sh install.sh

run: build
	./bin/erii start

build:
	go build -o bin/erii ./cmd/erii

test:
	npm run check --prefix web
	go test ./...
	npm test --prefix provider-bridge
	npm test --prefix runtime/javascript
	npm test --prefix guest-browser
	python3 -m unittest discover -s scripts -p 'test_*.py'

# The full SQLite package repeatedly migrates real schemas under instrumentation.
# Ubuntu exceeded Go's implicit 10m package budget while its current test was 1s old.
# Keep per-test/Tool/lease deadlines intact; bound the complete package at 15m.
# Run Bridge before unrelated compiler/race work, then every remaining package
# at normal parallelism. The maintained runner audits the complete inventory.
test-race:
	python3 scripts/ci_race.py

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

# Shards own isolated fixtures and disjoint evidence; repetitions stay serial.
VERIFY_JOBS ?= 2
VERIFY_EVIDENCE ?= evidence
VERIFY_SHARD ?=

verify: build
	python3 scripts/verify_suite.py --binary bin/erii --evidence "$(VERIFY_EVIDENCE)" --jobs "$(VERIFY_JOBS)" $(if $(VERIFY_SHARD),--shard "$(VERIFY_SHARD)")

# A successful inventory is not release acceptance. This returns 2 while the
# recorded whole-case/audit/model/L2 gaps remain; deterministic CI stays offline.
release-audit:
	python3 scripts/release_audit.py --require-complete

# Use a fresh durable fixture root outside the checkout; never contacts GitHub.
.PHONY: verify-evidence
verify-evidence:
	@test -n "$(EVIDENCE_ROOT)" || { echo 'Set EVIDENCE_ROOT to a fresh absolute durable fixture root'; exit 1; }
	python3 scripts/evidence_harness.py --root "$(EVIDENCE_ROOT)" --sha "$$(git rev-parse HEAD)" --evidence "$(EVIDENCE_ROOT)/report.json"
