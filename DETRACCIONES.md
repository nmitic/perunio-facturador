# Detracciones (SPOT)

When a comprobante is subject to detracción, how much, what the XML must carry,
and where each rule lives in Perunio. Read this before touching anything that
resolves, validates or emits a detracción.

The Sistema de Pago de Obligaciones Tributarias (SPOT) rests on the TUO of
D.Leg. 940 and is governed by **R.S. 183-2004/SUNAT** and its amendments, with
separate resolutions for transporte de bienes, transporte público de pasajeros
and the IVAP. It does not change what the comprobante charges: the buyer pays
part of the price into the seller's own restricted account at the Banco de la
Nación instead of the seller's ordinary account. The seller can only use that
account to pay its own taxes.

> **Why this file exists.** Factura F001-00000008 (S/ 780 of *contabilidad*)
> went out as a plain 0101 sale. Accounting services are Cat.54 **022** at
> **12 %** above S/ 700, so it owed a S/ 93.60 detracción. Its free-text line had
> no código, and the issuance form could only take one from a producto. In
> addition, both apps were pinned to a catalog that still had the 2015 rate (10 %).
> See [Cómo decide Perunio](#cómo-decide-perunio) for what changed.

Every rule here was checked against the primary text on **2026-10-17** (see
[Sources](#sources)). Rules marked ★ have no primary SUNAT source, or SUNAT's
own publications disagree; they are listed under [Open questions](#open-questions).
Rules marked ◇ have no primary source either, but Perunio has settled on a
reading (the conservative one) and does not expect to revisit it without one.
What Perunio knowingly does not model is listed under [Known gaps](#known-gaps).

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
9. [Known gaps](#known-gaps)
10. [Sources](#sources)

---

## Who deposits, when, and in what currency

### Who deposits

The obligated party is normally the **buyer / user of the service**. The seller
deposits instead in the cases below. (Retiros and traslados have their own
obligated parties, arts. 5.2–5.3; they never go through Perunio's issuance.)

| Regime | The seller deposits instead when | Source |
|---|---|---|
| Anexo 1 | (a) it carries and delivers goods whose importe per buyer is ½ UIT or less and the SPOT still applies (the per-transport-unit rule, art. 5.1 b.1: Perunio's `depositaEmisor` / `DetraccionDepositaEmisor`); (b) it receives the whole importe without the deposit having been made; (c) the sale goes through the Bolsa de Productos | RS 183-2004 art. 5 |
| Anexo 2 | (b) or (c) | art. 10 |
| Anexo 3 | (b) | art. 15 |
| 027 transporte de carga | (b) | RS 073-2006 art. 5 |
| 040 inmueble | the comprobante supports neither crédito fiscal nor gasto/costo (e.g. a boleta to a consumer); or (b) | RS 022-2013 (art. 10.3 of RS 183-2004) |
| 028 transporte público de pasajeros | always: the **carrier** pays at each toll booth | RS 057-2007 art. 5 |

### When

| Who, and for what | Deadline | Source |
|---|---|---|
| Anexo 1 | **Before the goods are moved** (a retiro: the earlier of the retiro and the comprobante) | RS 183-2004 art. 6 |
| Buyer: Anexos 2 and 3, carga, inmueble | The earlier of the (partial or full) payment and the 5th business day of the month after the comprobante is entered in the Registro de Compras | arts. 11.1, 11.3 a), 16; RS 073-2006 art. 6 |
| Seller paid in full without the deposit | Within 5 business days of receiving the whole importe | arts. 11.1, 11.3 c), 16; RS 073-2006 art. 6 |
| Seller of an inmueble on a comprobante without crédito fiscal | By the date of the (partial or full) payment | art. 11.3 b) |
| Bolsa de Productos | By the date the Bolsa pays the seller | art. 11.1 |

### Amounts and currency

| Topic | Rule | Source |
|---|---|---|
| Rounding | The deposit **may** be made without decimals: drop them if the first is below 5, otherwise round up. Optional, not mandatory. Perunio declares the exact amount to 2 decimals in the XML. | 5ª DF RS 183-2004 (added by RS 178-2005); 1ª DF RS 073-2006 |
| Foreign currency | The detracción is always declared and deposited in **soles**. Convert at the SBS **promedio ponderado venta** rate of the earlier of the IGV obligation date and the date the deposit is due (if none is published that day, the last one published). Transporte de carga uses the IGV obligation date only. | RS 183-2004 art. 20; RS 073-2006 art. 10 |
| Neto pendiente de pago | What a credit sale still owes the seller: the total minus IGV retenciones, minus the deposit **the buyer** must make, minus other deductions. The cuotas must add up to this figure, not to the total. When the seller is the obligated party (040 on a comprobante without crédito fiscal, above), the detracción does not come off: the buyer pays the whole total. Perunio handles that case (`DetraccionDepositaEmisor`; spec `boleta/06-detraccion-inmueble-credito`). The seller's other cases (paid in full without the deposit) arise after emission, so the comprobante cannot know them. | RS 193-2020 (items 64-A/64-B of Anexo 1 of RS 097-2012), in force 01-04-2021 |

---

## Códigos, rates and umbrales (Cat.54)

Cat.54 is Anexo 4 of RS 183-2004, the list of códigos (RS 000086-2025 art. 4
added 044 and 045 to it). The data lives in
`perunio-sunat-catalogs/catalogs/cat54.json` (v1.3.1) as `porcentaje`, `umbral`,
`anexo`, `porcentajeAlternativo` and `operationType`. perunio-facturador,
perunio-frontend and perunio-backend all pin that tag.

**Base** for every código except 027 is the *importe de la operación*: the valor
de venta plus IGV, and for bienes muebles the market price if it is higher
(art. 1 j.1). The **UIT for 2026 is S/ 5,500** (D.S. 301-2025-EF), so
**½ UIT = S/ 2,750**. The UIT that counts is the one in force at the earlier of
the start of the traslado and the IGV obligation date (art. 2.1). `umbral` for
001, 003, 034 and 045 holds a single figure, so a sale that straddles New Year
can need the previous year's: revise it every January.

The **Perunio** column says what the issuance form and the facturador do with
the código:

- **✓**: the picker (`DetraccionCodigoField`) offers it and the facturador emits it.
- **refused**: the catalog grants it `emit` but not `select`, so the picker does not offer it, and the facturador refuses it at emission.
- **—**: neither.

### Anexo 1 — bienes (deposit before the goods move; applies on boletas too)

| Num. | Cód | Description | Rate | Umbral | Perunio |
|---|---|---|---|---|---|
| 1 | 001 | Azúcar y melaza de caña | 10 % (since 16-10-2017, RS 246-2017) | > ½ UIT | ✓ |
| 2 | 003 | Alcohol etílico | 10 % (since 16-10-2017, RS 246-2017) | > ½ UIT | ✓ |
| 3 | 045 | Minerales de oro y sus concentrados (2616.90.10.00) | 10 % (since 01-04-2025) | > ½ UIT | ✓ |
| 4 | 034 | Minerales metálicos no auríferos (Cap. 26, except 2616.90.10.00) | 10 % (Anexo 1 since 01-04-2025; before that Anexo 2 num. 19, at any amount) | > ½ UIT | ✓ |

RS 343-2014 took 001, 003 and 007 out of the SPOT from 01-01-2015. RS 246-2017
brought them back at 10 %. RS 000086-2025 moved 034 and 045 here for operations
whose IGV obligation arose from 01-04-2025.

An Anexo 1 operación of ½ UIT or less is still subject when the Anexo 1 goods on
one transport unit add up to more than ½ UIT (art. 2.1), and the proveedor, who
carries and delivers them, makes the deposit (art. 5.1 b.1). Perunio cannot see
the transport unit, so it asks ([The pre-emit questions](#the-pre-emit-question)).
*Traslados* of 034 and 045 that do not originate in a sale
(art. 2.1 c) are excluded from the SPOT. That is the 2ª DCF of RS 000086-2025,
added by RS 000121-2025 and reworded by RS 000175-2025.

### Anexo 2 — bienes

| Num. | Cód | Description | Rate | Umbral | Perunio |
|---|---|---|---|---|---|
| 1 | 004 | Recursos hidrobiológicos | 4 % if the proveedor holds the vessel's permiso de pesca **and** is on SUNAT's listado (below); otherwise **10 %** (`porcentajeAlternativo`) | > S/ 700 | ✓ (Cat.51 1002) |
| 2 | 005 | Maíz amarillo duro | 4 % | > S/ 700 | ✓ |
| 4 | 007 | Caña de azúcar | 10 % (since 16-10-2017, RS 246-2017) | > S/ 700 | ✓ |
| 5 | 009 | Arena y piedra | 10 % | > S/ 700 | ✓ |
| 6 | 010 | Residuos, subproductos, desechos, recortes y desperdicios | 15 % | **any amount** | ✓ |
| 7 | 011 | Bienes gravados con el IGV por renuncia a la exoneración | 10 % (since 01-04-2018, RS 082-2018) | > S/ 700 | ✓ |
| 9 | 014 | Carnes y despojos comestibles | 4 % | > S/ 700 | ✓ |
| 11 | 016 | Aceite de pescado | 10 % (since 01-04-2018, RS 082-2018) | > S/ 700 | ✓ |
| 12 | 017 | Harina, polvo y pellets de pescado y demás | 4 % | > S/ 700 | ✓ |
| 14 | 023 | Leche cruda entera (only with renuncia a la exoneración) | 4 % (since 16-06-2018, RS 152-2018) | > S/ 700 | ✓ |
| 15 | 008 | Madera | 4 % | > S/ 700 | ✓ |
| 16 | 031 | Oro gravado con el IGV | 10 % | **any amount** | ✓ |
| 17 | 032 | Páprika y frutos de los géneros capsicum o pimienta | 10 % (since 01-08-2019, RS 130-2019) | > S/ 700 | ✓ |
| 20 | 035 | Bienes exonerados del IGV (Apéndice I-A) | 1.5 % | > S/ 700 | ✓ |
| 21 | 036 | Oro y demás minerales metálicos exonerados del IGV | 1.5 % | **any amount** | ✓ |
| 22 | 039 | Minerales no metálicos | 10 % | > S/ 700 | ✓ |
| 23 | 041 | Plomo | 15 % (since 01-08-2019, RS 130-2019) | > S/ 700 | ✓ |

**The "any amount" carve-out.** As SUNAT publishes art. 8 today (Cartilla;
orientación page "En la venta de bienes"), the S/ 700 exclusion does not apply to
the bienes of numerales 6, 16, 19 and 21 (010, 031, 036). Numeral 19 was
derogated in 2025, when 034 moved to Anexo 1. RS 000086-2025 also narrowed
numeral 16, sending gold ores (2616.90.10.00) to Anexo 1 as 045.

**004's listado** is the "Listado de proveedores sujetos al SPOT con el
porcentaje de 4 %". SUNAT publishes it by the last business day of each month,
and it applies from the first day of the next. The listado in force on the date
the deposit is due decides the rate. Perunio cannot read it, so the company
setting "listado de 4 %" (`companies.listado_spot_4`) only sets the rate the
form starts from, and the user can switch it per comprobante.

**004 on the wire.** Operation type 1002 demands six Cat.55 concepts as
`cac:AdditionalItemProperty` on **every** line (SUNAT runs them in the line
template). Perunio asks the embarcación and the descarga once and repeats them,
and asks the especie and its cantidad per line:

| Concept | Where it comes from | Fault if missing |
|---|---|---|
| 3001 matrícula de la embarcación (an..15) | `hidrobiologicos.matriculaEmbarcacion` | ERR 3063 |
| 3002 nombre de la embarcación (an..100) | `hidrobiologicos.nombreEmbarcacion` | 3130 |
| 3003 tipo de especie vendida (an..150) | the line's `especieVendida` | 3131 |
| 3004 lugar de descarga (an..100) | `hidrobiologicos.lugarDescarga` | 3132 |
| 3005 fecha de descarga (`cac:UsabilityPeriod/cbc:StartDate`) | `hidrobiologicos.fechaDescarga` | 3134 (3065) |
| 3006 cantidad de especie vendida (`cbc:ValueQuantity`, TNE) | the line's `cantidadEspecie` | 3133 (3135; 3115 for a unit other than TNE) |

### Anexo 3 — servicios and contratos de construcción (all > S/ 700)

| Num. | Cód | Description | Rate | Perunio |
|---|---|---|---|---|
| 1 | 012 | Intermediación laboral y tercerización | **12 %** (10 % until 31-03-2018) | ✓ |
| 2 | 019 | Arrendamiento de bienes: muebles **and inmuebles**, though Cat.54's label says only "muebles"; not leasing | 10 % | ✓ |
| 3 | 020 | Mantenimiento y reparación de bienes muebles | **12 %** (10 % until 31-03-2018) | ✓ |
| 4 | 021 | Movimiento de carga | 10 % | ✓ |
| 5 | 022 | Otros servicios empresariales: CIIU a) 7411 jurídicas, **b) 7412 contabilidad, teneduría de libros y auditoría; asesoramiento en materia de impuestos**, c) 7413 to i) 7495 | **12 %** (10 % until 31-03-2018) | ✓ |
| 6 | 024 | Comisión mercantil | 10 % | ✓ |
| 7 | 025 | Fabricación de bienes por encargo | 10 % | ✓ |
| 8 | 026 | Transporte de personas (with a comprobante that gives crédito fiscal) | 10 % | ✓ |
| 9 | 030 | Contratos de construcción | 4 % | ✓ |
| 10 | 037 | Demás servicios gravados con el IGV (with literal exclusions, see below) | **12 %** (10 % until 31-03-2018) | ✓ |
| 11 | 044 | Servicio de beneficio de minerales metálicos | 12 % (since 01-04-2025) | ✓ |

The 12 % rates come from RS 071-2018 (numerales 1, 3, 5 and 10). They apply where
the IGV obligation arose on or after 01-04-2018.

### Special regimes (outside Anexos 1–3)

| Cód | Description | Rule | Perunio |
|---|---|---|---|
| 027 | Transporte de bienes por vía terrestre | **4 % of the greater of the importe and the valor referencial** (D.S. 010-2006-MTC). Applies when either is **> S/ 400**. Movimiento de carga billed on the same comprobante, and subcontracted legs, are covered too. RS 073-2006 arts. 2.1, 4. Excluded on a comprobante without crédito fiscal and for a no-domiciliado user (art. 3). | ✓ (Cat.51 1004) |
| 028 | Transporte público de pasajeros por vía terrestre | A fixed **S/ 2 per axle** at toll booths that charge both ways, **S/ 4** at those that charge one way. The carrier pays it at the booth; it is not a percentage of a comprobante. A service billed with a comprobante that gives crédito fiscal is 026 instead. RS 057-2007 arts. 3–5. | — (refused) |
| 040 | Venta de inmueble gravada con IGV | **4 %, any amount**, with no boleta exclusion. Who deposits depends on the comprobante (see above). Art. 7.2 of RS 183-2004, added by RS 022-2013. | ✓ |
| 002 | Arroz pilado (IVAP) | 3.85 %, its own regime (RS 266-2004 art. 4). No rate in the catalog. | — |
| 099 | Ley 30737 | Public entities and third-category taxpayers paying a "Categoría 1" subject (MINJUSDH's list, updated monthly) withhold **8 % of each payment, IGV included**, into a separate BN account. While a company is in Categoría 1, none of its operations carry the ordinary SPOT. No rate in the catalog. | — |

### Not carried

- **013 and 015** (animales vivos; abonos, cueros y pieles) are in Cat.54 but in no current anexo. In the catalog: `anexo: null`, no rate.
- **038, 042, 043, 046, 047** (espectáculos públicos, ladrillos, estructuras metálicas, cobre, plata) appear in the workbook's Catálogos sheet. But the Control de Cambios row that added them is struck through, and no resolution puts them in the SPOT: none of the 204 resolutions in SUNAT's 2026 index (to 14-10-2026) touches it. They are not in the catalog. ★
- **006, 018, 029, 033** were derogated and are gone from Cat.54.

### Códigos the facturador refuses

- **028.** A fixed amount per axle collected at the toll booth, which `porcentaje × importe` cannot express. The picker does not offer it.

---

## Cuándo aplica: the umbral

- **Strictly greater than.** Anexos 2 and 3 exclude an operation whose importe
  is "igual o menor a S/ 700.00" (arts. 8 and 13). Anexo 1 requires an importe
  "mayor a media (1/2) UIT" (art. 2.1); transporte de carga "mayor a S/ 400.00"
  (RS 073-2006 art. 2.1). **S/ 700.00 exactly is not subject.**
- **Measured on the operación, in soles.** The importe is the total the buyer
  pays, including IGV (art. 1 j.1). Foreign-currency comprobantes are converted
  first; which rate the test itself should use is open (★, [Open questions](#open-questions)).
- **Anticipos.** Each comprobante deposits on its own importe: the anticipo on
  what it collected, the factura final on the saldo. That much is sourced:
  Informe 103-2013-SUNAT concludes that the detracción "deberá aplicarse sobre el
  importe consignado en cada comprobante de pago", for the advance and the saldo
  alike.
- **Measuring the umbral of a venta con anticipos.** No SUNAT source says whether
  the umbral is measured against the whole operación (its `monto acordado`) or
  against each comprobante. ◇ **Perunio measures the whole operación**, so a
  S/ 360 saldo of a S/ 2,360 operación still deposits 12 % = S/ 43.20. That is
  the conservative reading: it can produce a detracción that was not owed, never
  omit one that was. Decided 2026-10-17; the monto acordado comes from
  `ventas_anticipo.monto_acordado` (`IssueRequest.MontoOperacion`, filled by the
  pipeline in `internal/http/pipeline_core.go`).
- **027** compares the greater of the importe and the valor referencial del
  servicio against S/ 400, and uses that same figure as the base.
- **Splitting** one operation into comprobantes of S/ 700 or less: no SUNAT
  source addresses it. ◇ Perunio measures per comprobante unless the
  comprobante belongs to a venta con anticipos, and offers no way to link
  comprobantes of one contract outside that flow. Decided 2026-10-17: a
  contract billed in parts that should be measured as one operación is issued
  as a venta con anticipos.

---

## Cuándo NO aplica: exclusions

### Comprobantes that grant no crédito fiscal

A **boleta** (or a nota on one) supports neither crédito fiscal nor gasto/costo.
So **Anexo 2 and 3 códigos do not apply to it** (arts. 8 and 13), and neither
does **027** (RS 073-2006 art. 3).

All three provisions carry the same exception: "Esta excepción no opera cuando el
adquirente [usuario] sea una entidad del Sector Público Nacional a que se
refiere el inciso a) del artículo 18° de la Ley del Impuesto a la Renta".
Perunio cannot tell such a buyer from its RUC, so on a boleta to a RUC whose
código the exclusion rules out it asks ([The pre-emit questions](#the-pre-emit-question)).
A "sí" is stored as `adquirente_sector_publico` and lifts all three exclusions,
in the form and in the facturador.

**Anexo 1 is different.** It excludes only the póliza de adjudicación and the
liquidación de compra (art. 3), so it **does** apply on a boleta. So does the
inmueble regime (040): RS 022-2013 scoped art. 8's exclusions to the Anexo 2
goods of art. 7.1.

### Other exclusions

| Exclusion | Applies to | Perunio |
|---|---|---|
| Exportaciones (Cat.51 0200–0208): not operaciones gravadas con el IGV | All | Form: excluded. Facturador: a detracción requires operation type 1001–1004 (or 0104 on an anticipo), so an export cannot carry one. |
| Usuario **no domiciliado** (or whoever commissions the construction) | Anexo 3 (art. 13); 027 (RS 073-2006 art. 3) | Excluded for Anexo 3 and 027 when the receptor doc is 0 / B: in the form, and in the facturador on a factura (on a boleta the crédito-fiscal exclusion already applies, and its anonymous consumidor final also uses doc 0). The form also excludes Cat.51 0401, which the facturador refuses with any detracción. |
| No línea gravada con el IGV | Anexo 3 ("servicios gravados") | Form: excluded. |
| The documents of art. 4 num. 6.1 of the Reglamento de Comprobantes de Pago | Anexo 2 (except Bolsa de Productos pólizas), Anexo 3 | Not issued by Perunio. |
| Listed issuers, when they issue an electronic comprobante for the listed operations (Anexo 3, art. 13 c). Both SUNAT lists: AFPs; centros de inspección técnica vehicular; public rail passenger transport; domestic water freight (conocimiento de embarque); domestic air cargo (carta de porte aéreo); non-scheduled and special air passenger transport. Cartilla only: EPS; financial and insurance entities and cooperativas de ahorro y crédito; the Iglesia Católica, for leases of inmuebles. ★ | Anexo 3 | Not modelled. Such a company simply sets no default código, and answers "No está sujeta". |
| A comprobante that entitles the buyer to deduct the expense under art. 46 (2nd paragraph, a) of the Income Tax Law (Cartilla, item e; the orientación page does not mention it) | Anexo 3 | Not modelled. |
| Liquidación de compra | Anexos 1 and 2 | Not issued by Perunio. |
| **Numeral 10 literal exclusions** (037 only). Detail below the table. | 037 | Not modelled: the user does not pick 037 for these. |
| **Exclusions inside other numerales.** Detail below the table. | 019, 021, 022, 024, 025 | Not modelled. |
| Amazonía-exonerated sales (Ley 27037 art. 13.1 a): only Anexo 2 num. 21 b) goods are subject, so Amazonía café is not (art. 7.1; Informe 103-2013) | Anexo 2 | Not modelled. |
| Operations of a Ley 30737 Categoría 1 subject | All | Not modelled. |

**Numeral 10 literal exclusions (037).** Numeral 10 excludes:

- a) entities of the financial and insurance system
- b) ESSALUD
- c) ONP
- d) restaurants and bars
- e) hospedaje
- f) postal and express delivery
- g) carga terrestre (→ 027)
- h) public passenger transport (→ 028)
- i) the services excluded from numerales 6 a) and 7 a)/b)
- j) electricity
- k) PERUPETRO hydrocarbon contracts
- l) clearing houses (CAVALI)
- ll) port and airport administrators
- m) espectáculos públicos

