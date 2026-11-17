# Follow-ups

Findings that came up while building the facturador (first while fixing the
pending XML spec cases, plan F1–F22; later the facturador plans and
seguimiento de guías) and were deliberately left out of that work. Each entry says what is wrong or missing, what SUNAT requires,
and where the code is, so it can be picked up later without re-investigating.

When you pick one up: add spec cases first (`internal/xmlbuilder/spec/`, see its
README), then fix, then delete the entry here.

## FU-28 — Seguimiento de guías: try it end to end on a real phone

Everything in seguimiento de guías (facturador plan Empresa, perunio-backend
`services/guia-tracking/`, perunio-frontend `pages/conductor/`,
`components/seguimiento/`) passed type-checks, unit and component tests and
`verify-guia-tracking.ts` against local Postgres, but was never clicked through
in a browser or used on a phone. Before launch, on beta with a real Android
and iPhone:
- the consent tick, then Iniciar → Llegué → Entregar with photos and a signature;
- the camera opening directly (`capture`), the stamp bar on the photo, and
  camera-only refusing a gallery photo (it relies on `lastModified`, which some
  browsers set to the pick time, not the shot time);
- a step taken in airplane mode, then sent when the signal returns (outbox in
  localStorage, photo bytes in IndexedDB; iOS private mode drops to memory);
- the live position every 90 s, and that it stops when the screen locks;
- the maps with `VITE_GOOGLE_MAPS_API_KEY` set (embed on the driver and
  destinatario pages, pin picker and trip map for the emitter);
- the notice emails (Resend) and push, and the Constancia de entrega PDF with
  real photos (size and layout).
Found when the feature was finished (2026-11-17).

## FU-27 — Guía de devolución: confirm SUNAT's rule for refused goods

"Emitir guía de devolución" (rejected or partial trip of a guía remitente,
`buildGuiaDevolucion` in perunio-frontend `src/lib/guia-prefill.ts`) builds a
new guía remitente from the destination back to the company, with the company
as destinatario, citing the original guía (Cat.61 09). Its motivo is "13 Otros"
with "Devolución de bienes no recibidos por el destinatario (guía …)", because
Cat.20 06 Devolución is not among the selectable codes (`CAT20_SELECT_CODES`).
SUNAT's guidance found covers a retorno only for bienes fiscalizados (a "guía
complementaria"); for ordinary goods refused at the door it was not confirmed
whether a new guía is required, which motivo, or whether the original guía
covers the way back. The form shows a notice asking the user to confirm with
their accountant. Settle it against the Reglamento de Comprobantes de Pago
(capítulo V) and R.S. 123-2022, then adjust the prefill and drop or reword the
notice. Found while building phase 5 (2026-11-17).

## FU-26 — Seguimiento: a trip that never starts is never flagged

The `guia-trips-overdue` cron (perunio-backend
`services/notifications/guia-seguimiento.notifier.ts`) only watches trips in
`in_transit` or `arrived`. A guía assigned to a conductor who never taps
"Iniciar traslado" stays `pending` with no warning, which is the case an
emitter most wants to hear about. Add a separate rule: warn once when a
pending trip is older than N hours past the guía's `start_date` (fecha de
inicio de traslado), reusing `overdue_notified_at` and the company's
`notifyIncident` toggle. Found when the feature was finished (2026-11-17).

## FU-25 — Seguimiento: photos of a refused checkpoint stay in R2

The driver's outbox sends a checkpoint's photos before the checkpoint. If the
checkpoint is then refused for good (for example `EVIDENCE_REQUIRED` after a
later rule change, or `TRIP_FINISHED` from a second phone), its photos remain:
rows in `guia_trip_photos` with a `client_event_id` no event has, and their
objects under `documents/…/tracking/`. They are invisible (the timeline groups
photos by event) but cost storage. Sweep them in the `guia-trips-overdue` job:
photos older than a day whose `client_event_id` matches no event of their
trip, deleting the object first, then the row. Found while building phase 4
(2026-11-17).

## FU-24 — Seguimiento: no conductor filter on the guías list

The guías list (perunio-frontend `screens/facturador/GuiasRemision.tsx`) shows
each guía's trip chip with the conductor's name, but cannot filter by
conductor or by trip status. The guías come from this service
(`GET /gre/{companyId}`) and the trips from perunio-backend, so the filter
needs either the backend to return the despatch ids for a conductor/status
(`GET /facturador/seguimiento/:companyId/estados?conductorId=…`) for the
frontend to pass to `/gre` as an id list, or a join here. Planned in phase 3
and left out (2026-11-17).

## FU-23 — Facturador plans are sold and renewed by hand

Facturador plans (perunio-backend `src/config/facturador-plans.ts`, column
`users.facturador_tier`) are set by an admin on the user pages or with
`POST /admin/set-facturador-tier`; "Contratar…" on the plans page opens the
sales WhatsApp. There is no payment, invoice or automatic renewal, and the
admin panel has no bulk page that calls `setFacturadorTier` (the API client
function exists). The prices live in three copies (backend, frontend, home)
plus the allowances in `internal/db/usage.go`; nothing checks they agree.
When facturador billing is decided: a payment flow, and a test (like the
backend's error-codes mirror test) that fails when the copies drift. The
admin panel also has no test setup, so the facturador plan fields there are
covered by type-check only. Found when the plans were added (2026-11-17).

## FU-22 — Seguimiento: the destinatario's page does not link the guía

The destinatario's tracking page (`/seguimiento/$token`) shows the steps and
the position but not the guía itself. The guía's share link
(`/ver-comprobante/$token`, perunio-backend `share.routes.ts`) is a separate
token created at emission, and the trip does not know it. Store the share
token's id on the trip when both exist (or look up the latest share link for
the despatch) and add "Ver la guía" to the page. Planned in phase 6 and left
out (2026-11-17).

## FU-21 — Seguimiento: live position only while the driver page is open

By design, the driver's position is sent only while `/conductor/$token/$tripId`
is open in front (`use-live-position.ts`): browsers do not report location from
the background, so the trail stops when the phone locks or the driver opens
Google Maps for directions. Checkpoints (Iniciar, Llegué, Entregar) always
carry their own position, so the record of the trip is complete; the live
position is not. If customers ask for continuous tracking, the options are a
native app with background location, or reading the GPS the trucks already
carry from the fleet providers' APIs; both were deliberately left out (items
33 and 34 of the brainstorm, 2026-11-17).

## FU-12 — Summaries and voids are not stamped with an environment

`daily_summaries` and `voided_documents` have no `sunat_environment`. Their
issue and poll handlers (`routes_pipeline.go`, `resolvePipelineEnv`) send to
the body override, else to whatever the company is set to *now*. A resumen or
baja created in beta and issued after the company switches to production goes
to SUNAT production, and a ticket obtained in one environment is polled in the
other. Comprobantes and guías are stamped at creation and always go to their
own environment (`stampedPipelineEnv`); these two should follow: add the
column in perunio-backend, stamp it on create, scope the lists, read it on
issue and poll. Found while giving guías a beta (2026-11-16).

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
emission path (2026-10-28). Since the facturador plans (2026-11-17) guía drafts
(`createDespatchHandler`, `routes_gre.go`) check and count the same allowance
the same way, so the fix must cover both.

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
