.PHONY: build run test lint fmt

build:
	go build -o bin/facturador ./cmd/app

run:
	go run ./cmd/app

test:
	go test -shuffle on ./...

lint:
	golangci-lint run

fmt:
	gofmt -w .
