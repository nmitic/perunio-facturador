# Follow-ups

Findings that came up while fixing the pending XML spec cases (branch
`fix/xmlbuilder-pending-specs`, plan F1–F22) and were deliberately left out
of that work. Each entry says what is wrong or missing, what SUNAT requires,
and where the code is, so it can be picked up later without re-investigating.

When you pick one up: add spec cases first (`internal/xmlbuilder/spec/`, see its
README), then fix, then delete the entry here.

## FU-1 — Otros tributos (Cat.05 9999) are half-built

**Status:** wanted as a feature, not scheduled. No spec case exists yet.

**Today**
- The frontend always sends `totalOtherTaxes: '0.00'`
  (`perunio-frontend/src/hooks/use-comprobante-form.ts:836`,
  `perunio-frontend/src/lib/nota-request.ts:124`); there is no UI to enter it.
- The facturador API accepts any value (`internal/http/routes_documents.go:270`),
  stores it in `issued_documents.total_other_taxes`, and the builder emits a
  document 9999 subtotal from it (`internal/xmlbuilder/invoice.go`,
  `buildDocumentTaxTotal`) with the **wrong base** (`req.Subtotal`), **no line
  9999 subtotals**, and the amount **left out of the document
  `cbc:TaxAmount`**. Any non-zero value produces XML SUNAT rejects.
- The RC emits a 9999 line `cac:TaxTotal` from `TotalOtherTaxes`
  (`internal/xmlbuilder/summary.go`, `buildSummaryLine`).

**What SUNAT requires** (`resources/reglas/reglas-validacion-cpe-2026-08-26.xlsx`)
- 9999 lives per line, next to the line's main tributo. Allowed combinations
  with base > 0: `1000 + 2000 and/or 9999`, `1016 + 9999`, `9995 + 9999`,
  `9996 + 2000 and/or 9999`, `9997 + 2000 and/or 9999`, `9998 + 2000 and/or 9999`.
- Line 9999 amount = rate × base (±1). Line `cbc:TaxAmount` = Σ of
  1000, 1016, 2000, 7152 and 9999 (3292).
- At most one document 9999 subtotal; its base and amount are the Σ of the
  line 9999 bases and amounts (±1).
- Document `cbc:TaxAmount` includes 9999 (3294); total precio de venta includes
  "Sumatoria otros tributos" (3279).
- Forbidden on exportación (operation types 0200–0208; NC/ND motivo 11).
- RC: "Total Otros tributos" per line (9999 `cac:TaxTotal`).

**To build it**
- Per-line otros-tributos rate + amount in `model.LineItem`, the DB
  (`perunio-backend/src/db/schema.ts`, issued document items), the facturador
  routes, and the frontend form.
- Builder: line 9999 subtotal, document subtotal from Σ lines, include in
  line/document `TaxAmount` and in total precio de venta.
- Validation: refuse 9999 on exportación.
- Spec cases: a gravado line with 9999, a line with ISC + 9999, an exportación
  refusal, and an RC line.

## FU-4 — GRE related document: issuer and catálogo 61

**Status:** the related document goes out as `cac:AdditionalDocumentReference`
(F22), with the remitente's own RUC as its issuer.

**Today**
- `model.Despatch` has `relatedDocType/Series/Number` but no issuer, so
  `internal/xmlbuilder/despatch_remitente.go` declares the remitente as the
  issuer of the related document (`cac:IssuerParty`, required for Cat.61 01,
  03, 04, 09, 12, 48, 92: ERROR 3380/3382). Right for a venta (motivo 01);
  wrong when the document was issued by someone else — e.g. the proveedor's
  factura on a compra (motivo 02), which SUNAT also observes (4375).
- Only one related document can be stored; GRE 2.0 allows several.
- **Cat.61** is in `perunio-sunat-catalogs` since v1.4.0 (`sunat.Cat61*`,
  `Cat61GreR` / `Cat61GreT`, `Cat61IssuerRemitente` / `Cat61IssuerTransportista`),
  but nothing reads it yet: the code is taken as given and its name from Cat.01
  where the codes coincide, so a Cat.61-only code (48, 50, 92…) goes without
  `cbc:DocumentType` (OBSERV 4371 only), and 2692 is not pre-checked.
- The GRE workbook ties the related documents to the motivo de traslado
  (3440, 3445, 3493, 3494, 3612, 3463, 3613, 3635 on the remitente) and caps
  how many a transportista lists (3345, 3346); none is pre-checked.

**To build it**
- `related_doc_issuer_ruc` (and a list of related documents) in the backend
  schema, the facturador routes and the guía form.
- Validate the code against `Cat61GreR` / `Cat61GreT` and the issuer against
  the `issuer*` flags; take the name from Cat.61; pre-check the motivo and count
  rules above.
- Spec cases: compra with the proveedor as issuer, two related documents, a
  Cat.61-only code.

## FU-8 — Detracción rules to re-verify

Items from `DETRACCIONES.md` › Open questions that no SUNAT source settles yet.
Re-check each against SUNAT's legislation index, then update the doc and the
catalog. (The Anexo 3 issuer list was settled and the Sector Público buyer
modelled on 2026-10-17.)

- 038, 042, 043, 046, 047 (espectáculos públicos, ladrillos, estructuras
  metálicas, cobre, plata): the workbook's change-log row is struck through,
  and no resolution through R.S. 000204-2026 touches the SPOT (re-checked
  2026-10-17). A trade note dated them 01-09-2026. If they become law, add
  them to
  `perunio-sunat-catalogs/catalogs/cat54.json` with rate, umbral and anexo.
- Which tipo de cambio the S/ 700 test uses on a USD comprobante. Perunio uses
  the comprobante's own. R.S. 183-2004 art. 20 governs only the deposit (SBS
  promedio ponderado venta, earlier of the IGV obligation and the deposit
  date), and no article says how the umbral converts (re-read 2026-10-17).
