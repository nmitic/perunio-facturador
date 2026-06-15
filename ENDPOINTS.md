# perunio-facturador — Endpoint Reference

Every HTTP endpoint exposed by the Go facturador service: its **path & contract**, its **purpose**, and **who calls it**. Verified against `internal/http/*.go` (`server.go` route table + handlers).

## Conventions

- **Base path:** every business endpoint is mounted under `/api/facturador` (Chi router, `server.go`).
- **Auth:** all `/api/facturador/*` routes pass through `authMW.Authenticate` — HS256 JWT in the `auth_token` httpOnly cookie, plus blacklist + `tokenVersion` checks. `/health` is the only unauthenticated route. Tenant isolation is set per-request via `db.WithTenant` → `SET app.current_tenant_id` (PostgreSQL RLS).
- **Success envelope:** `{ "success": true, "data": ... }` (`writeSuccess`). The list endpoints for documents/GRE instead return `{ "success": true, "data": [...], "pagination": {...} }`. This deliberately mirrors `perunio-backend`'s shape so the frontend consumes both services without per-call shape detection.
- **Error envelope:** `{ "success": false, "error": "<mensaje>", "code": "<CODE>" }` (`writeError`).
- **Decoding:** request bodies are parsed with `DisallowUnknownFields()` — unknown JSON keys are rejected as `400 VALIDATION_ERROR`.

### "Used by" legend

Who actually calls each endpoint at runtime (see `ARCHITECTURE.md` for the full picture):

| Marker | Meaning |
|---|---|
| 🟢 **frontend (live)** | `perunio-frontend` calls it today via `facturadorClient` (`VITE_FACTURADOR_BASE_URL`). |
| 🟡 **implemented, not wired** | Handler exists & works, but no client points here yet — the frontend still uses the equivalent `perunio-backend` endpoint. Migration target. |
| ⚪ **infra** | Called by platform infra (load balancer / uptime), not a product client. |

`perunio-admin` does **not** call this service at all (no facturador URL in its env).

---

## Health

### `GET /health` — ⚪ infra
**Purpose:** liveness probe. Unauthenticated, no DB touch.
**Response:** `200 {"status":"ok"}`.

---

## Usage

### `GET /api/facturador/usage` — 🟢 frontend (live)
**Purpose:** current billing period's issued-document count vs. the tenant's tier limit. Drives quota UI and "X of Y documents used" indicators. Mirrors the Node `getDocumentUsage` shape.
**Request:** none (tenant comes from JWT).
**Response:** `{ used: number, limit: number|null, tier: string, period: string }` — `limit` is `null` for unlimited tiers.
**Consumed by:** `documentsApi.getUsage()` in the frontend.

---

## Series — `cac` document series (F001, B001, …)

CRUD over the `document_series` table. `docType` ∈ `01` (factura), `03` (boleta), `07` (NC), `08` (ND). Series code must match `^[A-Z0-9]{1,4}$`. **All 🟢 frontend (live)** via `seriesApi`.

| Method & path | Purpose | Contract |
|---|---|---|
| `GET /series/{companyId}` | List every series row for a company (populates series dropdowns when issuing). | → `Series[]` |
| `POST /series/{companyId}` | Create a series. Enforces uniqueness on `(docType, series)`. | Body `{ docType, series, description? }` → `201 Series`. `409 SERIES_DUPLICATE` if it exists. |
| `PUT /series/{companyId}/{seriesId}` | Patch `description` / `isActive` (e.g. retire a series). | Body `{ description?, isActive? }` → `Series`. `404 NOT_FOUND`. |
| `DELETE /series/{companyId}/{seriesId}` | Delete a series — **only if it has no documents**. | → `{message}`. `409 SERIES_HAS_DOCUMENTS` if documents reference it. |

---

## Documents — facturas, boletas, notas de crédito/débito

The core: draft → issue pipeline (validate → UBL XML → sign → ZIP → SOAP → CDR → PDF). **All 🟢 frontend (live)** via `documentsApi`.

### `GET /documents/{companyId}` — list (paginated, filterable)
**Purpose:** the issued-documents table/grid.
**Query:** `page`, `limit` (1–100, default 20), `docType`, `status`, `customer` (customer doc number).
**Response:** `{ success, data: IssuedDocument[], pagination: { page, limit, total, totalPages } }`.

