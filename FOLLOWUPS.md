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

## FU-2 — IGV 10.5 % (MYPE restaurantes / hoteles / alojamientos) end to end

**Status:** the XML builder is done (F15: each gravado line declares the rate
it was taxed at, read from IGV ÷ base, and the document IGV subtotal and the
global-discount / anticipo recalculation follow it). Nothing upstream can
produce a 10.5 % document yet.

**Today**
- Validation checks every gravado line's IGV at the Cat.07 rate, 18 %
  (`internal/validation/rules.go`, `validateIGVTolerance(li,
  cat07RateFraction(...))`), so a 10.5 % draft is refused before it reaches
  the builder.
- Anticipo validation hard-codes `base*1.18` (`internal/validation/rules.go`,
  anticipo block).
- The frontend computes IGV at 18 % (`perunio-frontend/src/lib/factura-decimal.ts`,
  `lib/facturador-utils.ts`, `sunat/cat07.ts`) and has no rate choice.

**What SUNAT requires** (`reglas-validacion-cpe-2026-08-26.xlsx`)
- Line rate 18.00 or 10.50 (3103 / 3279); every IGV line of a document the
  same rate (ERROR 3462).
- The issuer must be in padrón 01 (MYPE restaurantes/hoteles/alojamientos),
  otherwise OBSERV 4439.

**To build it**
- A per-company setting ("IGV reducido 10.5 %", padrón 01) in the backend
  schema and the company settings screen.
- Frontend: compute line IGV at the company's rate; one rate per document.
- Validation: accept 10.5 when the company has it, enforce a single rate per
  document (3462), and use the document rate for anticipos.
- Spec cases: 10.5 with a global discount, 10.5 with an anticipo, a boleta at
  10.5, a mixed-rate document refused.

## FU-3 — Cat.52 leyenda texts missing from perunio-sunat-catalogs

**Status:** worked around with literals in the builder.

**Today**
- `sunat.Cat52Texto` returns `""` for 1002 (transferencia gratuita) and 2007
  (operación sujeta al IVAP) in `perunio-sunat-catalogs` v1.1.0 (only 2006 has
  a Texto). `internal/xmlbuilder/invoice.go` therefore hard-codes
  "TRANSFERENCIA GRATUITA O A TITULO GRATUITO" (1002) and
  "Operación sujeta al IVAP" (2007).
- The 1002 literal does not match the catalog Description ("TRANSFERENCIA
  GRATUITA DE UN BIEN Y/O SERVICIO PRESTADO GRATUITAMENTE"); SUNAT does not
  validate the text, only the code, so it is accepted either way.

**To fix**
- Fill `Texto` for 1002 and 2007 (and any other leyenda the builder may
  inject) in `perunio-sunat-catalogs/catalogs`, regenerate, tag, then
  `go get github.com/nmitic/perunio-sunat-catalogs@vX.Y.Z` here and replace the
  literals with `sunat.Cat52Texto(...)`. `perunio-backend` and
  `perunio-frontend` consume the same tag.
- Re-run `go test ./internal/xmlbuilder -run TestSpec -update` and review the
  leyenda text diffs.
- While there, add what the RC builder needed and the catalog lacks:
  - **Cat.11** (código de tipo de valor de venta, RC `sac:BillingPayment/
    cbc:InstructionID`): 01-05, plus 06-09 from 2027-01-01. Today
    `internal/xmlbuilder/summary.go` writes them as single literals.
  - A **category on Cat.07** entries (gravado / exonerado / inafecto /
    exportación). `FillSummaryTotals` (`internal/xmlbuilder/summary_totals.go`)
    tells exonerado from inafecto gratuitos by the código's first digit
    (2x / 3x).
  - **GRE doc type `EV` must lose its `select` capability.** The facturador
    refuses por evento guías (F8: SEE-SOL only, R.S. 123-2022 numeral 5.2),
    but the frontend guía form builds its type picker from
    `greDocTypeSelectEnum`, which still offers EV
    (`perunio-frontend/src/lib/comprobante-form-utils.ts`). Keep EV valid
    (old records still display; the GRE list filter keeps it).

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
- The shared catalog has no **Cat.61** (GRE documento relacionado). The code is
  taken as given and its name from Cat.01 where the codes coincide; a Cat.61
  code Cat.01 does not have (48, 50, 92…) goes without `cbc:DocumentType`
  (OBSERV 4371 only). XSL rule 2692 checks the code against Cat.61 `gre-r`.

**To build it**
- `related_doc_issuer_ruc` (and a list of related documents) in the backend
  schema, the facturador routes and the guía form.
- Add Cat.61 with its `gre-r` / `gre-t` capabilities to
  `perunio-sunat-catalogs`; validate the code and take the name from it.
- Spec cases: compra with the proveedor as issuer, two related documents, a
  Cat.61-only code.

## FU-5 — Detracción Anexo 1: the per-transport-unit rule

**Status:** not modelled. Rule ★ in `DETRACCIONES.md`.

**Today** `resolveDetraccion` (perunio-frontend) and `validateDetraccionCatalogo`
measure an Anexo 1 código (001, 003, 034, 045) only against ½ UIT of the operación.

**What SUNAT requires** R.S. 183-2004 art. 2.1: an operación of ½ UIT or less is
still subject when the Anexo 1 bienes carried on **one transport unit** add up
to more than ½ UIT. Needs the guía / transport unit to be known at emission.

## FU-6 — Detracción umbral when one operation is split into several comprobantes

**Status:** open question ★. No SUNAT source found addresses it.

**Today** the umbral is measured per comprobante, except for a comprobante that
belongs to a venta con anticipos: there the pipeline fills
`IssueRequest.MontoOperacion` from `ventas_anticipo.monto_acordado`
(`internal/http/pipeline_core.go`), and the form uses `ventaMontoAcordado`.

**To decide** whether two facturas of S/ 400 for one contract are one operación
of S/ 800 (subject) or two (not). If they are one, the user needs a way to link
them outside the venta con anticipos flow.

## FU-7 — Detracción 004 (recursos hidrobiológicos)

**Status:** refused by `validateDetraccion`.

**Missing**
- The per-line `cac:AdditionalItemProperty` entries (Cat.55 3001 matrícula,
  3005 fecha de descarga, 3006 cantidad de especies; faults 3063/3133/3134).
- The rate: 4 % only when the proveedor is on SUNAT's monthly "Listado de
  proveedores sujetos al SPOT con el porcentaje de 4 %"; otherwise 10 %
  (`Cat54PorcentajeAlternativo`). Needs a company setting or a consulta of the
  listado. Validation already accepts either rate.

## FU-8 — Detracción rules to re-verify

Unverified items from `DETRACCIONES.md` › Open questions. Re-check each against
SUNAT (and beta where it is an XML question), then update the doc and the
catalog.

- 038, 042, 043, 046, 047 (espectáculos públicos, ladrillos, estructuras
  metálicas, cobre, plata): a trade note dates them 01-09-2026, the workbook's
  change-log row is struck through, no resolution found. If they become law,
  add them to `perunio-sunat-catalogs/catalogs/cat54.json` with rate, umbral and anexo.
- The Anexo 3 issuer exclusion list (Cartilla vs "Servicios" page).
- Cat.51 1003 pinned to 028 vs a factura for 026 (transporte de personas with
  crédito fiscal), which Perunio emits as 1001.
- Which tipo de cambio the S/ 700 test uses on a USD comprobante (Perunio uses
  the comprobante's own).
- Sector Público buyer receiving a boleta with an Anexo 2/3 código (art. 18 a)
  LIR): today the boleta exclusion always applies.
