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

## FU-5 — Detracción Anexo 1: the per-transport-unit rule

**Status:** not modelled. The rule is settled law (R.S. 183-2004 art. 2.1);
listed under `DETRACCIONES.md` › Known gaps.

**Today** `resolveDetraccion` (perunio-frontend) and `validateDetraccionCatalogo`
measure an Anexo 1 código (001, 003, 034, 045) only against ½ UIT of the operación.

**What SUNAT requires** R.S. 183-2004 art. 2.1: an operación of ½ UIT or less is
still subject when the Anexo 1 bienes carried on **one transport unit** add up
to more than ½ UIT. Needs the guía / transport unit to be known at emission.

## FU-6 — Detracción umbral for an operación spread over several comprobantes

**Status:** two open questions ★. No SUNAT source found answers either.

**Today** the umbral is measured per comprobante, except for a comprobante that
belongs to a venta con anticipos: there the pipeline fills
`IssueRequest.MontoOperacion` from `ventas_anticipo.monto_acordado`
(`internal/http/pipeline_core.go`), and the form uses `ventaMontoAcordado`.

**To decide**
- Venta con anticipos: is the umbral measured against the whole operación, as
  Perunio does, or against each comprobante? Informe 103-2013-SUNAT, which this
  was attributed to, only says that each comprobante deposits on its own
  importe (the anticipo and the saldo); it never mentions the umbral. Perunio's
  reading is the conservative one: it can produce a detracción that was not
  owed, never omit one that was.
- Splitting: whether two facturas of S/ 400 for one contract are one operación
  of S/ 800 (subject) or two (not). If they are one, the user needs a way to
  link them outside the venta con anticipos flow.

## FU-7 — Detracción 004 (recursos hidrobiológicos)

**Status:** refused by `validateDetraccion`. Since perunio-sunat-catalogs
v1.3.1 Cat.54 grants 004 only `emit`, so the pickers no longer offer it; a
producto saved with it still resolves (`detraccionItem`) and is refused by name
at emission. Building 004 means restoring its `select` too.

**Missing**
- The per-line `cac:AdditionalItemProperty` entries. Operation type 1002
  requires all six Cat.55 concepts (Factura2_0 and Boleta2_0; none struck
  through): 3001 matrícula de la embarcación (ERR 3063), 3002 nombre de la
  embarcación (3130), 3003 tipo de especie vendida (3131), 3004 lugar de
  descarga (3132), 3005 fecha de descarga (3134), 3006 cantidad de especie
  vendida (3133; 3135 without its quantity).
- The rate: 4 % only when the proveedor holds the vessel's permiso de pesca and
  is on SUNAT's "Listado de proveedores sujetos al SPOT con el porcentaje de
  4 %". The listado is published by the last business day of each month and
  applies from the next; the one in force on the deposit date decides.
  Otherwise 10 % (`Cat54PorcentajeAlternativo`). Needs a company setting or a
  consulta of the listado. Validation already accepts either rate.

## FU-8 — Detracción rules to re-verify or model

Items from `DETRACCIONES.md` › Open questions and Known gaps. Re-check each
against SUNAT (and beta where it is an XML question), then update the doc and
the catalog.

- 038, 042, 043, 046, 047 (espectáculos públicos, ladrillos, estructuras
  metálicas, cobre, plata): the workbook's change-log row is struck through,
  and no resolution through R.S. 000204-2026 (14-10-2026) touches the SPOT. A
  trade note dated them 01-09-2026. If they become law, add them to
  `perunio-sunat-catalogs/catalogs/cat54.json` with rate, umbral and anexo.
- The Anexo 3 issuer list (R.S. 183-2004 art. 13 c): the orientación
  "Servicios" page lists six issuers; the Cartilla (2019–2020) adds EPS,
  financial and insurance entities and cooperativas, and the Iglesia Católica
  for leases. Find the amending resolution to settle which list is current.
- Which tipo de cambio the S/ 700 test uses on a USD comprobante. Perunio uses
  the comprobante's own; art. 20 governs only the deposit.
- **Confirmed, not modelled:** a Sector Público buyer receiving a boleta with
  an Anexo 2/3 or 027 código. Arts. 8 and 13 (and R.S. 073-2006 art. 3) say the
  boleta exclusion "no opera" for an entity of art. 18 a) LIR; today it always
  applies.