### `POST /documents/{companyId}` — create draft
**Purpose:** persist a draft document + line items before issuing. Runs **monthly quota check** (`CheckDocumentQuota`) then atomically increments the issued-document counter and reserves the correlative.
**Body (key fields):** `seriesId` (UUID), `issueDate` (`YYYY-MM-DD`), `currencyCode` (3-letter, defaults `PEN`), customer fields (optional at draft time — boletas default to *consumidor final*; facturas require RUC at issue time), monetary totals (`subtotal`, `totalIgv`, `totalAmount`, plus optional `globalDiscount`, `totalIsc`, …), optional credit terms (`formaPago`, `cuotas`), note references (`referenceDocType/Series/Correlative`, `creditDebitReasonCode/Desc`), and `items[]` (each: `description`, `quantity`, `unitCode`, `unitPrice`, `igvAmount`, `lineTotal`, optional discount/ISC/price-type). At least one item required.
**Response:** `201 IssuedDocument`. Errors: `400 VALIDATION_ERROR`, `403 DOCUMENT_QUOTA_EXCEEDED`, `400 SERIES_NOT_FOUND` (inactive/missing), `409 DOCUMENT_DUPLICATE`.

### `GET /documents/{companyId}/{docId}` — detail
**Purpose:** one document with its line items, for the detail view / edit form.
**Response:** `IssuedDocument` fields **plus** `items: IssuedDocumentItem[]`. `404 NOT_FOUND`.

### `PUT /documents/{companyId}/{docId}` — update draft
**Purpose:** edit a draft. All fields optional; if `items` is present it **fully replaces** existing line items.
**Response:** `IssuedDocument`. `400 NOT_DRAFT` if the document is no longer a draft, `404 NOT_FOUND`.

### `DELETE /documents/{companyId}/{docId}` — delete draft
**Purpose:** discard a draft. Only drafts deletable. `400 NOT_DRAFT`, `404 NOT_FOUND`.

### `POST /documents/{companyId}/{docId}/issue` — **run the SUNAT pipeline**
**Purpose:** the heart of the service. Loads the draft + company SUNAT credentials + active certificate, then: pre-submission **validation** → build **UBL 2.1** XML → **XMLDSig RSA-SHA1** sign → **ZIP** → SOAP **sendBill** → parse **CDR** → generate **PDF** (QR) → upload all artifacts to R2 → persist status + CDR outcome.
**Body (optional):** `{ environment?: "beta" | "production" }` — overrides the per-company default; falls back to `beta`.
**Response:** updated `IssuedDocument` (status + CDR summary). Notable errors: `400 ALREADY_ACCEPTED`, `400 NO_ITEMS`, `400` validation failures (with SUNAT codes), `400 SUNAT_CREDENTIALS_MISSING` / `400 CERTIFICATE_MISSING`, `502 SUNAT_ERROR`, plus `XML_BUILD_ERROR` / `SIGN_ERROR` / `ZIP_ERROR` / `CDR_PARSE_ERROR` / `R2_UPLOAD_ERROR`.

### `GET /documents/{companyId}/{docId}/files/{fileType}` — download artifact
**Purpose:** hand the browser a **presigned R2 URL** for a generated file. `fileType` ∈ `xml | signed_xml | zip | cdr` (PDF is generated in-pipeline; valid file keys come from the document row).
**Response:** `{ url, fileType }`. `400 INVALID_FILE_TYPE`, `404 FILE_NOT_FOUND` (artifact not yet generated).

---

## Summaries — Resúmenes Diarios de Boletas (RC)

Batch-reports accepted boletas to SUNAT via UBL **2.0** `SummaryDocuments`, using the **async ticket** flow (issue returns a ticket; poll fetches the CDR later). **Status: 🟡 implemented, not wired** — no frontend namespace points here yet (boletas summaries still go through `perunio-backend`).

| Method & path | Purpose | Contract |
|---|---|---|
| `GET /summaries/{companyId}` | List daily summaries. | → `DailySummary[]` |
| `POST /summaries/{companyId}` | Create a summary from **unlinked accepted boletas** for a date. | Body `{ referenceDate, summaryId }` → created summary. |
| `GET /summaries/{companyId}/{summaryId}` | Summary + its linked boleta items. | → summary + `items[]`. `404 NOT_FOUND`. |
| `POST /summaries/{companyId}/{summaryId}/issue` | Build RC XML → sign → ZIP → SOAP **sendSummary**; store the returned **ticket** on the row (CDR comes later). | Body `{ environment? }` → updated summary. `400 ALREADY_ACCEPTED`, `400 NO_ITEMS`, `502 SUNAT_ERROR`. |
| `POST /summaries/{companyId}/{summaryId}/poll` | Call SUNAT **getStatus** with the stored ticket; write the CDR outcome. | Body `{ environment? }` → updated summary. `400 NO_TICKET` if not yet issued, `502 SUNAT_ERROR`. |