It also excludes the same comercio-exterior operators as numerales 4 and 5.

**Exclusions inside other numerales:**

- Num. 2 excludes leasing.
- Nums. 4 and 5 exclude comercio-exterior operators (agentes marítimos, compañías aéreas, agentes de carga internacional, almacenes aduaneros, entrega rápida, agentes de aduana) serving special customs regimes, from 14-07-2012.
- Num. 6 excludes comisionistas who are Bolsa brokers, financial or insurance entities, or customs agents.
- Num. 7 excludes the user supplying only avíos textiles, or only designs and intangibles.

**Buyer without RUC.** There is no express exclusion. Such a buyer normally gets
a boleta, which triggers the crédito-fiscal exclusion for Anexos 2 and 3 and
027, but not for Anexo 1 or 040. The facturador requires an identified buyer
(DNI or RUC) on a **boleta** with a detracción (`validateCustomer`), because the
constancia de depósito is filed under the buyer; a factura names one already.

---

## What the XML must carry

From `resources/reglas/reglas-validacion-cpe-2026-08-26.xlsx`, sheets Factura2_0
and Boleta2_0, which carry identical detracción rules. NotaDebito2_0 and
NotaCredito2_0 differ; see below the table.

| Element | Rule | SUNAT code |
|---|---|---|
| `cbc:InvoiceTypeCode/@listID` (tipo de operación) | Cat.51: 1001 general, 1002 recursos hidrobiológicos, 1003 transporte de pasajeros, 1004 transporte de carga. Perunio mirrors it in `cbc:ProfileID`, which no current rule validates. | ERR 3205, 3206 |
| Operation type ↔ código | Under 1002, 1003 or 1004, the Detraccion `cbc:PaymentMeansID` must be 004, 028 or 027 respectively. SUNAT does not check the reverse: 027 under 1001 is not a 3129. Perunio pins both directions anyway, because `sunatOperationType` re-derives the operation type from the código. | ERR 3129 |
| `cbc:Note[@languageLocaleID='2006']` | "Operación sujeta a detracción". Missing → observation, not rejection. A leyenda code repeated → error. | OBS 4265, ERR 3014 |
| `cac:PaymentMeans` with `cbc:ID` = `Detraccion` | Required on 1001–1004. `cac:PayeeFinancialAccount/cbc:ID` = the Banco de la Nación account. `cbc:PaymentMeansCode`, when present, must be in Cat.59; Perunio sends 999. | ERR 3034, 3174 |
| `cac:PaymentTerms` with `cbc:ID` = `Detraccion` | Required on 1001–1004. `cbc:PaymentMeansID` = a Cat.54 código in the list (an `@schemeName`, if sent, must read "Codigo de detraccion"; Perunio sends none). `cbc:Amount` in **PEN**, positive, 2 decimals. `cbc:PaymentPercent` is **not validated**. | ERR 3127, 3033, OBS 4255, ERR 3035, 3037, 3208 |
| `cac:PaymentTerms` "Detraccion" on a non-detracción operation | Rejected. | ERR 3128 |
| Forma de pago crédito | The "Credito" `cac:PaymentTerms` amount is the neto pendiente. It may not exceed `PayableAmount`, and neither may any cuota. For a RUC buyer the cuotas must add up to it. SUNAT never checks the neto against total − detracción; that arithmetic is Perunio's. | ERR 3265, 3266, 3319 |
| 1004 (027) | Per-line `cac:Delivery`: origin and destination ubigeo and address, trip detail, and the three valores referenciales (01 service, 02 carga efectiva, 03 carga útil), all in PEN. | ERR 3116–3120, 3122–3126, 3208 |
| 1002 (004) | Per-line `cac:AdditionalItemProperty`, Cat.55 3001–3006. | ERR 3063, 3130–3135 |

