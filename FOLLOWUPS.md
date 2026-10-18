# Follow-ups

Findings that came up while fixing the pending XML spec cases (branch
`fix/xmlbuilder-pending-specs`, plan F1–F22) and were deliberately left out
of that work. Each entry says what is wrong or missing, what SUNAT requires,
and where the code is, so it can be picked up later without re-investigating.

When you pick one up: add spec cases first (`internal/xmlbuilder/spec/`, see its
README), then fix, then delete the entry here.

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

## FU-13 — Drop the legacy `despatches.related_doc_*` columns

**Status:** superseded on 2026-10-17, not yet dropped.

Every guía's documentos relacionados live in `despatch_related_documents`
(perunio-backend migrations 0147 / 0148 copied the single old one there). The
three columns `related_doc_type`, `related_doc_series` and `related_doc_number`
are still read into `model.Despatch` and the frontend `Despatch` type for
display, and the create/update routes still accept them as a one-item list for
an old client. Drop them, and those fields, once no client sends them and
migration 0148 has run in every environment.
