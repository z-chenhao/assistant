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
	python3 scripts/verify.py --binary bin/personal-agent --evidence evidence/verification.json