**Notas.**

- **NotaDebito2_0** carries the PaymentTerms/PaymentMeans block: 3127 for a missing código, 3033, 3034, 3035, 3037, 3174, 3208 and OBS 4255. It also requires the two to come as a pair (ERR 3313, 3314).
- It has no operation-type trigger (3127/3128 on 1001–1004), no 3129 pairing and no leyenda 2006 rule.
- **NotaCredito2_0** carries no detracción rules at all.
- Perunio emits the block on both notas, because a nota mirrors the operation it corrects; only the nota de débito half is validated by SUNAT.

No rule checks the umbral. SUNAT accepts a detracción below the umbral, and a
0101 factura that should have carried one, even though the 2026 workbook names
0101 "Venta interna no sujeta a Detracción o Percepción". Perunio's validation is
what catches both.

**Spec cases** (`internal/xmlbuilder/spec/`):

- `invoice/24-detraccion-contado` to `invoice/31-detraccion-transporte-carga`
- `invoice/32-transporte-carga-varias-lineas`
- `invoice/39-detraccion-servicio-contabilidad`: F001-00000008 as it should have gone out
- `boleta/04-detraccion`
- `boleta/06-detraccion-inmueble-credito`: the seller deposits, so the cuotas carry the whole total
- `creditnote/06-detraccion`
- `debitnote/03-detraccion`

