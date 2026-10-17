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

verify: build
	@mkdir -p evidence
	rm -f evidence/events.json evidence/relay.json evidence/models.jsonl
	python3 scripts/verify.py --binary bin/personal-agent --evidence evidence/verification.json
	python3 scripts/verify_local_provider.py --binary bin/personal-agent --evidence evidence/local-provider.json
	python3 scripts/verify_account.py --binary bin/personal-agent --evidence evidence/account.json
	python3 scripts/verify_main.py --binary bin/personal-agent --evidence evidence/main.json
	python3 scripts/verify_tasks.py --binary bin/personal-agent --evidence evidence/tasks.json
	@for attempt in 1 2 3 4 5; do python3 scripts/verify_children.py --binary bin/personal-agent --evidence evidence/children-$$attempt.json || exit 1; done
	@for attempt in 1 2 3 4 5; do python3 scripts/verify_scheduling.py --binary bin/personal-agent --evidence evidence/scheduling-$$attempt.json || exit 1; done
	@for attempt in 1 2 3 4 5; do python3 scripts/verify_task_control.py --binary bin/personal-agent --evidence evidence/task-control-$$attempt.json || exit 1; done
	@for attempt in 1 2 3 4 5; do python3 scripts/verify_tools.py --binary bin/personal-agent --evidence evidence/tools-$$attempt.json || exit 1; done
	@for attempt in 1 2 3 4 5; do python3 scripts/verify_artifacts.py --binary bin/personal-agent --evidence evidence/artifacts-$$attempt.json || exit 1; done
	@for attempt in 1 2 3 4 5; do python3 scripts/verify_delivery.py --binary bin/personal-agent --evidence evidence/delivery-$$attempt.json || exit 1; done
	@for attempt in 1 2 3 4 5; do python3 scripts/verify_capabilities.py --binary bin/personal-agent --evidence evidence/capabilities-$$attempt.json || exit 1; done
	python3 scripts/run_cases.py --binary bin/personal-agent --evidence evidence/cases.json
	python3 scripts/run_task_cases.py --binary bin/personal-agent --evidence evidence/task-cases.json
	python3 scripts/verify_tui.py --binary bin/personal-agent --evidence evidence/tui.json
	@for attempt in 2 3 4 5; do python3 scripts/verify_tui.py --binary bin/personal-agent --evidence evidence/tui-$$attempt.json || exit 1; done
	python3 scripts/verify_terminal_input.py --binary bin/personal-agent --evidence evidence/terminal-input.json
	rm -f evidence/pi-bridge.tap
	@for attempt in 1 2 3 4 5; do node --test --test-reporter=tap provider-bridge/test/*.test.mjs >> evidence/pi-bridge.tap || { cat evidence/pi-bridge.tap; exit 1; }; done
	@for attempt in 1 2 3 4 5; do python3 scripts/verify_account.py --binary bin/personal-agent --evidence evidence/account-$$attempt.json || exit 1; done
	go test -count=5 -json ./internal/llm/... ./internal/agent/sqlite > evidence/models.jsonl || { cat evidence/models.jsonl; exit 1; }
	ERII_TEST_BINARY="$(CURDIR)/bin/personal-agent" ERII_EVENT_EVIDENCE="$(CURDIR)/evidence/events.json" go test -count=1 -v ./internal/event/integration
