# Detracciones (SPOT)

When a comprobante is subject to detracción, how much, what the XML must carry,
and where each rule lives in Perunio. Read this before touching anything that
resolves, validates or emits a detracción.

The Sistema de Pago de Obligaciones Tributarias (SPOT) is governed by
**R.S. 183-2004/SUNAT** and its amendments. It does not change what the comprobante
charges: the buyer pays part of the price into the seller's own restricted
account at the Banco de la Nación instead of the seller's ordinary account. The
seller can only use that account to pay its own taxes.

> **Why this file exists.** Factura F001-00000008 (S/ 780 of *contabilidad*)
> went out as a plain 0101 sale. Accounting services are Cat.54 **022** at
> **12 %** above S/ 700, so it owed a S/ 93.60 detracción. Its free-text line had
> no código, and the issuance form could only take one from a producto. In
> addition, both apps were pinned to a catalog that still had the 2015 rate (10 %).
> See [Cómo decide Perunio](#cómo-decide-perunio) for what changed.

Rules marked ★ could not be confirmed against a primary SUNAT source; they are
also listed under [Open questions](#open-questions).

---

## Contents

1. [Who deposits, when, and in what currency](#who-deposits-when-and-in-what-currency)
2. [Códigos, rates and umbrales (Cat.54)](#códigos-rates-and-umbrales-cat54)
3. [Cuándo aplica: the umbral](#cuándo-aplica-the-umbral)
4. [Cuándo NO aplica: exclusions](#cuándo-no-aplica-exclusions)
5. [What the XML must carry](#what-the-xml-must-carry)
6. [Cómo decide Perunio](#cómo-decide-perunio)
7. [Where each rule lives](#where-each-rule-lives)
8. [Open questions](#open-questions)
9. [Sources](#sources)

---

## Who deposits, when, and in what currency

| Topic | Rule | Source |
|---|---|---|
| Who deposits | The **buyer / user of the service** (arts. 10 and 15). The **seller** deposits when it is paid in full without a deposit having been made (within 5 business days), for sales through the Bolsa de Productos, and for Anexo 1 goods it delivers itself under ½ UIT per buyer. | RS 183-2004 |
| When — Anexos 2 and 3, carga, inmueble | By the earlier of the (partial or full) payment date and the 5th business day of the month after the comprobante is entered in the Registro de Compras. | RS 183-2004 arts. 11, 16; RS 073-2006 art. 6 |
| When — Anexo 1 | **Before the goods are moved.** | RS 183-2004 art. 6 |
| Rounding | The deposit **may** be made without decimals: drop the decimals if the first is below 5, otherwise round up. Optional, not mandatory. Perunio declares the exact amount to 2 decimals in the XML. | 5ª DF RS 183-2004; 1ª DF RS 073-2006 |
| Foreign currency | The detracción is always declared and deposited in **soles**. Convert at the SBS **promedio ponderado venta** rate on the earlier of the IGV obligation date and the deposit due date (if none is published that day, the last one published). Transporte de carga uses the IGV obligation date only. | RS 183-2004 art. 20; RS 073-2006 art. 10 |
| Neto pendiente de pago | The comprobante's total minus the detracción (and minus IGV retenciones). On a credit sale the cuotas must add up to this figure, not to the total. | RS 193-2020 Anexo 1, field 64-A; SUNAT ERR 3319 |

---

## Códigos, rates and umbrales (Cat.54)

Cat.54 is Anexo 4 of RS 183-2004. The data lives in
`perunio-sunat-catalogs/catalogs/cat54.json` (v1.3.0) as `porcentaje`, `umbral`,
`anexo`, `porcentajeAlternativo` and `operationType`; both apps read it from there.

**Base** for every código except 027 is the *importe de la operación*: the
amount including IGV (for goods, the market price if it is higher). The
**UIT for 2026 is S/ 5,500** (D.S. 301-2025-EF), so **½ UIT = S/ 2,750** — revise
`umbral` for 001, 003, 034 and 045 every January.

"Emit" means the código is offered in the pickers and accepted by the
facturador.

### Anexo 1 — bienes (deposit before the goods move; applies on boletas too)

| Cód | Description | Rate | Umbral | Emit |
|---|---|---|---|---|
| 001 | Azúcar y melaza de caña | 10 % (since 16-10-2017, RS 246-2017) | > ½ UIT | ✓ |
| 003 | Alcohol etílico | 10 % (RS 246-2017) | > ½ UIT | ✓ |
| 034 | Minerales metálicos no auríferos | 10 % (Anexo 1 since 01-04-2025; before that Anexo 2 at any amount) | > ½ UIT | ✓ |
| 045 | Minerales de oro y sus concentrados (2616.90.10.00) | 10 % (since 01-04-2025) | > ½ UIT | ✓ |

An Anexo 1 operación of ½ UIT or less is still subject when the Anexo 1 goods on
one transport unit add up to more than ½ UIT (art. 2.1). **Not modelled** — see
FU-5. Since RS 175-2025, *traslados* of 034 and 045 are excluded.

### Anexo 2 — bienes

| Cód | Description | Rate | Umbral | Emit |
|---|---|---|---|---|
| 004 | Recursos hidrobiológicos | 4 % if the proveedor is on SUNAT's monthly "Listado … 4 %"; otherwise **10 %** (`porcentajeAlternativo`) | > S/ 700 | refused (see below) |
| 005 | Maíz amarillo duro | 4 % | > S/ 700 | ✓ |
| 007 | Caña de azúcar | 10 % (RS 246-2017) | > S/ 700 | ✓ |
| 008 | Madera | 4 % | > S/ 700 | ✓ |
| 009 | Arena y piedra | 10 % | > S/ 700 | ✓ |
| 010 | Residuos, subproductos, desechos, recortes y desperdicios | 15 % | **any amount** | ✓ |
| 011 | Bienes gravados por renuncia a la exoneración | 10 % (since 01-04-2018, RS 082-2018) | > S/ 700 | ✓ |
| 014 | Carnes y despojos comestibles | 4 % | > S/ 700 | ✓ |
| 016 | Aceite de pescado | 10 % (RS 082-2018) | > S/ 700 | ✓ |
| 017 | Harina, polvo y pellets de pescado y demás | 4 % | > S/ 700 | ✓ |
| 023 | Leche cruda entera (only with renuncia a la exoneración) | 4 % (since 16-06-2018, RS 152-2018) | > S/ 700 | ✓ |
| 031 | Oro gravado con el IGV | 10 % | **any amount** | ✓ |
| 032 | Páprika y frutos de los géneros capsicum o pimienta | 10 % (since 01-08-2019, RS 130-2019) | > S/ 700 | ✓ |
| 035 | Bienes exonerados del IGV (Apéndice I-A) | 1.5 % | > S/ 700 | ✓ |
| 036 | Oro y demás minerales metálicos exonerados del IGV | 1.5 % | **any amount** | ✓ |
| 039 | Minerales no metálicos | 10 % | > S/ 700 | ✓ |
| 041 | Plomo | 15 % (RS 130-2019) | > S/ 700 | ✓ |

The "any amount" carve-out is art. 8(a) as amended by RS 249-2012: the S/ 700
exclusion does not apply to numerales 6, 16 and 21 (010, 031, 036). Its
numeral 19 was moved to Anexo 1 as 034 in 2025.

### Anexo 3 — servicios and contratos de construcción (all > S/ 700)

| Cód | Description | Rate | Emit |
|---|---|---|---|
| 012 | Intermediación laboral y tercerización | **12 %** (10 % until 31-03-2018, RS 071-2018) | ✓ |
| 019 | Arrendamiento de bienes (muebles e inmuebles; not leasing) | 10 % | ✓ |
| 020 | Mantenimiento y reparación de bienes muebles | **12 %** (10 % until 31-03-2018) | ✓ |
| 021 | Movimiento de carga | 10 % | ✓ |
| 022 | Otros servicios empresariales (num. 5) — among them actividades jurídicas, and **contabilidad, teneduría de libros, auditoría y asesoramiento tributario** (num. 5 b, CIIU 7412) | **12 %** (10 % until 31-03-2018) | ✓ |
| 024 | Comisión mercantil | 10 % | ✓ |
| 025 | Fabricación de bienes por encargo | 10 % | ✓ |
| 026 | Transporte de personas (with a comprobante that gives crédito fiscal) | 10 % | ✓ |
| 030 | Contratos de construcción | 4 % | ✓ |
| 037 | Demás servicios gravados con el IGV (num. 10, with literal exclusions — see below) | **12 %** (10 % until 31-03-2018) | ✓ |
| 044 | Servicio de beneficio de minerales metálicos | 12 % (since 01-04-2025) | ✓ |

The 12 % rates apply where the IGV obligation arose on or after 01-04-2018.

### Special regimes (outside Anexos 1–3)

| Cód | Description | Rule | Emit |
|---|---|---|---|
| 027 | Transporte de bienes por vía terrestre | **4 % of the greater of the importe and the valor referencial** (DS 010-2006-MTC). Applies when either is **> S/ 400**. RS 073-2006 arts. 2.1, 4. | ✓ (Cat.51 1004) |
| 028 | Transporte público de pasajeros por vía terrestre | A fixed **S/ 2 or S/ 4 per axle**, paid at the toll booth; not a percentage. Excluded when a comprobante giving crédito fiscal is issued — that case is 026. RS 057-2007. | refused |
| 040 | Venta de inmueble gravada con IGV | **4 %, any amount.** When the comprobante gives no crédito fiscal (e.g. a boleta) the **seller** deposits. RS 022-2013. | ✓ |
| 002 | Arroz (IVAP) | 3.85 %, its own regime (RS 266-2004). No rate in the catalog. | not offered |
| 099 | Ley 30737 | 8 % of each payment, to "Categoría 1" subjects. No rate in the catalog. | not offered |

### Not carried

- **013 and 015** (animales vivos; abonos, cueros y pieles) are in Cat.54 but in no current anexo. `anexo: null`, no rate.
- **038, 042, 043, 046, 047** (espectáculos públicos, ladrillos, estructuras metálicas, cobre, plata) appear in SUNAT's Catálogos sheet, but the change-log row that added them is struck through and no resolution adding them to the SPOT was found. Not in the catalog. ★
- 006, 018, 029, 033 were derogated and are gone from Cat.54.

### Códigos the facturador refuses

- **004** — SUNAT demands three per-line `cac:AdditionalItemProperty` entries (Cat.55 3001, 3005, 3006; faults 3063/3133/3134) that the builder does not emit.
- **028** — a fixed amount per vehicle, which `porcentaje × importe` cannot express.

---

## Cuándo aplica: the umbral

- **Strictly greater than.** Anexos 2 and 3 exclude an operation whose importe
  is "igual o menor a S/ 700.00" (arts. 8(a), 13(a)). Anexo 1 requires
  "mayor a media UIT" (art. 2.1); transporte de carga "mayor a S/ 400.00"
  (RS 073-2006 art. 2.1). **S/ 700.00 exactly is not subject.**
- **Measured on the operación, in soles.** The importe is the total the buyer
  pays, including IGV (art. 1 j.1). Foreign-currency comprobantes are converted
  first.
- **A comprobante that is part of a larger operation** — an anticipo, or the
  factura final of a *venta con anticipos* — is measured against the whole
  operación (its `monto acordado`), and each comprobante deposits its own share
  (Informe 103-2013-SUNAT: "sin importar si el saldo final es menor al tope").
  A S/ 360 saldo of a S/ 2,360 operación still deposits 12 % = S/ 43.20.
- **027** compares the greater of the importe and the valor referencial del
  servicio against S/ 400, and uses that same figure as the base.
- **Splitting** one operation into comprobantes of S/ 700 or less: no SUNAT
  source found addresses it. ★ Perunio measures per comprobante unless the
  comprobante belongs to a venta con anticipos.

---

## Cuándo NO aplica: exclusions

### Comprobantes that grant no crédito fiscal — Anexos 2 and 3 only

A **boleta** (or a nota on one) does not support crédito fiscal or tax cost, so
**Anexo 2 and 3 códigos do not apply to it** (arts. 8 and 13). Exception: the
buyer is a Sector Público Nacional entity (art. 18 a) of the Income Tax Law). ★
Perunio does not model that exception, because such buyers receive facturas.

**Anexo 1 is different**: it excludes only the póliza de adjudicación and the
liquidación de compra (art. 3), so it **does** apply on a boleta. The inmueble
(040) and carga (027) regimes also carry no such exclusion.

### Other exclusions

| Exclusion | Applies to | Perunio |
|---|---|---|
| Exportaciones (Cat.51 0200–0208): not operaciones gravadas con el IGV | All | Form: excluded. Facturador: a detracción requires 1001–1004, so an export cannot carry one. |
| Usuario **no domiciliado** | Anexo 3 | Form: excluded for receptor doc 0 / B and Cat.51 0401. |
| No línea gravada con el IGV | Anexo 3 ("servicios gravados") | Form: excluded. |
| Comprobante gives a deduction under art. 46 of the Income Tax Law | Anexo 3 | Not modelled. |
| Liquidación de compra | Anexo 2 | Not issued by Perunio. |
| Issuers: AFPs, centros de inspección técnica vehicular, rail passenger transport, domestic water freight, domestic air cargo, non-scheduled air passenger transport. ★ The Cartilla adds EPS, financial entities, cooperativas, and the Iglesia Católica for leases. | Anexo 3 | Not modelled. Such a company simply sets no default código, and answers "No está sujeta". |
| **Anexo 3 numeral 10 literal exclusions** (037 only): entities of the financial and insurance system; ESSALUD; ONP; restaurants and bars; hospedaje; postal and express delivery; carga terrestre (→ 027); public passenger transport (→ 028); electricity; PERUPETRO hydrocarbon contracts; clearing houses (CAVALI); port and airport operators; espectáculos públicos. | 037 | Not modelled: the user does not pick 037 for these. |
| Amazonía-exonerated sales: only numeral 21 b) goods are subject (e.g. Amazonía café is not; Informe 103-2013) | Anexo 2 | Not modelled. |

**Buyer without RUC:** there is no express exclusion. Such a buyer normally gets a
boleta, which triggers the crédito-fiscal exclusion except for Anexo 1. The
facturador requires an identified buyer (DNI or RUC) on any comprobante with a
detracción, because the deposit is filed under the buyer.

---

## What the XML must carry

From `resources/reglas/reglas-validacion-cpe-2026-08-26.xlsx` (Factura2_0; the
Boleta2_0 and NotaDebito2_0 sheets carry the same rules; **NotaCredito2_0 carries
none**).

| Element | Rule | SUNAT code |
|---|---|---|
| `cbc:ProfileID` and `cbc:InvoiceTypeCode/@listID` | Cat.51: 1001 general, 1002 ↔ 004, 1003 ↔ 028, 1004 ↔ 027. Any other pairing for the three pinned códigos is rejected. | ERR 3129 |
| `cbc:Note[@languageLocaleID='2006']` | "Operación sujeta a detracción". Missing → observation, not rejection. Duplicate → error. | OBS 4265, ERR 3014 |
| `cac:PaymentMeans` with `cbc:ID` = `Detraccion` | `cac:PayeeFinancialAccount/cbc:ID` = the Banco de la Nación account. `cbc:PaymentMeansCode` must be in Cat.59. | ERR 3034, 3174 |
| `cac:PaymentTerms` with `cbc:ID` = `Detraccion` | `cbc:PaymentMeansID` = the Cat.54 código; `cbc:PaymentPercent`; `cbc:Amount` in **PEN**, 2 decimals. | ERR 3127, 3033, 3035, 3037, 3208 |
| `cac:PaymentTerms` "Detraccion" on a non-detracción operation | Rejected. | ERR 3128 |
| Forma de pago crédito | `cac:PaymentTerms` "Credito" amount = neto pendiente (total − detracción); Σ cuotas = that neto; neither may exceed `PayableAmount`. | ERR 3319, 3265, 3266 |
| 1004 (027) | Per-line `cac:Delivery`: origin and destination ubigeo and address, trip detail, three valores referenciales (01 service, 02 carga efectiva, 03 carga útil), all in PEN. | ERR 3116–3126, 3208 |
| 1002 (004) | Per-line `cac:AdditionalItemProperty` (Cat.55 3001–3006). | ERR 3063, 3130–3135 |

`cbc:PaymentPercent` is **not validated** by SUNAT, and no threshold rule exists
in the XSL: SUNAT accepts a detracción below the umbral, and a 0101 factura that
should have carried one. The validation in Perunio is what catches both.

Spec cases: `internal/xmlbuilder/spec/invoice/24`–`33`, `boleta/04`,
`creditnote/06`, `debitnote/03`. `invoice/33-detraccion-servicio-contabilidad` is
F001-00000008 as it should have gone out.

---

## Cómo decide Perunio

Detracción is a **document-level** decision: one código, one porcentaje, one
monto per comprobante. The issuance form (factura and boleta) shows it in
**Cobro › "¿Sujeto a detracción?"** (`SimpleDetraccion`), with one line saying
which rule decided.

### Resolution order (`resolveDetraccion`)

1. **Switched off** by the user → none, and the comprobante is emitted with
   `detraccionNoSujeta: true` (stored on `issued_documents.detraccion_no_sujeta`).
2. **The código**, first match wins:
   1. picked by hand in the section;
   2. the productos' código (`productos.detraccion_codigo`, copied onto the line).
      More than one distinct código is a **conflict** and blocks emission;
   3. the **company default** (`companies.detraccion_codigo_default`, set in
      Empresa › Detracción), when no line carries a código of its own.
3. **Exclusions**: exportación; Anexo 2/3 on a boleta (or a nota on one); Anexo 3
   for a no domiciliado or with no línea gravada con IGV.
4. **The umbral** of the código, strictly greater than, over the operación in
   soles (027: the greater of the importe and the valor referencial). Notas skip
   it: a partial nota mirrors the referenced comprobante.
5. **The monto**: porcentaje × importe of this comprobante in soles (027: the
   greater of it and the valor referencial), rounded to céntimos.

A código picked by hand **does not bypass** steps 3 and 4: they are the law, and
the facturador rejects a detracción that breaks them.

### The pre-emit question

A **factura** whose operación is over S/ 700 and that has **no código at all** —
none from a producto, no company default, none picked — asks before emitting:

> **Factura mayor a S/ 700 sin detracción.** ¿Es un bien o servicio sujeto al SPOT?
> *Marcar detracción* · *No está sujeta, emitir* · *Cancelar*

"No está sujeta" emits with `detraccionNoSujeta: true`. Boletas are not asked:
Anexos 2 and 3 never apply to them, and an Anexo 1 bien always arrives with its
producto's código. Exportaciones are not asked.

### What the facturador enforces

`validateDetraccion` / `validateDetraccionCatalogo` in
`internal/validation/rules.go` reject, before anything reaches SUNAT:

- a código that is not `emit` in Cat.54 (pre-empts ERR 3033), or 004 / 028;
- a porcentaje other than the catálogo's (004 may also declare its alternative 10 %);
- an Anexo 2/3 código on a boleta or on a nota referencing one;
- an operación at or under the código's umbral — measured on
  `TaxInclusiveAmount`, or the venta's `monto_acordado` when the comprobante
  belongs to one (filled by the pipeline, never by the client). Notas and 0104
  anticipos are not measured;
- a monto that is not porcentaje × the SPOT base in soles (±0.01), for any
  currency (027: the greater of the importe and the valor referencial);
- a missing cuenta BN, a missing tipo de cambio on a foreign-currency document,
  and an incomplete 027 trip block.

It does **not** detect a factura that should have carried a detracción but has
none: from the XML alone it cannot know what was sold. That is the form's job,
via the section and the pre-emit question.

---

## Where each rule lives

| Rule | Where |
|---|---|
| Rates, umbrales, anexo, operation type per código | `perunio-sunat-catalogs/catalogs/cat54.json` → `sunat.Cat54*` (Go), `sunat-catalogs/cat54` (TS) |
| Resolution in the form | `perunio-frontend/src/lib/facturador-utils.ts` › `resolveDetraccion` |
| The section and the reason line | `perunio-frontend/src/components/facturador/SimpleDetraccion.tsx` |
| Pre-emit question | `perunio-frontend/src/hooks/use-comprobante-form.ts` › `runSubmit`; `use-detraccion-confirm.ts`; `DetraccionConfirmDialog.tsx` |
| Company default código | `perunio-frontend/src/components/company-detraccion-tab.tsx`; `perunio-backend` `companies.detraccion_codigo_default` |
| Notas | `perunio-frontend/src/hooks/use-nota-form.ts` (mirrors the reference, no umbral) |
| Server validation | `perunio-facturador/internal/validation/rules.go` › `validateDetraccion`, `validateDetraccionCatalogo`, `validateTransporteCarga` |
| XML | `perunio-facturador/internal/xmlbuilder/common.go` › `buildDetraccionPaymentMeans`, `buildDetraccionPaymentTerms`, `appendDetraccionLegend`, `sunatOperationType` |
| Neto pendiente | `model.NetoPendientePago` (Go), `netoPendientePago` (TS) |
| DB columns | `perunio-backend/src/db/schema.ts`: `companies.cuenta_detraccion`, `companies.detraccion_codigo_default`, `issued_documents.detraccion_*`, `issued_documents.detraccion_no_sujeta`, `productos.detraccion_codigo` |

### Tests

- `perunio-facturador`: `go test ./internal/validation -run 'TestValidateDetraccion|TestValidateTransporteCarga'`;
  `go test ./internal/xmlbuilder -run TestSpec/invoice/33`.
- `perunio-frontend`: `npm run check:resolve-detraccion` (table test of `resolveDetraccion`).

A case that resolves a detracción in the frontend table must pass the Go
validator, and vice versa.

---

## Open questions

Each one is also an entry in `FOLLOWUPS.md`.

| ★ | Question | FU |
|---|---|---|
| 1 | Anexo 1: an operación ≤ ½ UIT is still subject when one transport unit carries > ½ UIT of Anexo 1 goods. Not modelled. | FU-5 |
| 2 | Splitting one operation into comprobantes ≤ S/ 700 — no SUNAT source addresses it. Perunio measures per comprobante outside ventas con anticipos. | FU-6 |
| 3 | 004 at 4 % vs 10 % depends on SUNAT's monthly listado; 004 is refused until the per-line data is built anyway. | FU-7 |
| 4 | Status of 038, 042, 043, 046, 047 (one trade note says 01-09-2026; no resolution found). | FU-8 |
| 5 | The Anexo 3 issuer exclusion list differs between the Cartilla and the "Servicios" page. | FU-8 |
| 6 | Cat.51 1003 is pinned to 028; a factura for 026 (transporte de personas con crédito fiscal) is 1001. Unverified against beta. | FU-8 |
| 7 | Which exchange rate the S/ 700 test uses on a USD comprobante (Perunio uses the comprobante's own tipo de cambio). | FU-8 |

---

## Sources

- R.S. 183-2004/SUNAT (consolidated text is out of date after 2008; read with the amendments): https://www.sunat.gob.pe/legislacion/superin/2004/183.htm
- Amendments: R.S. 246-2017 (001, 003, 007 to 10 %), **R.S. 071-2018** (012, 020, 022, 037 to 12 % from 01-04-2018), R.S. 082-2018 (011, 016), R.S. 152-2018 (023), R.S. 130-2019 (032, 041), R.S. 249-2012 (the any-amount numerales), **R.S. 000086-2025** (044, 045; 034 to Anexo 1, from 01-04-2025): https://www.sunat.gob.pe/legislacion/superin/2025/000086-2025.pdf, R.S. 000121-2025 and **000175-2025** (traslados of 034/045).
- Transporte de bienes: R.S. 073-2006/SUNAT, https://www.sunat.gob.pe/legislacion/superin/2006/073.htm
- Transporte de pasajeros: R.S. 057-2007/SUNAT. Inmuebles: R.S. 022-2013/SUNAT. Arroz (IVAP): R.S. 266-2004/SUNAT.
- SUNAT orientación: "En la venta de bienes" https://orientacion.sunat.gob.pe/3141-02-en-la-venta-de-bienes-empresas ; "En la prestación de servicios" https://orientacion.sunat.gob.pe/3142-03-en-la-prestacion-de-servicios-empresas ; Apéndices https://orientacion.sunat.gob.pe/apendices-del-sistema-de-detracciones ; Cartilla https://orientacion.sunat.gob.pe/sites/default/files/inline-files/Cartilla_detracciones.pdf
- Informe N.° 103-2013-SUNAT (anticipos; Amazonía).
- UIT 2026: D.S. 301-2025-EF.
- Validation workbook: `resources/reglas/reglas-validacion-cpe-2026-08-26.xlsx` (Factura2_0, Boleta2_0, Catálogos, Control de Cambios).
- Research for this file was done on 2026-10-17. cpe.sunat.gob.pe returned 403 throughout, so its "factura con detracción" page was read only through a search snippet.