---

## Cómo decide Perunio

Detracción is a **document-level** decision: one código, one porcentaje, one
monto per comprobante. The issuance form (factura and boleta) shows it first in
**Cobro › "¿Sujeto a detracción?"** (`SimpleDetraccion`), with one line saying
which rule decided.

### Resolution order (`resolveDetraccion`)

1. **Switched off** by the user → none, and the comprobante is emitted with
   `detraccionNoSujeta: true` (stored on `issued_documents.detraccion_no_sujeta`).
2. **The código**, first match wins:
   1. picked by hand in the section;
   2. the productos' código (`productos.detraccion_codigo`, copied onto the line).
      More than one distinct código is a **conflict** and blocks emission;
   3. the **company default** (`companies.detraccion_codigo_default`, set in the
      company's facturador settings › Detracción), when no line carries a código
      of its own.
3. **Exclusions**: exportación; Anexo 2/3 and 027 on a boleta (or a nota on
   one); Anexo 3 and 027 for a no domiciliado; Anexo 3 with no línea gravada
   con IGV.
4. **The umbral** of the código, strictly greater than, over the operación in
   soles: the venta's monto acordado when the comprobante belongs to one (◇),
   and for 027 the greater of the importe and the valor referencial. Notas skip
   it, because a partial nota mirrors the referenced comprobante.
5. **The monto**: porcentaje × importe of this comprobante in soles (027: the
   greater of it and the valor referencial), rounded to céntimos. The neto
   pendiente subtracts it, except when the seller deposits (040 on a
   comprobante without crédito fiscal: `depositaEmisor`).

A código picked by hand **does not bypass** steps 3 and 4: they are the law, and
the facturador rejects a detracción that breaks them.

### The pre-emit question

A **factura** whose operación is over S/ 700 and that has **no código at all**
(none from a producto, no company default, none picked) asks before emitting:

> **Factura mayor a S/ 700 sin detracción.** ¿Es un bien o servicio sujeto al SPOT?
> *Marcar detracción* · *No está sujeta, emitir* · *Cancelar*

"No está sujeta" emits with `detraccionNoSujeta: true`.

Boletas are not asked, because Anexos 2 and 3 and 027 never apply to them and
an Anexo 1 bien always arrives with its producto's código. Exportaciones are not
asked. Below S/ 700, only 027 (above S/ 400) and the any-amount códigos (010,
031, 036, 040) could apply; those are expected to come from a producto or the
company default.

Two more questions are asked when a código is present but a rule ruled it out
that a fact only the user knows would lift (`detraccionPregunta`). Each is asked
once per comprobante and its answer stays editable in the detracción section;
"Sí" applies the detracción and returns the user to review it before emitting.

> **Boleta con detracción excluida.** ¿El adquirente es una entidad del Sector Público Nacional?

Asked on a boleta (or a nota on one) to a buyer with RUC whose Anexo 2/3 or 027
código the exclusion rules out. Sent as `adquirenteSectorPublico`.

> **Bien del Anexo 1 bajo el umbral.** ¿Una misma unidad de transporte lleva más de ½ UIT en bienes del Anexo 1?

Asked when an Anexo 1 código falls at or under ½ UIT. Sent as
`anexo1UnidadTransporte`, only when it is what made the detracción apply; the
proveedor deposits then, so the neto pendiente is the whole total.

### What the facturador enforces

`validateDetraccion`, `validateDetraccionCatalogo`, `validateTransporteCarga`,
`validateHidrobiologicos` and `validateCustomer` in `internal/validation/rules.go` reject, before anything
reaches SUNAT:

- a detracción on anything but a factura, a boleta, or a nota referencing one;
- an operation type other than 1001–1004, or 0104 on an anticipo (which goes out as 1001–1004);
- a código that is not `emit` in Cat.54 (pre-empts ERR 3033), or 028;
- a porcentaje other than the catálogo's (`porcentajeAlternativo` also passes, which today means only 004's 10 %);
- an Anexo 2/3 or 027 código on a boleta or on a nota referencing one;
- an Anexo 3 or 027 código on a factura whose receptor is identified as no domiciliado (Cat.06 0 or B);
- an operación at or under the código's umbral, unless an Anexo 1 código carries `anexo1UnidadTransporte` (refused above the umbral). It is measured on `TaxInclusiveAmount`, or on the venta's `monto_acordado` (`IssueRequest.MontoOperacion`, filled by the pipeline, never by the client) when that is larger. Notas and 0104 anticipos are not measured;
- a monto that is not porcentaje × the SPOT base in soles (±0.01), for any currency (027: the greater of the importe and the valor referencial), or one larger than the payable;
- a missing cuenta BN, or a missing tipo de cambio on a foreign-currency document;
- an incomplete 027 trip block, including ubigeos that are not INEI districts;
- an incomplete 004 block: the embarcación and descarga, and every line's especie and cantidad, in SUNAT's formats;
- a boleta with a detracción whose buyer is not identified by DNI or RUC.

