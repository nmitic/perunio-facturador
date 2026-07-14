.PHONY: build run test test-prod lint fmt

build:
	go build -o bin/facturador ./cmd/app

run:
	go run ./cmd/app

test:
	go test -shuffle on ./...

# Runs the signing tests against the exact xmlsec version prod ships (see
# Dockerfile.test). Use this before shipping signature changes — your host
# xmlsec differs from prod and won't catch strict-key-search regressions.
test-prod:
	docker build -f Dockerfile.test -t perunio-facturador-test .

lint:
	golangci-lint run

fmt:
	gofmt -w .
