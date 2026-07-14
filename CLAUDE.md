# CLAUDE.md

## Project Overview

perunio-facturador is a **stateful Go microservice** for SUNAT (Peru Tax Authority) electronic invoicing. It owns the full facturador domain: certificates, series, draft documents, daily summaries, void communications, and the compliance pipeline (validate → XML → sign → ZIP → SOAP → CDR → PDF).

It talks directly to the shared PostgreSQL DB (RLS-isolated by tenant), Cloudflare R2 buckets, and AWS Secrets Manager. Clients (frontend/admin) call it directly; `perunio-backend` handles auth/billing/consultas but no longer touches facturador resources.

## Development Commands

```bash
make build          # Build binary to bin/facturador
make run            # Run with go run
make test           # go test -shuffle on ./...
make test-prod      # signing tests against prod's exact xmlsec (Docker)
make lint           # golangci-lint run
make fmt            # gofmt -w .
```

Run a specific test:
```bash
go test ./internal/xmlbuilder -run TestBuildDocumentXML_Invoice
```

### Testing signing against the production xmlsec

`make test` uses whatever `xmlsec1` is on your host. That version can differ
from production and behave differently: xmlsec >=1.3 made key search strict
(KEY-NOT-FOUND on our empty `ds:KeyInfo` template — `signer.go` adds
`--lax-key-search` to compensate). Ubuntu's apt ships 1.2.x, so `make test`
locally never exercises the 1.3.x path that runs in prod.

`make test-prod` closes that gap. It builds `Dockerfile.test`, which compiles
the signature test binary and runs it inside the **exact** runtime prod ships
(`alpine:3.21` + `xmlsec=1.3.7-r0`). The test binary is run *during the image
build*, so a failing test aborts the build with a non-zero exit — no volume
mounts, no changes to your host xmlsec.

Run it before shipping any change to `internal/signature`, `internal/xmlbuilder`
(signature template), or the xmlsec pin in `Dockerfile`. Keep the alpine tag and
xmlsec pin in `Dockerfile.test` in sync with `Dockerfile`.

## Architecture

```
cmd/app/main.go              # Entry point: awssecrets -> db pool -> r2 client -> http server
internal/
  awssecrets/                # AWS Secrets Manager client (JWT + encryption keys)
  config/                    # Environment config (R2, DB, SUNAT URLs + GRE URLs)
  auth/                      # JWT middleware + tenant-id context helpers
  db/                        # pgxpool + WithTenant (RLS) + table helpers
  r2/                        # Cloudflare R2 (S3-compatible) certificates + documents
  model/                     # Domain types shared across the pipeline (incl. Despatch)
  xmlbuilder/                # UBL 2.1 (Invoice/NC/ND), 2.0 (RC/RA), and GRE (Despatch) XML builders
  signature/                 # PFX loading, XMLDSig RSA-SHA1 signing
  soap/                      # SUNAT SOAP client (sendBill, sendSummary, getStatus)
  greclient/                 # SUNAT GRE REST client: OAuth2 token cache, Send, Poll
  cdr/                       # CDR ZIP extraction and ApplicationResponse parsing
  zipper/                    # ZIP creation for SUNAT submission
  validation/                # Pre-submission validation with SUNAT error codes (incl. Despatch)
  pdf/                       # Invoice PDF generation with QR code
  crypto/                    # AES-256-GCM en/decryption (shared format with Node.js)
  http/                      # Chi router, JWT middleware, route handlers
```

## API Endpoints

All under `/api/facturador/*`, JWT-authenticated via the `auth_token` cookie (HS256, same secret as `perunio-backend`, sourced from AWS Secrets Manager). RLS isolation is enforced in `db.WithTenant` by setting `app.current_tenant_id` on the transaction.

### Certificates
- `GET /certificates/{companyId}` — list
- `POST /certificates/{companyId}` — upload PFX + encrypt password
- `GET /certificates/{companyId}/{certId}` — metadata
- `PUT /certificates/{companyId}/{certId}/activate` — set active
- `DELETE /certificates/{companyId}/{certId}` — delete

### Series
- `GET /series/{companyId}`
- `POST /series/{companyId}` — create (unique on docType+series)
- `PUT /series/{companyId}/{seriesId}`
- `DELETE /series/{companyId}/{seriesId}` — refuses if docs exist

### Documents
- `GET /documents/{companyId}` — paginated, filterable
- `POST /documents/{companyId}` — create draft (quota check + atomic correlative)
- `GET /documents/{companyId}/{docId}` — with line items
- `PUT /documents/{companyId}/{docId}` — update draft
- `DELETE /documents/{companyId}/{docId}` — delete draft
- `POST /documents/{companyId}/{docId}/issue` — run full pipeline against draft
- `GET /documents/{companyId}/{docId}/files/{fileType}` — presigned R2 URL (`xml|signed_xml|zip|cdr|pdf`)

### Summaries
- `GET /summaries/{companyId}`
- `POST /summaries/{companyId}` — create daily summary from unlinked accepted boletas
- `GET /summaries/{companyId}/{summaryId}` — with linked items
- `POST /summaries/{companyId}/{summaryId}/issue` — sign + send RC, store ticket
- `POST /summaries/{companyId}/{summaryId}/poll` — poll by ticket, write CDR outcome

### Voids
- `GET /voids/{companyId}`
- `POST /voids/{companyId}` — create void (enforces 7-day window)
- `GET /voids/{companyId}/{voidId}` — with linked items
- `POST /voids/{companyId}/{voidId}/issue` — sign + send RA, store ticket
- `POST /voids/{companyId}/{voidId}/poll` — poll by ticket, write CDR outcome

