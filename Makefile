.PHONY: build test test-race lint verify

build:
	go build -o bin/personal-agent ./cmd/personal-agent

test:
	go test ./...
	npm test --prefix provider-bridge

test-race:
	go test -race ./...

lint:
	@files="$$(gofmt -l cmd internal)"; if [ -n "$$files" ]; then echo "$$files"; exit 1; fi
	go vet ./...
	go mod verify
	npm run lint --prefix provider-bridge

verify: build
	@mkdir -p evidence
	rm -f evidence/events.json evidence/relay.json evidence/models.jsonl
	python3 scripts/verify.py --binary bin/personal-agent --evidence evidence/verification.json
	python3 scripts/verify_local_provider.py --binary bin/personal-agent --evidence evidence/local-provider.json
	rm -f evidence/pi-bridge.tap
	@for attempt in 1 2 3 4 5; do node --test --test-reporter=tap provider-bridge/test/*.test.mjs >> evidence/pi-bridge.tap || { cat evidence/pi-bridge.tap; exit 1; }; done
	go test -count=5 -json ./internal/llm/... ./internal/agent/sqlite > evidence/models.jsonl || { cat evidence/models.jsonl; exit 1; }
	ERII_TEST_BINARY="$(CURDIR)/bin/personal-agent" ERII_EVENT_EVIDENCE="$(CURDIR)/evidence/events.json" go test -count=1 -v ./internal/event/integration
