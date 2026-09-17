.PHONY: build run test test-prod lint fmt \
        docker-build docker-vet docker-test docker-fmt docker-check docker-shell

# ---------------------------------------------------------------------------
# Host targets — require a local Go toolchain.
# ---------------------------------------------------------------------------

build:
	go build -o bin/facturador ./cmd/app

run:
	go run ./cmd/app

test:
	go test -shuffle on ./...

# Runs the signing tests against the exact xmlsec version prod ships (see
# Dockerfile.test). Use this before shipping signature changes — your host
# xmlsec differs from prod and won't catch strict-key-search regressions.
#
# NOTE: Dockerfile.test does not exist yet, so this target currently fails.
# Kept as the declared intent; see the repo TODO rather than deleting it.
test-prod:
	docker build -f Dockerfile.test -t perunio-facturador-test .

lint:
	golangci-lint run

fmt:
	gofmt -w .

# ---------------------------------------------------------------------------
# Docker targets — the same gates, with no Go installed on the host.
#
# Every developer here already needs Docker (the xmlsec pin in Dockerfile is
# why), while Go on the host is optional — so these exist to make "can I still
# ship this?" answerable from a clean Windows machine. They run the SAME image
# tag the Dockerfile builds from, so the compiler and stdlib match production
# rather than whatever happens to be on PATH.
#
# Both caches are named volumes: without them every run re-downloads the whole
# module graph (~30 modules) and recompiles the stdlib, turning a 3-second test
# run into a 2-minute one.
# ---------------------------------------------------------------------------

GO_IMAGE   := golang:1.26-alpine
GO_DOCKER   = docker run --rm \
	-v "$(CURDIR)":/src \
	-v perunio-go-mod-cache:/go/pkg/mod \
	-v perunio-go-build-cache:/root/.cache/go-build \
	-w /src $(GO_IMAGE)

docker-build:
	$(GO_DOCKER) go build ./...

docker-vet:
	$(GO_DOCKER) go vet ./...

docker-test:
	$(GO_DOCKER) go test -shuffle on ./...

# gofmt -l on a Windows checkout flags nearly every file, because the working
# tree has CRLF line endings and gofmt wants LF. That makes a plain `gofmt -l .`
# useless as a gate here, so this strips CR before checking and reports only
# files whose formatting is genuinely wrong. Read-only — it never rewrites.
docker-fmt:
	$(GO_DOCKER) sh -c 'bad=""; for f in $$(find . -name "*.go" -not -path "./vendor/*"); do \
		tr -d "\r" < "$$f" > /tmp/f.go; \
		if [ -n "$$(gofmt -l /tmp/f.go)" ]; then bad="$$bad $$f"; fi; \
	done; \
	if [ -n "$$bad" ]; then echo "needs gofmt:$$bad"; exit 1; fi; \
	echo "gofmt: all files formatted"'

# What CI enforces (build + vet + test), runnable before you push.
docker-check: docker-build docker-vet docker-test

# A shell in the same image, for poking at things by hand.
docker-shell:
	$(GO_DOCKER) sh
