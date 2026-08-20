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
| 🟡 **implemented, not wired** | Handler exists & works, but no client points here yet — the frontend still uses the equivalent `perunio-backend` endpoint. Migration target. (Only **summaries** remain here.) |
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
| `POST /series/{companyId}` | Create a series. Enforces uniqueness on `(docType, series)`. | Body `{ docType, series, description?, nextCorrelative? }` → `201 Series`. `409 SERIES_DUPLICATE` if it exists. |
| `PUT /series/{companyId}/{seriesId}` | Patch `description` / `isActive` (e.g. retire a series) / `nextCorrelative`. | Body `{ description?, isActive?, nextCorrelative? }` → `Series`. `404 NOT_FOUND`, `409 CORRELATIVE_TOO_LOW`. |
| `DELETE /series/{companyId}/{seriesId}` | Delete a series — **only if it has no documents**. | → `{message}`. `409 SERIES_HAS_DOCUMENTS` if documents reference it. |

### `nextCorrelative` — migrating from another facturador

A company arriving with `F001` already in use at 4312 seeds the counter so its
first comprobante here is 4313 instead of 1. Range `1 … 99999999` (SUNAT's
correlativo is 8 digits); omit the field for a brand-new serie.

It seeds **`next_correlative` (producción) only** — `next_correlative_beta` is an
independent sandbox sequence at SUNAT's side, and neither counter resets on an
environment switch, so a user can test in beta from 1 and still resume at 4313 in
producción.

On `PUT` it is **raise-only** and must clear every number the serie already put on
the wire (`MAX(correlative)` over production `issued_documents` **and**
`despatches` — GRE series share `document_series`). Violations return `409
CORRELATIVE_TOO_LOW` with the minimum named in the message. The check and the
write share one transaction with the row locked, so a concurrent draft creation
cannot interleave. Changes are recorded in `audit_logs` as
`series_correlative_set` — the only `audit_logs` writer in this service.

---

## Documents — facturas, boletas, notas de crédito/débito

The core: draft → issue pipeline (validate → UBL XML → sign → ZIP → SOAP → CDR → PDF). **All 🟢 frontend (live)** via `documentsApi`.

### `GET /documents/{companyId}` — list (paginated, filterable)
**Purpose:** the issued-documents table/grid.
**Query:** `page`, `limit` (1–100, default 20), `docType`, `status`, `customer` (free-text, case-insensitive partial match over customer **name** and **doc number**), `payment` (derived payment status: `pagado`|`parcial`|`pendiente`|`vencido`).
**Response:** `{ success, data: IssuedDocument[], pagination: { page, limit, total, totalPages } }`. Each row carries a derived `paymentStatus` (`pagado`|`parcial`|`pendiente`) + `paymentOverdue` — contado from `paid_at`, crédito rolled up from the cuotas schedule vs recorded installment payments (mirrors the installments report). Not populated on the detail endpoint (the CuotasPanel derives it live there).

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

### Installment payments — cuotas recorded against a credit document

Records the actual payments applied to a credit document's cuotas. The cuotas *schedule* lives as jsonb on the document; these rows are the *actuals*. Cuota status (pagada / parcial / vencida / pendiente) is always **derived** from `sum(amount)` vs the scheduled `monto` vs `fechaVencimiento` — never stored. **All 🟢 frontend (live)** via `documentsApi.listPayments/createPayment/deletePayment`.

| Method & path | Purpose | Contract |
|---|---|---|
| `GET /documents/{companyId}/{docId}/payments` | List recorded payments for a document. | → `InstallmentPayment[]` (ordered by cuota, then date). |
| `POST /documents/{companyId}/{docId}/payments` | Record one payment against a cuota. Guarded by an EXISTS check so a payment can't attach to a document outside the company/tenant. | Body `{ cuotaNumero, amount, paidAt, method?, reference?, notes? }` → `201 InstallmentPayment`. `404 NOT_FOUND`. |
| `DELETE /documents/{companyId}/{docId}/payments/{paymentId}` | Delete a recorded payment. | → `{message}`. `404 NOT_FOUND`. |

### Manual payment status — contado documents

A document-level "pagado" mark for **contado** comprobantes, orthogonal to SUNAT `status` and independent of the cuotas system. Crédito documents are rejected here (they record payment per-cuota above). Drafts and voided documents can't be marked paid. `paid_at` present = pagado. **🟢 frontend (live)** via `documentsApi.setPayment/clearPayment`.

| Method & path | Purpose | Contract |
|---|---|---|
| `PUT /documents/{companyId}/{docId}/payment` | Mark a contado document as paid. | Body `{ paidAt, method?, reference?, notes? }` → refreshed `IssuedDocument`. `409 CREDIT_DOCUMENT` (crédito), `409 INVALID_STATUS` (draft/voided), `404 NOT_FOUND`. |
| `DELETE /documents/{companyId}/{docId}/payment` | Revert to unpaid (nulls every payment_* column). | → `{message}`. `404 NOT_FOUND`. |

### Attachments — supporting documents on a comprobante

Arbitrary internal-only supporting files (a signed order, a payment voucher, a contract) stored in R2 under the comprobante's per-document prefix (`…/{docId}/attachments/{attachmentId}.{ext}`). Never exposed on the public share link. Extension allow-list + 10 MiB cap. **🟢 frontend (live)** via `documentsApi.listAttachments/uploadAttachment/getAttachmentUrl/deleteAttachment`.