### GRE (Guías de Remisión Electrónica) — REST API via `greclient`
- `GET /gre/{companyId}` — list despatches (paginated, filterable by docType/status)
- `POST /gre/{companyId}` — create draft despatch (quota check + atomic correlative)
- `GET /gre/{companyId}/{despatchId}` — with line items
- `PUT /gre/{companyId}/{despatchId}` — update draft
- `DELETE /gre/{companyId}/{despatchId}` — delete draft
- `POST /gre/{companyId}/{despatchId}/issue` — validate → build XML → sign → zip → send via GRE OAuth2 REST, store ticket
- `POST /gre/{companyId}/{despatchId}/poll` — poll SUNAT ticket, decode base64 CDR, persist outcome
- `GET /gre/{companyId}/{despatchId}/files/{fileType}` — presigned R2 URL (`xml|signed_xml|zip|cdr`)

GRE credentials (OAuth2 `client_id` + `client_secret`) are stored AES-encrypted on the `companies` row (`encrypted_client_id`, `encrypted_client_secret`). The token cache in `greclient` is keyed by `(companyID, environment)`.

Despatch doc types: `09` (Remitente), `31` (Transportista), `EV` (Por-eventos). Statuses: `draft → signed → sent → accepted / rejected / error`.

### Other
- `GET /usage` — current month's document usage + tier limit
- `GET /health` — unauthenticated health check

## Environment Variables

```
PORT=8080                        # HTTP port
DATABASE_URL=                    # Shared PostgreSQL (required)

# AWS Secrets Manager (source of truth for JWT + encryption keys)
AWS_SECRET_NAME=                 # Empty -> falls back to JWT_SECRET / ENCRYPTION_KEY env vars (dev)
AWS_REGION=

# Cloudflare R2 (required)
R2_ACCOUNT_ID=
R2_ACCESS_KEY_ID=
R2_SECRET_ACCESS_KEY=
R2_CERTIFICATES_BUCKET=perunio-certificates
R2_DOCUMENTS_BUCKET=perunio-facturador

# SUNAT SOAP endpoints (defaults baked in)
SUNAT_BETA_URL=
SUNAT_PRODUCTION_URL=
SUNAT_CONSULT_URL=

# SUNAT GRE REST endpoints (defaults baked in)
SUNAT_GRE_SECURITY_URL=     # default: https://api-seguridad.sunat.gob.pe
SUNAT_GRE_BETA_URL=         # default: https://api-cpe.sunat.gob.pe
SUNAT_GRE_PRODUCTION_URL=   # default: https://api-cpe.sunat.gob.pe
```

## Critical SUNAT Gotchas

- RC/RA use UBL **2.0**, NOT 2.1 — #1 cause of silent rejections
- Encoding MUST be ISO-8859-1, not UTF-8
- XMLDSig uses RSA-**SHA1** for SOAP interop
- Tax tolerance: +/-1 cent on computed vs declared amounts
- Boleta > S/700 requires customer identity
- NC on boletas: codes 04, 05, 08 are forbidden
- RC deadline: 7 calendar days, max 500 lines per block

### Line discounts (descuento por ítem, Cat.53 "00")
- **Invoices** (`cac:InvoiceLine`): keep `cac:Price/cbc:PriceAmount` at the **gross** valor unitario and emit a line `cac:AllowanceCharge` (`ChargeIndicator=false`, reason `00`, `MultiplierFactorNumeric`, `Amount`, `BaseAmount`). SUNAT recomputes `LineExtensionAmount = Price × Qty − Amount`; omitting the AllowanceCharge → **fault 3271**.
- **Notes** (`cac:CreditNoteLine` / `cac:DebitNoteLine`): SUNAT's note-line model has **no** line `cac:AllowanceCharge`. It computes `LineExtensionAmount = Quantity × cac:Price` (guía NC, punto 29), so the descuento must be **baked into a NET `cac:Price`** (`= LineExtensionAmount ÷ Quantity`) with no AllowanceCharge. Emitting an InvoiceLine-style AllowanceCharge on a note → SUNAT ignores it and **fault 3271**. (`noteLineUnitPrice` in `invoice.go`.)
- Per-line discounts are already in each line's `LineExtensionAmount`; do **not** also declare them as a document `cbc:AllowanceTotalAmount` (that's for global discounts only).

### Gratuito / free items (Cat.07 codes 11-16, 21, 31-37) — all verified against accepted SUNAT CDRs
- Every gratuito line uses **tributo 9996 (GRA/FRE)**, `cac:Price/PriceAmount = 0`, and `PricingReference/AlternativeConditionPrice` PriceTypeCode **02** carrying the IGV-**exclusive** per-unit referencial (not the IGV-inclusive price → **fault 3272**).
- Line `cbc:LineExtensionAmount` must equal the line's base imponible (the referencial base), **not** 0 → otherwise **fault 3272**. The document `LegalMonetaryTotal` totals stay 0 (customer pays nothing).
- The document `cac:TaxTotal` **must** include a `9996` subtotal aggregating the gratuito base + would-be IGV; document `cbc:TaxAmount` excludes gratuito IGV. Falling back to a `1000` (IGV) subtotal makes the op look onerosa → **fault 3224**.
- Both line and document `9996` subtotals must carry a `cbc:Percent` rate tag (**fault 2992**): `18.00` for gravado-gratuito (11-16, IGV at 18%), `0.00` for exonerado/inafecto gratuito (21, 31-37, no IGV).
- Reference: greenter's `Factura-Gratuita` output is the source of truth this matches.

## Reference

See `SKILL.md` for complete UBL/XML/SOAP/signing specification.