It does **not** detect a factura that should have carried a detracción but has
none: from the XML alone it cannot know what was sold. That is the form's job,
via the section and the pre-emit question.

---

## Where each rule lives

| Rule | Where |
|---|---|
| Rates, umbrales, anexo, operation type per código | `perunio-sunat-catalogs/catalogs/cat54.json` → `sunat.Cat54*` (Go), `sunat-catalogs/cat54` (TS) |
| The picker and its list | `perunio-frontend/src/components/facturador/DetraccionCodigoField.tsx`; `perunio-frontend/src/sunat/detracciones.ts` (`DETRACCIONES`, built from `CAT54_SELECT_CODES`; `detraccionItem`, any `emit` código, for resolving) |
| Resolution in the form | `perunio-frontend/src/lib/facturador-utils.ts` › `resolveDetraccion` |
| The section and the reason line | `perunio-frontend/src/components/facturador/SimpleDetraccion.tsx` |
| Pre-emit question | `perunio-frontend/src/hooks/use-comprobante-form.ts` › `runSubmit`; `use-detraccion-confirm.ts`; `DetraccionConfirmDialog.tsx` |
| Company default código | `perunio-frontend/src/components/company-detraccion-tab.tsx` (tab "Detracción" in `facturador-company-settings.tsx`); `perunio-backend` `companies.detraccion_codigo_default` |
| Notas | `perunio-frontend/src/hooks/use-nota-form.ts` (mirrors the reference, no umbral) |
| Server validation | `perunio-facturador/internal/validation/rules.go` › `validateDetraccion`, `validateDetraccionCatalogo`, `validateTransporteCarga`, `validateCustomer` |
| XML | `perunio-facturador/internal/xmlbuilder/common.go` › `buildDetraccionPaymentMeans`, `buildDetraccionPaymentTerms`, `appendDetraccionLegend`, `sunatOperationType` |
| Neto pendiente | `model.IssueRequest.NetoPendientePago()` and `DetraccionDepositaEmisor()` (Go, `internal/model/document.go`); `netoPendientePago` and `detraccionDepositaEmisor` (TS, `facturador-utils.ts`); `buildDetraccion` for the PDF (`perunio-frontend/src/components/invoice-pdf/lib/map-common.ts`) |
| DB columns | `perunio-backend/src/db/schema.ts`: `companies.cuenta_detraccion`, `companies.detraccion_codigo_default`, `issued_documents.detraccion_*`, `issued_documents.detraccion_no_sujeta`, `productos.detraccion_codigo` |