---

## Voids — Comunicaciones de Baja (RA)

Cancels already-issued facturas (UBL **2.0** `VoidedDocuments`), enforcing SUNAT's **7-day window** at the DB layer. Same async ticket flow as summaries. **Status: 🟡 implemented, not wired.**

| Method & path | Purpose | Contract |
|---|---|---|
| `GET /voids/{companyId}` | List void communications. | → `VoidedDocument[]` |
| `POST /voids/{companyId}` | Create a void over one or more issued documents. DB enforces the 7-day limit. | Body `{ voidId, voidDate, documentIds[], reason }` → created void. |
| `GET /voids/{companyId}/{voidId}` | Void + its linked items. | → void + `items[]`. `404 NOT_FOUND`. |
| `POST /voids/{companyId}/{voidId}/issue` | Build RA XML → sign → ZIP → SOAP **sendSummary**; store ticket. | Body `{ environment? }` → updated void. `400 ALREADY_ACCEPTED`, `502 SUNAT_ERROR`. |
| `POST /voids/{companyId}/{voidId}/poll` | getStatus by ticket; persist CDR outcome. | Body `{ environment? }`. `400 NO_TICKET`, `502 SUNAT_ERROR`. |

---

## GRE — Guías de Remisión Electrónica (REST, not SOAP)

Despatch guides over SUNAT's **GRE REST API** with **OAuth2** (token cache keyed by `(companyID, environment)`; credentials AES-encrypted on the `companies` row). Doc types: `09` (Remitente), `31` (Transportista), `EV` (Por-eventos). Status: `draft → signed → sent → accepted/rejected/error`. **Status: 🟡 implemented, not wired** — the frontend's `greApi` currently calls `perunio-backend`'s `/gre/*`, which talks to SUNAT directly; this Go implementation is the migration target.

| Method & path | Purpose | Contract |
|---|---|---|
| `GET /gre/{companyId}` | List despatches (paginated). | Query `page`, `limit`, `docType`, `status` → `{ data: Despatch[], pagination }`. |
| `POST /gre/{companyId}` | Create a draft despatch (quota check + atomic correlative). | Body: `seriesId`, `docType`, `series`, `correlative`, `issueDate`, recipient (`recipientDocType/Number/Name/Address`), transport (`transportModality`, `transferReason`, weights, packages), route (`startUbigeo/Address`, `arrivalUbigeo/Address`), optional driver/vehicle (modality 02) or carrier (modality 01) blocks, optional `eventCode`/`originalGreId` (EV) and related-doc refs, plus `lines[]` → `201 Despatch`. |
| `GET /gre/{companyId}/{despatchId}` | Despatch + its `lines[]`. | → despatch + `lines`. `404 NOT_FOUND`. |
| `PUT /gre/{companyId}/{despatchId}` | Update a draft despatch. | Draft-only. |
| `DELETE /gre/{companyId}/{despatchId}` | Delete a draft despatch. | Draft-only. |
| `POST /gre/{companyId}/{despatchId}/issue` | validate → build XML → sign → ZIP → send via **GRE OAuth2 REST**; store ticket. | Body `{ environment? }`. |
| `POST /gre/{companyId}/{despatchId}/poll` | Poll the SUNAT ticket; base64-decode the CDR; persist outcome. | Body `{ environment? }`. |
| `GET /gre/{companyId}/{despatchId}/files/{fileType}` | Presigned R2 URL. `fileType` ∈ `xml \| signed_xml \| zip \| cdr`. | → `{ url, fileType }`. |

---

## Pipeline note — the `{ environment }` body

Every `issue` / `poll` endpoint accepts an **optional** `{ "environment": "beta" | "production" }` body. Resolution order (`resolvePipelineEnv`): explicit override → per-company default → `beta`. An empty/absent body is fine (it is not a validation error).

## Cross-service summary

| Resource | Endpoints | Used by today |
|---|---|---|
| usage, series, documents | `/usage`, `/series/*`, `/documents/*` | 🟢 `perunio-frontend` via `facturadorClient` |
| summaries, voids, gre | `/summaries/*`, `/voids/*`, `/gre/*` | 🟡 implemented here; frontend still hits `perunio-backend` |
| certificates | *(none — kept in backend)* | `perunio-backend` (`/companies/{id}/certificates`); the Go signing pipeline reads the active cert straight from the DB |
| health | `/health` | ⚪ infra / uptime |

_See `ARCHITECTURE.md` for service wiring, `CLAUDE.md` for internal package layout, and `SKILL.md` for the UBL/XML/SOAP/signing spec._
