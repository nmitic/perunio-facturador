# XML output spec — perunio-facturador

This folder describes **what XML perunio-facturador must send to SUNAT**, as plain data. You can read it and check it without knowing Go or this codebase. A Go runner (`../spec_runner_test.go`) executes every case on `make test`, but the files here are the spec.

Use it to:

- review a builder change (the `expected.xml` diff is the review artifact)
- ask a person or another AI "is this right?", using the prompt template at the end
- check our output against SUNAT's rules with any XPath 1.0 engine (xmllint, lxml, Saxon, …)

## Layout

```
spec/
  README.md          ← this file
  invariants.yaml    ← rules every document must satisfy (SUNAT arithmetic, currency, signature slot…)
  <family>/<NN-case>/
    input.json       ← what the builder receives (business data)
    spec.yaml        ← why the case exists + assertions on the output
    expected.xml     ← the exact output, pretty-printed as UTF-8 (absent for pending cases)
```

Families: `invoice` (factura 01), `boleta` (03), `creditnote` (07), `debitnote` (08), `summary` (Resumen Diario RC), `voided` (Comunicación de Baja RA), `despatch` (Guía de Remisión 09/31/por eventos), `filename`.

## Reading a case

`input.json` is the builder's input as JSON:

| `document` | input shape | Go type |
|---|---|---|
| `cpe` | one comprobante (factura, boleta, NC, ND) | `model.IssueRequest` (`internal/model/document.go`) |
| `summary` | Resumen Diario | `model.SummaryRequest` (`internal/model/summary.go`) |
| `voided` | Comunicación de Baja | `model.VoidRequest` |
| `despatch` | `{ruc, companyName, companyAddress, eventBaseDocType, despatch, lines}` | `model.Despatch` + `[]model.DespatchLine` |
| `filename` | `{kind, ruc, docType, series, correlative, issueDate, eventBaseDocType}` | the `*Filename` helpers |

Amounts are decimal strings. **Totals are inputs, not outputs**: the frontend computes `subtotal`, `totalIgv`, `taxInclusiveAmount` and `totalAmount`, and the builder places them (and derives some document figures from the lines). Every input here is internally consistent. A case with inconsistent totals would test the fixture, not the builder.

Key `IssueRequest` fields: `docType` (Cat.01), `operationType` (Cat.51), `customerDocType` (Cat.06), `items[].taxExemptionReasonCode` (Cat.07 afectación), `items[].priceTypeCode` (Cat.16), `items[].unitPrice` (IGV-exclusive valor unitario), `items[].unitPriceWithTax` (IGV-inclusive precio unitario), `items[].lineTotal` (valor de venta of the line, net of its descuento), `globalDiscount` (Cat.53 02), `anticipos[]` (`baseAmount` without IGV, `totalAmount` with IGV), `detraccion` (Cat.54 código, monto always in PEN), `formaPago` + `cuotas[]`.

A few pending cases carry an input field the model does not have yet (for example `summary/09` sends `customerName`). Inputs are decoded strictly, so such a case fails to decode until the field exists. That is deliberate: the input already says what the fix needs.

`spec.yaml`:

```yaml
case: factura-exonerada          # matches the folder name after the NN- prefix
document: cpe                    # which builder runs
why: >-                          # the business rule, in one paragraph
  Exonerado lines (Cat.07 20) are declared under tributo 9997 …
sources:                         # where the rule comes from
  - "resources/reglas/reglas-validacion-cpe-2026-08-26.xlsx › Factura2_0"
pending: >-                      # optional: we KNOW the builder is wrong here (see Known deviations)
  Finding F1: …
effective: "2027-01-01"          # optional: the date SUNAT starts enforcing the rule (see Upcoming rules)
assert:
  - xpath: "/*/cac:TaxTotal/cac:TaxSubtotal[cac:TaxCategory/cac:TaxScheme/cbc:ID='9997']/cbc:TaxableAmount"
    equals: "500.00"
    sunat: "2638/3275"           # the code SUNAT answers with when this is wrong
    why: >-                      # optional, per assertion
      …
```

### Assertion kinds

| key | passes when |
|---|---|
| `equals: "x"` | the XPath selects **exactly one** node whose text is exactly `x`, or evaluates to the number/string `x`. Document text is compared literally (`1180.0` ≠ `1180.00`); results of `sum()`/`count()`/arithmetic are compared to the cent. |
| `count: n` | the node-set has `n` nodes (or the expression evaluates to `n`) |
| `absent: true` | the node-set is empty |
| `true: true` | the expression is true |
| `matches: "re"` | the single selected value matches the regular expression |
| `before: [a, b]` | the first node of `a` comes before the first node of `b` in document order (UBL schema sequence) |
| *(xpath only)* | at least one node matches |