### Tests

- `perunio-facturador`:
  - `go test ./internal/validation -run 'TestValidateDetraccion|TestValidateTransporteCarga'` (this also runs `TestValidateDetraccionCatalogo`)
  - `go test ./internal/xmlbuilder -run 'TestSpec/(invoice/39|boleta/0[46])'`
- `perunio-frontend`: `npm run check:resolve-detraccion` (table test of `resolveDetraccion`).

A case that resolves a detracción in the frontend table must pass the Go
validator, and vice versa.

---

## Open questions

Rules marked ★ above. Each one is also an entry in `FOLLOWUPS.md`.

| ★ | Question | FU |
|---|---|---|
| 1 | Status of 038, 042, 043, 046, 047. The workbook's change-log row is struck through, and no resolution up to RS 000204-2026 (14-10-2026) touches the SPOT. | FU-8 |
| 2 | The Anexo 3 issuer list: the orientación "Servicios" page lists six issuers, and the Cartilla (2019–2020) eight. | FU-8 |
| 3 | Which exchange rate the S/ 700 test uses on a USD comprobante. Art. 20 governs the deposit; Perunio uses the comprobante's own tipo de cambio for both. | FU-8 |

## Known gaps

Rules that are settled law but that Perunio does not model yet. None at the
moment: the last ones (FU-5, FU-7 and the Sector Público buyer of FU-8) were
built in October 2026.

