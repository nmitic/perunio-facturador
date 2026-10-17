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
