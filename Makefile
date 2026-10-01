.PHONY: help setup doctor run build start vet test check lint fmt clean

# Local development on Ubuntu. Production does not use this file: Railway
# builds from Dockerfile (see railway.json) and CI runs plain `go` commands.

BIN := bin/facturador

help:
	@echo "First time on Ubuntu:  sudo apt install make && make setup"
	@echo ""
	@echo "  make setup    install Go + xmlsec1 via apt, download modules"
	@echo "  make doctor   verify Go, xmlsec1 and .env are in place"
	@echo "  make run      run from source (the app loads .env itself)"
	@echo "  make start    build $(BIN) and run the binary"
	@echo "  make check    build + vet + test (what CI enforces)"
	@echo "  make build | vet | test | lint | fmt | clean"

# xmlsec1 is a runtime dependency, not a build one: signing shells out to the
# xmlsec1 binary (internal/signature), so the service starts without it but
# every issue call fails.
#
# apt's Go may be a patch release behind go.mod; Go then fetches the exact
# version go.mod pins on first use (GOTOOLCHAIN=auto is Ubuntu's default).
setup:
	sudo apt-get update
	sudo apt-get install -y golang-go xmlsec1 ca-certificates
	go mod download
	@$(MAKE) --no-print-directory doctor

doctor:
	@status=0; \
	if command -v go >/dev/null; then echo "ok    $$(go version)"; \
	else echo "FAIL  go not found (run: make setup)"; status=1; fi; \
	if command -v xmlsec1 >/dev/null; then echo "ok    $$(xmlsec1 --version)"; \
	else echo "FAIL  xmlsec1 not found (run: make setup)"; status=1; fi; \
	if [ -f .env ]; then echo "ok    .env present"; \
	else echo "FAIL  .env missing (DATABASE_URL and R2_* are required)"; status=1; fi; \
	exit $$status

run:
	go run ./cmd/app

build:
	go build -o $(BIN) ./cmd/app

start: build
	./$(BIN)

vet:
	go vet ./...

test:
	go test -shuffle on ./...

check: build vet test

lint:
	golangci-lint run

fmt:
	gofmt -w .

clean:
	rm -rf bin
