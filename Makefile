.PHONY: build test test-race lint verify

build:
	go build -o bin/personal-agent ./cmd/personal-agent

test:
	go test ./...
	npm test --prefix provider-bridge
	npm test --prefix runtime/javascript
	python3 -m unittest discover -s scripts -p 'test_*.py'

# The full SQLite package repeatedly migrates real schemas under instrumentation.
# Ubuntu exceeded Go's implicit 10m package budget while its current test was 1s old.
# Keep per-test/Tool/lease deadlines intact; bound the complete package at 15m.
# Run Bridge before unrelated compiler/race work, then every remaining package
# at normal parallelism. The maintained runner audits the complete inventory.
test-race:
	python3 scripts/ci_race.py

lint:
	@files="$$(gofmt -l cmd internal)"; if [ -n "$$files" ]; then echo "$$files"; exit 1; fi
	go vet ./...
	go mod verify
	npm run lint --prefix provider-bridge
	npm run lint --prefix runtime/javascript

# Shards own isolated fixtures and disjoint evidence; repetitions stay serial.
VERIFY_JOBS ?= 2
VERIFY_EVIDENCE ?= evidence
VERIFY_SHARD ?=

verify: build
	python3 scripts/verify_suite.py --binary bin/personal-agent --evidence "$(VERIFY_EVIDENCE)" --jobs "$(VERIFY_JOBS)" $(if $(VERIFY_SHARD),--shard "$(VERIFY_SHARD)")