`error_contains: "…"` means the builder must **refuse** the input. `output: "…"` (filename cases) is the exact string.

### SUNAT codes

`sunat:` is the return code SUNAT answers with when the assertion is false, as listed in the validation workbooks (`resources/reglas/`). Two codes joined by `/` are the related rules that apply (usually "tag missing / value wrong").

- Codes **2000–3999 reject** the document (CDR rechazado). Codes **4000 and up are observaciones**: the document is accepted with a warning. A few rules are marked "OBSERV ERROR" in the workbook; they change kind on a stated date.
- The same check can carry different codes per document type. A **boleta** answers most totals arithmetic with an observación (for example 4309), where a **factura** rejects (3278). A **nota de crédito or débito** uses the factura's code only when it modifies a factura (`01`) and the boleta's observación otherwise.
- **GRE** cases cite `reglas-validacion-gre-2026-09-25.xlsx` (sheets Guía-Remitente2_0 and Guía-Transportista2_0), which has its own codes: UBL version 2111/2110 and CustomizationID 2113/2112, not the CPE's 2074/2072.
- An assertion without `sunat:` pins Perunio's output where SUNAT has no rule. Its `why` says so.

### XPath conventions

Plain **XPath 1.0**. The root element is always addressed as `/*`, so the document's default namespace never needs a prefix. These prefixes are fixed:

| prefix | namespace URI |
|---|---|
| `cac` | `urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2` |
| `cbc` | `urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2` |
| `ext` | `urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2` |
| `sac` | `urn:sunat:names:specification:ubl:peru:schema:xsd:SunatAggregateComponents-1` |
| `ds`  | `http://www.w3.org/2000/09/xmldsig#` |

