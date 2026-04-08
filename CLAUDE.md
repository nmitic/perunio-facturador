# CLAUDE.md

## Project Overview

perunio-facturador is a **stateless Go microservice** for SUNAT (Peru Tax Authority) electronic invoicing compliance. It handles XML generation, digital signing, SUNAT SOAP submission, CDR parsing, and PDF generation.

It is called by the Node.js backend (`perunio-backend`) via HTTP REST. It does NOT handle business logic, database access, or file storage — those remain in the backend.

## Development Commands

```bash
make build          # Build binary to bin/facturador
make run            # Run with go run
make test           # go test -shuffle on ./...
make lint           # golangci-lint run
make fmt            # gofmt -w .
```

Run a specific test:
```bash
go test ./internal/xmlbuilder -run TestBuildDocumentXML_Invoice
```

## Architecture

```
cmd/app/main.go              # Entry point, config loading, HTTP server
internal/
  config/                    # Environment-based configuration
  model/                     # Request/response types, SUNAT catalogs
  xmlbuilder/                # UBL 2.1 (Invoice/NC/ND) and 2.0 (RC/RA) XML builders
  signature/                 # PFX loading, XMLDSig RSA-SHA1 signing
  soap/                      # SUNAT SOAP client (sendBill, sendSummary, getStatus)
  cdr/                       # CDR ZIP extraction and ApplicationResponse parsing
  zipper/                    # ZIP creation for SUNAT submission
  validation/                # Pre-submission validation with SUNAT error codes
  pdf/                       # Invoice PDF generation with QR code
  crypto/                    # AES-256-GCM decryption (compatible with Node.js backend)
  http/                      # Chi router, handlers, middleware
```

## API Endpoints

All authenticated via `X-API-Key` header.

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/v1/documents/issue` | Full pipeline: validate -> XML -> sign -> ZIP -> SOAP -> CDR -> PDF |
| `POST` | `/api/v1/documents/validate` | Dry run validation only |
| `POST` | `/api/v1/documents/cdr` | Query CDR for specific document |
| `POST` | `/api/v1/summaries/issue` | Issue Resumen Diario (async, returns ticket) |
| `POST` | `/api/v1/summaries/status` | Poll ticket status |
| `POST` | `/api/v1/voids/issue` | Issue Comunicacion de Baja (async, returns ticket) |
| `POST` | `/api/v1/voids/status` | Poll ticket status |
| `POST` | `/api/v1/certificates/validate` | Validate PFX certificate |
| `GET` | `/health` | Health check |

## Environment Variables

```
PORT=8080                    # HTTP port
API_KEY=                     # Shared secret with backend (required)
ENCRYPTION_KEY=              # 64-char hex, same as backend's ENCRYPTION_KEY (required)
SUNAT_BETA_URL=              # Override beta endpoint
SUNAT_PRODUCTION_URL=        # Override production endpoint
SUNAT_CONSULT_URL=           # Override consultation endpoint
```

## Critical SUNAT Gotchas

- RC/RA use UBL **2.0**, NOT 2.1 — #1 cause of silent rejections
- Encoding MUST be ISO-8859-1, not UTF-8
- XMLDSig uses RSA-**SHA1** for SOAP interop
- Tax tolerance: +/-1 cent on computed vs declared amounts
- Boleta > S/700 requires customer identity
- NC on boletas: codes 04, 05, 08 are forbidden
- RC deadline: 7 calendar days, max 500 lines per block

## Reference

See `SKILL.md` for complete UBL/XML/SOAP/signing specification.
