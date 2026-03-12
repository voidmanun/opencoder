.PHONY: build run test clean

BINARY_NAME := dingtalk-bridge
MAIN_PATH := ./cmd/dingtalk-bridge

build:
	go build -o $(BINARY_NAME) $(MAIN_PATH)

run:
	go run $(MAIN_PATH)

test:
	go test -v ./...

test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

clean:
	rm -f $(BINARY_NAME)
	rm -f coverage.out coverage.html

deps:
	go mod download
	go mod tidy

lint:
	go fmt ./...
	go vet ./...

install: build
	cp $(BINARY_NAME) /usr/local/bin/

dev:
	go run $(MAIN_PATH) --help