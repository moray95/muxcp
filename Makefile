BINARY ?= muxcp
CONFIG ?= config.yaml
COVERAGE_THRESHOLD ?= 80

.PHONY: build run lint test check fix fmt clean

build:
	go build -o $(BINARY) ./cmd/muxcp

run: build
	./$(BINARY) -config $(CONFIG)

lint:
	golangci-lint run ./...

test:
	@go test -coverprofile=coverage.out -covermode=atomic -race -timeout 30s ./internal/... && \
	COVERAGE=$$(go tool cover -func=coverage.out | grep ^total | awk '{print $$3}' | tr -d '%'); \
	echo "Coverage: $${COVERAGE}%"; \
	if [ $$(echo "$${COVERAGE} < $(COVERAGE_THRESHOLD)" | bc) -eq 1 ]; then \
		echo "FAIL: coverage $${COVERAGE}% is below threshold $(COVERAGE_THRESHOLD)%"; \
		exit 1; \
	fi

check: lint test

fix:
	golangci-lint run --fix ./...
	golangci-lint fmt ./...

fmt:
	golangci-lint fmt ./...

clean:
	rm -f $(BINARY) coverage.out