| Method & path | Purpose | Contract |
|---|---|---|
| `GET /documents/{companyId}/{docId}/attachments` | List attachments (newest first). | → `ComprobanteAttachment[]`. |
| `POST /documents/{companyId}/{docId}/attachments` | Upload a supporting file (`multipart/form-data`, field `file`). | → `201 ComprobanteAttachment`. `413`/MaxBytes, `415 UNSUPPORTED_TYPE`, `404 NOT_FOUND`. |
| `GET /documents/{companyId}/{docId}/attachments/{attachmentId}/download` | Presigned R2 URL (original filename, attachment disposition). | → `{url}`. `404 NOT_FOUND`. |
| `DELETE /documents/{companyId}/{docId}/attachments/{attachmentId}` | Delete attachment (row + R2 object). | → `{message}`. `404 NOT_FOUND`. |

---

## Reports — aggregations over issued documents

Read-only GROUP BY aggregations over `issued_documents` / `issued_document_items`. Every report reuses the same company + current-SUNAT-environment scope as the historial (`docScope`), so the two can never disagree about which documents exist. Money reports add `status IN ('accepted','accepted_with_observations')` + `issue_date BETWEEN from AND to` + `currency_code`. Sign convention: facturas/boletas (01/03) `+`, notas de crédito (07) `−`, notas de débito (08) `+`. **All 🟢 frontend (live)** via `reportesFacturadorApi`.

Shared query params (`parseReportFilter`): `from`, `to` (YYYY-MM-DD, default current month), `currency` (default `PEN`).

| Method & path | Purpose |
|---|---|
| `GET /reports/{companyId}/sales/summary` | Revenue, IGV, per-type counts, average sale. |
| `GET /reports/{companyId}/sales/series` | Net sales bucketed by `?bucket=day\|month\|year` (default month). |
| `GET /reports/{companyId}/customers` | Customer ranking, `?orderBy=revenue\|count\|average`, `?limit=`. |
| `GET /reports/{companyId}/products` | Product ranking, `?orderBy=quantity\|revenue`, `?limit=`. Groups by producto link, falls back to normalized description. |
| `GET /reports/{companyId}/tax/breakdown` | Base+IGV per afectación category (gravado/exonerado/inafecto/exportación/gratuito) from items + document tax totals (IGV/ISC/other/total). |
| `GET /reports/{companyId}/tax/notes` | Notas de crédito/débito grouped by reason code. `?docType=07\|08`. |
| `GET /reports/{companyId}/installments` | Per-credit-document cuotas rollup (paid/balance/next due/status). `?status=paid\|partial\|pending\|overdue`. |
| `GET /reports/{companyId}/sunat/submissions` | Document counts by SUNAT status (all statuses, currency-agnostic). |

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

Cancels already-issued facturas (UBL **2.0** `VoidedDocuments`), enforcing SUNAT's **7-day window** at the DB layer. Same async ticket flow as summaries. **Status: 🟢 frontend (live)** via `voidsApi`. A `poll` response is either the finished void (has `status`) or, while SUNAT is still processing the ticket, `{ statusCode }` (e.g. `"98"`).

| Method & path | Purpose | Contract |
|---|---|---|
| `GET /voids/{companyId}` | List void communications. | → `VoidedDocument[]` |
| `POST /voids/{companyId}` | Create a void over one or more issued documents. DB enforces the 7-day limit. | Body `{ voidId, voidDate, documentIds[], reason }` → created void. |
| `GET /voids/{companyId}/{voidId}` | Void + its linked items. | → void + `items[]`. `404 NOT_FOUND`. |
| `POST /voids/{companyId}/{voidId}/issue` | Build RA XML → sign → ZIP → SOAP **sendSummary**; store ticket. | Body `{ environment? }` → updated void. `400 ALREADY_ACCEPTED`, `502 SUNAT_ERROR`. |
| `POST /voids/{companyId}/{voidId}/poll` | getStatus by ticket; persist CDR outcome. | Body `{ environment? }`. `400 NO_TICKET`, `502 SUNAT_ERROR`. |

---

## GRE — Guías de Remisión Electrónica (REST, not SOAP)

Despatch guides over SUNAT's **GRE REST API** with **OAuth2** (token cache keyed by `(companyID, environment)`; credentials AES-encrypted on the `companies` row). Doc types: `09` (Remitente), `31` (Transportista), `EV` (Por-eventos). Status: `draft → signed → sent → accepted/rejected/error`. **Status: 🟢 frontend (live)** via `despatchesApi` — the despatch *emission* pipeline (create draft → issue → poll → files). **Not** to be confused with the frontend's `greApi`, which is GRE *consulta* (search / detail / download) and still calls `perunio-backend`'s `/gre/*`. As with voids, a `poll` response is either the finished despatch (has `status`) or `{ statusCode }` while the ticket is still processing.

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
| usage, series, documents | `/usage`, `/series/*`, `/documents/*` | 🟢 `perunio-frontend` via `facturadorClient` (`documentsApi`, `seriesApi`) |
| installment payments | `/documents/*/payments` | 🟢 `perunio-frontend` via `documentsApi` (cuotas payment tracking) |
| reports | `/reports/*` | 🟢 `perunio-frontend` via `reportesFacturadorApi` |
| voids | `/voids/*` | 🟢 `perunio-frontend` via `voidsApi` |
| gre (emission) | `/gre/*` | 🟢 `perunio-frontend` via `despatchesApi` (despatch emission). GRE *consulta* is a separate `greApi` → `perunio-backend`. |
| summaries | `/summaries/*` | 🟡 implemented here; frontend still hits `perunio-backend` |
| certificates | *(none — kept in backend)* | `perunio-backend` (`/companies/{id}/certificates`); the Go signing pipeline reads the active cert straight from the DB |
| health | `/health` | ⚪ infra / uptime |

_See `ARCHITECTURE.md` for service wiring, `CLAUDE.md` for internal package layout, and `SKILL.md` for the UBL/XML/SOAP/signing spec._
