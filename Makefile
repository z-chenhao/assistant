.PHONY: build test test-race lint verify

build:
	go build -o bin/personal-agent ./cmd/personal-agent

test:
	go test ./...
	npm test --prefix provider-bridge
	python3 -m unittest discover -s scripts -p 'test_*.py'

test-race:
	go test -race ./...

lint:
	@files="$$(gofmt -l cmd internal)"; if [ -n "$$files" ]; then echo "$$files"; exit 1; fi
	go vet ./...
	go mod verify
	npm run lint --prefix provider-bridge

# Shards own isolated fixtures and disjoint evidence; repetitions stay serial.
VERIFY_JOBS ?= 2
VERIFY_EVIDENCE ?= evidence
VERIFY_SHARD ?=

verify: build
	python3 scripts/verify_suite.py --binary bin/personal-agent --evidence "$(VERIFY_EVIDENCE)" --jobs "$(VERIFY_JOBS)" $(if $(VERIFY_SHARD),--shard "$(VERIFY_SHARD)")