---

## Sources

All primary texts were read on 2026-10-17. For resolutions, the dates are given
as signature date; vigencia.

**R.S. 183-2004/SUNAT** (consolidated text). It is updated only through
RS 098-2008, so read it together with the amendments below:
https://www.sunat.gob.pe/legislacion/superin/2004/183.htm

**Amendments**

- R.S. 246-2017 (02-10-2017; 16-10-2017): 001 and 003 back into Anexo 1, 007 into Anexo 2, all at 10 %. https://www.sunat.gob.pe/legislacion/superin/2017/246-2017.pdf
- R.S. 071-2018 (published 03-03-2018; 01-04-2018): Anexo 3 numerales 1, 3, 5 and 10 to 12 %. https://www.sunat.gob.pe/legislacion/superin/2018/071-2018.pdf
- R.S. 082-2018 (16-03-2018; 01-04-2018): 011 and 016 at 10 %. https://www.sunat.gob.pe/legislacion/superin/2018/082-2018.pdf
- R.S. 152-2018 (08-06-2018; 16-06-2018): 023 leche at 4 %. https://www.sunat.gob.pe/legislacion/superin/2018/152-2018.pdf
- R.S. 130-2019 (25-06-2019; 01-08-2019): 032 at 10 %, 041 at 15 %. https://www.sunat.gob.pe/legislacion/superin/2019/130-2019.pdf
- R.S. 000086-2025 (28-03-2025; 01-04-2025): 045 and 034 into Anexo 1, 044 into Anexo 3, and Anexo 2 numeral 19 derogated. https://www.sunat.gob.pe/legislacion/superin/2025/000086-2025.pdf
- R.S. 000121-2025 (14-04-2025) and R.S. 000175-2025 (29-05-2025): traslados of Anexo 1 numerales 3–4 excluded. https://www.sunat.gob.pe/legislacion/superin/2025/000121-2025.pdf and https://www.sunat.gob.pe/legislacion/superin/2025/000175-2025.pdf
- R.S. 249-2012 (vigente 01-11-2012): added Anexo 2 numerales 19–21 and, apparently, the any-amount carve-out. Its PDF is a scan, so this was not machine-checked; the carve-out itself is confirmed by the Cartilla and the orientación page. https://www.sunat.gob.pe/legislacion/superin/2012/249-2012.pdf

