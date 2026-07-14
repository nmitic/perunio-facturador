FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app ./cmd/app

FROM alpine:3.21
# xmlsec is the XMLDSig signing engine (see internal/signature). Its version is
# pinned because signing behavior changed across releases: xmlsec 1.3.0 made key
# search strict (KEY-NOT-FOUND on our empty ds:KeyInfo template) — signer.go
# adds --lax-key-search for >=1.3.0. Pin so the lib can't change under us; if
# Alpine drops this version the build fails loudly — bump it here and re-verify
# a real SUNAT signing before shipping. Find the current version with:
#   docker run --rm alpine:3.21 sh -c 'apk add xmlsec >/dev/null 2>&1 && apk list -I xmlsec'
RUN apk add --no-cache ca-certificates tzdata xmlsec=1.3.7-r0
COPY --from=build /app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
