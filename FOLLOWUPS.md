# Follow-ups

Findings that came up while fixing the pending XML spec cases (branch
`fix/xmlbuilder-pending-specs`, plan F1–F22) and were deliberately left out
of that work. Each entry says what is wrong or missing, what SUNAT requires,
and where the code is, so it can be picked up later without re-investigating.

When you pick one up: add spec cases first (`internal/xmlbuilder/spec/`, see its
README), then fix, then delete the entry here.

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