To stay portable across engines, a parenthesized union gets at most one predicate (conditions are joined with `and`), and line numbering is checked with `preceding-sibling` counts, not `position()`. XPath 1.0 has no regex, so `invariants.yaml` checks number formats with `translate`, `substring-before`/`substring-after` and `string-length`. (`matches:` assertions use a regex, but that is the runner's check, not XPath.)

## Invariants

`invariants.yaml` holds the rules that apply to every document of a given root (`Invoice`, `CreditNote`, `DebitNote`, `SummaryDocuments`, `VoidedDocuments`, `DespatchAdvice`). Each one is a single XPath 1.0 boolean expression, with the SUNAT code per document type and the reason. Where SUNAT publishes the formula in its XSL validators, the expression follows it. They cover:

- the totals arithmetic SUNAT recomputes: total valor de venta, total precio de venta (at the rate the lines declare, 18 or 10.5), importe total for facturas/boletas and the separate formula for notas, document tax amount, ISC and the 9995/9997/9998 bases (3278, 3279, 3280, 3294, 3296/3298, 3273–3275 and their boleta/nota variants)
- per-line values and rates: line value, unit price, line tax total, rate present/format/non-zero/equal across IGV lines (3271, 3270, 3292, 3103, 2992, 3102, 2993/3104, 3462)
- gratuito rules: price, price type, leyenda, and the document 9996 totals (2640, 3224, 3234, 2416, 2641, 3276, 3302)
- the tributo subtotals that must exist (2638)
- currency (2071, 3208)
- leyenda format (3006, 3014)
- the signature slot (Firma sheet; GRE 1059)
- UBL versions (2074/2072; GRE 2111/2110, 2113/2112)
- RC/RA identifiers (2220)

They run on every case, so a new case inherits them for free. The one invariant outside XPath is the prolog: the bytes must start with `<?xml version="1.0" encoding="ISO-8859-1"`.

## Pending cases

A case with `pending:` states what SUNAT requires and what the builder does not do yet. The runner reports it as **SKIP**, with the first failing assertion, so CI stays green while the gap stays visible. If a pending case starts passing, the runner **fails** and tells you to remove `pending` and run `-update`. That way a fix can't go unnoticed. Pending cases have no `expected.xml`, because their current output is known to be wrong.

`effective:` marks a case whose rule SUNAT enforces from a given date; the SKIP line shows it (`PENDING (effective 2027-01-01) …`). Fix those before that date.

## Golden files

`expected.xml` is the builder's exact output, decoded from ISO-8859-1 and indented. Indentation goes between elements only; no text or attribute value changes. Any change to the output shows up as a diff. After an intended change:

```bash
go test ./internal/xmlbuilder -run TestSpec -update   # rewrites expected.xml for non-pending cases
git diff internal/xmlbuilder/spec                      # review it like code
```

## Running

```bash
make test                                              # everything, as CI does
go test ./internal/xmlbuilder -run TestSpec -v         # one subtest per case; pending ones show their reason
go test ./internal/xmlbuilder -run 'TestSpec/invoice/11'
```

The Go runner evaluates XPath with `github.com/antchfx/xpath`, which must be **v1.3.9 or later**. v1.3.3 lost the context node after a relative path with a predicate, so in `sum(a[b = 'x']/c) - d` it read `d` from the wrong node. Under it `INV-LINE-TAX-SUM` (3292) never fired, and `INV-LINE-UNIT-PRICE` would not have either.

### Without Go

Every assertion is ordinary XPath, so you can check a golden file with any engine. With Python and lxml:

```python
from lxml import etree
NS = {
    "cac": "urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2",
    "cbc": "urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2",
    "ext": "urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2",
    "sac": "urn:sunat:names:specification:ubl:peru:schema:xsd:SunatAggregateComponents-1",
    "ds":  "http://www.w3.org/2000/09/xmldsig#",
}
doc = etree.parse("invoice/11-gratuito-gravado-retiro/expected.xml")
print(doc.xpath("/*/cac:TaxTotal/cac:TaxSubtotal[cac:TaxCategory/cac:TaxScheme/cbc:ID='9996']/cbc:TaxableAmount/text()", namespaces=NS))
```

With xmllint, bind the prefixes in its shell:

```
xmllint --shell invoice/11-gratuito-gravado-retiro/expected.xml
/ > setns cac=urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2
/ > setns cbc=urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2
/ > xpath /*/cac:TaxTotal/cbc:TaxAmount/text()
```

### Asking someone else to review a case

> You are reviewing an electronic invoice for SUNAT (Peru), UBL 2.1. Attached are `input.json` (what the seller entered), `spec.yaml` (the rules we believe apply, each with a SUNAT error code) and `expected.xml` (what we send). Using SUNAT's "Guía de elaboración" and its current validation-rules workbook (Reglas de validación), tell us (1) whether each assertion in `spec.yaml` is a correct statement of SUNAT's rules, (2) whether `expected.xml` would be accepted, and if not, which SUNAT code it would get and why, and (3) any rule we are missing.

## Known deviations

Each row is a `pending` reason. The SUNAT codes come from `resources/reglas/reglas-validacion-cpe-2026-08-26.xlsx` (GRE: `reglas-validacion-gre-2026-09-25.xlsx`) unless noted.

| id | what is wrong | SUNAT | where | cases |
|---|---|---|---|---|
| F18 | No "fecha de entrega de bienes al transportista" (`ShipmentStage/LoadingTransportEvent/cbc:OccurrenceDate`) on transporte público guías; mandatory since 2026-06-01. `model.Despatch` has no field. | ERROR 3617, 3618, 3619 | `buildShipment`, `model.Despatch` | despatch/10 |

## Upcoming rules

SUNAT dates some changes in the workbook's *Control de Cambios*. Rules that already have a case carry `effective:`.

| from | rule | case |
|---|---|---|
| 2026-06-01 (in force) | GRE: fecha de entrega de bienes al transportista on transporte público (ERR 3617/3618/3619, replacing OBS 4385/4386 and ERR 3407) | despatch/10 |
| 2027-01-01 | RC: IGV rate on each line subtotal (ERR 2992/3504/3102) | summary/07 |
| 2027-01-01 | RC: gratuitas split into Cat.11 06–09 and an "IGV gratuitas" TaxTotal (ERR 3051 …) | summary/08 |
| 2027-01-01 | RC: customer name, field 10.3 (ERR 2022; OBSERV until 2026-12-31) | summary/09 |
| 2027-01-01 | NC: only one `BillingReference` for every motivo except 11 (ERR 3261); NC amounts may not exceed the original's (ERR 3503, needs the original, not checkable from the XML) | — (the builder already emits one reference) |
| 2027-01-01 | ND: new motivo 13 Penalidades (Cat.10), with no `BillingReference` required (ERR 2524) and no IGV (ERR 3507); one `BillingReference` for every motivo (ERR 3194) | — (motivo 13 is not offered yet) |
| 2027-01-01 | Código de producto SUNAT format becomes an error (ERR 3496) | — (not emitted) |
| 2027-01-01 | Cat.01 25 and the DAE documents 28/56 in NC/ND/RA | — (not issued) |

## Not covered yet

Rules SUNAT checks that no case pins today. Each is either an observación or a feature Perunio does not offer.

- GRE observaciones: carrier MTC number (4391; `carrierMtc` is ignored), vehicle TUC (4399), freight-payer indicator (4388), proveedor on motivo 02 (4375).
- GRE motivo 04 needs `AddressTypeCode` on both addresses (ERR 3365/3369); motivos 08/09/19 need customs documents (ERR 3440/3493/3483). The builder can emit neither.
- Secondary vehicles (`vehiclePlates`) are ignored (no rejection, data loss).
- Validation-layer gaps (not XML): destinatario = / ≠ remitente by motivo (ERR 2554/2555), carrier name (2563), driver document (2568), start date (3406), issue date in the future (2108).

Settled along the way:

- **Gratuito and the document 9996 subtotal.** The old string test said the document must *not* carry a 9996 subtotal. CLAUDE.md (accepted CDRs) and rules 2638/2641 say it must. `invoice/16` pins it.
- **A fallback IGV subtotal on a gratuita** is rejected as ERROR 3277 (the 1000 base must equal Σ gravado lines). 3224 is the line-level rule: a 02 referencial without a 9996 line subtotal.
- **`cbc:Percent` 0.00** is valid: SUNAT's XSL implements 3102 with the regex `^(?!(0)[0-9]+$)[0-9]{1,3}(\.[0-9]{1,5})?$`, which accepts it, although the workbook text says "diferente de cero".
- **Document-level `cbc:Percent`** is not validated: every Percent check in `ValidaExprRegFactura-2.0.1.xsl` runs in the line template. The builder emits it on the 9996 document subtotal (as greenter does); it is harmless.
- **Detracción on a nota de crédito** is not validated: the NotaCredito2_0 sheet has no detracción rule. The nota de débito sheet does validate it (`debitnote/03`).
- **`sac:AdditionalMonetaryTotal` 1004** (total gratuito) is UBL 2.0-era markup. The 2.1 validation workbook never reads it. It sits inside `ext:UBLExtensions`, where it is harmless, so it stays until someone decides to drop it.
- **`cac:TaxCategory/cbc:ID`** (UN/ECE 5305 S/E/O/Z/G) is not validated by SUNAT 2.1, so the cases do not assert it. The builder emits `E` for 9996, while greenter emits `Z`.

## Sources

Everything lives in `resources/`. Its `README.md` lists each file's SUNAT publication date and where to get a newer one.

- `resources/reglas/reglas-validacion-cpe-2026-08-26.xlsx` is SUNAT's CPE validation workbook: every rule with its XPath and return code, one sheet per document type (factura, boleta, NC, ND, Resumen Diario, Comunicación de Baja, …), the current catálogos, and the Control de Cambios with effective dates.
- `resources/reglas/reglas-validacion-gre-2026-09-25.xlsx` holds the same for GRE Remitente 2.0 and Transportista 2.0.
- `resources/xsl/cpe/` and `resources/xsl/gre/` are SUNAT's executable validators (`ValidaExprReg*.xsl`), the checks SUNAT and its Facturador run. Where a workbook sentence is ambiguous, the XSL decides.
- `resources/xsd/2.1/` holds the OASIS UBL 2.1 schemas (Invoice, CreditNote, DebitNote, DespatchAdvice). `resources/xsd/2.0/` holds SUNAT's UBL 2.0 schemas, including `SummaryDocuments`, `VoidedDocuments` and the `sac` extensions.
- `resources/guias/` holds SUNAT's "Guía de elaboración" for factura, boleta, NC and ND (UBL 2.1, May 2017), the Resumen Diario 1.1 guide (Jan 2018) and the manual del programador.
- `resources/normas/rs-123-2022-sunat.pdf` is the GRE 2.0 regime (R.S. 123-2022/SUNAT).
- CLAUDE.md › Critical SUNAT Gotchas lists rules learned from real SUNAT beta CDRs.
- greenter (`github.com/thegreenter/greenter`) is a widely used open-source reference implementation.
