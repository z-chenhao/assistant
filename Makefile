.PHONY: build test test-race lint verify

build:
	go build -o bin/personal-agent ./cmd/personal-agent

test:
	go test ./...

test-race:
	go test -race ./...

lint:
	@files="$$(gofmt -l cmd internal)"; if [ -n "$$files" ]; then echo "$$files"; exit 1; fi
	go vet ./...
	go mod verify

verify: build
	@mkdir -p evidence
	rm -f evidence/events.json evidence/relay.json evidence/models.jsonl
	python3 scripts/verify.py --binary bin/personal-agent --evidence evidence/verification.json
	go test -count=5 -json ./internal/llm/... ./internal/agent/sqlite > evidence/models.jsonl || { cat evidence/models.jsonl; exit 1; }
	ERII_TEST_BINARY="$(CURDIR)/bin/personal-agent" ERII_EVENT_EVIDENCE="$(CURDIR)/evidence/events.json" go test -count=1 -v ./internal/event/integration
