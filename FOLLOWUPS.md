# Follow-ups

Findings that came up while fixing the pending XML spec cases (branch
`fix/xmlbuilder-pending-specs`, plan F1–F22) and were deliberately left out
of that work. Each entry says what is wrong or missing, what SUNAT requires,
and where the code is, so it can be picked up later without re-investigating.

When you pick one up: add spec cases first (`internal/xmlbuilder/spec/`, see its
README), then fix, then delete the entry here.

## FU-11 — Exonerado and inafecto anticipos (Cat.53 05/06)

Perunio emits only gravado anticipos: the anticipo line is always Cat.07 10
(`buildAnticipoLine` in perunio-frontend `src/lib/anticipo-line.ts`) and a
regularización deducts them with Cat.53 04 only (`buildAnticipoAllowances`,
`internal/xmlbuilder/invoice.go`). Since 2026-11-03 both sides refuse the rest
instead of emitting a wrong comprobante: the form blocks an anticipo on
exonerado, inafecto, IVAP or export productos (`anticipo-no-gravado` in
`emission-issues.ts`), and `validateAnticipos` refuses anticipos larger than the
sale's gravado-with-IGV base. Sellers of exonerado goods (libros, Amazonía) and
exporters therefore cannot use anticipos at all.

To support them: emit the anticipo line with the sale's own afectación, and on
the regularización deduct each anticipo with the Cat.53 code of its afectación
(04 gravado, 05 exonerado, 06 inafecto) against the matching base, then drop
the gravado-only checks. Add spec cases per code first; confirm each against
SUNAT beta. Exportación anticipos (no IGV, 0200/0201) need their own check of
what SUNAT accepts.

## FU-10 — Partial amortization of an anticipo

A venta con anticipos closes with one factura final (`finalDocumentID` →
regularizada, `internal/db/ventas_anticipo.go`), and an anticipo is either
aplicado or pendiente (`deductsAnticipos`, `internal/db/anticipos.go`), never
partly used. Construction (valorizaciones) and sales delivered in parts deduct
each anticipo partially across several facturas, which Perunio cannot record:
`anticipoSourceError` (`internal/validation/rules.go`) refuses the second
deduction outright.

First confirm in SUNAT beta that a `cac:PrepaidPayment/cbc:PaidAmount` lower
than the anticipo's own total is accepted. If it is, track the monto aplicado
per anticipo (the sum across accepted regularizaciones), refuse only when it
would exceed the anticipo's importe, report the remaining balance, and let a
venta take several finals until its anticipos are used up.

## FU-9 — Document quota is check-then-insert

`createDocumentDraft` (`internal/http/pipeline_core.go`) reads the quota with
`CheckDocumentQuota`, inserts the draft in a separate transaction, then calls
`IncrementDocumentUsage` best-effort (a failure is only logged). Two concurrent
creates at `limit - 1` both pass the check, and a failed increment under-counts
usage for good. Fix by reserving quota inside the `CreateDocumentWithItems`
transaction (conditional `UPDATE … SET used = used + 1 WHERE used < limit
RETURNING`, refusing when no row comes back). Found while auditing the scheduled
emission path (2026-10-28).

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
