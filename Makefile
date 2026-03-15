BINARY ?= muxcp
CONFIG ?= config.yaml

.PHONY: build run lint fix fmt clean

build:
	go build -o $(BINARY) ./cmd/muxcp

run: build
	./$(BINARY) -config $(CONFIG)

lint:
	golangci-lint run ./...

fix:
	golangci-lint run --fix ./...
	golangci-lint fmt ./...

fmt:
	golangci-lint fmt ./...

clean:
	rm -f $(BINARY)