**Special regimes**

- Transporte de bienes: R.S. 073-2006, as amended by R.S. 081-2006 (S/ 400) and R.S. 158-2006 (4 %). https://www.sunat.gob.pe/legislacion/superin/2006/073.htm
- Transporte público de pasajeros: R.S. 057-2007. https://www.sunat.gob.pe/legislacion/superin/2007/057.htm
- Inmuebles: R.S. 022-2013. https://www.sunat.gob.pe/legislacion/superin/2013/022-2013.pdf
- Arroz (IVAP): R.S. 266-2004. https://www.sunat.gob.pe/legislacion/superin/2004/266.htm

**Other sources**

- Neto pendiente: R.S. 193-2020 (05-11-2020; 01-04-2021). https://www.sunat.gob.pe/legislacion/superin/2020/193-2020.pdf
- Informe N.° 103-2013-SUNAT/4B0000 (anticipos for café; Amazonía). https://www.sunat.gob.pe/legislacion/oficios/2013/informe-oficios/i103-2013.pdf
- SUNAT orientación pages:
  - "En la venta de bienes": https://orientacion.sunat.gob.pe/3141-02-en-la-venta-de-bienes-empresas
  - "En la prestación de servicios": https://orientacion.sunat.gob.pe/3142-03-en-la-prestacion-de-servicios-empresas
  - Apéndices: https://orientacion.sunat.gob.pe/apendices-del-sistema-de-detracciones
  - Ley 30737: https://orientacion.sunat.gob.pe/ley-30737
  - Cartilla (2019–2020): https://orientacion.sunat.gob.pe/sites/default/files/inline-files/Cartilla_detracciones.pdf
- UIT 2026: D.S. 301-2025-EF. https://busquedas.elperuano.pe/dispositivo/NL/2469116-1
- SUNAT's 2026 index of resoluciones de superintendencia, checked for SPOT changes: https://www.sunat.gob.pe/legislacion/superin/2026/indices/indcor.htm
- Validation workbook: `resources/reglas/reglas-validacion-cpe-2026-08-26.xlsx` (Factura2_0, Boleta2_0, NotaDebito2_0, NotaCredito2_0, Catálogos, Control de Cambios). cpe.sunat.gob.pe still returns 403, so its "factura con detracción" page was not used.
