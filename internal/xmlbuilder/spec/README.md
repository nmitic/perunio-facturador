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

### XPath conventions

Plain **XPath 1.0**. The root element is always addressed as `/*`, so the document's default namespace never needs a prefix. These prefixes are fixed:

| prefix | namespace URI |
|---|---|
| `cac` | `urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2` |
| `cbc` | `urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2` |
| `ext` | `urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2` |
| `sac` | `urn:sunat:names:specification:ubl:peru:schema:xsd:SunatAggregateComponents-1` |
| `ds`  | `http://www.w3.org/2000/09/xmldsig#` |

To stay portable across engines, a parenthesized union gets at most one predicate (conditions are joined with `and`), and line numbering is checked with `preceding-sibling` counts, not `position()`.

## Invariants

`invariants.yaml` holds the rules that apply to every document of a given root (`Invoice`, `CreditNote`, `DebitNote`, `SummaryDocuments`, `VoidedDocuments`, `DespatchAdvice`). Each one is a single XPath 1.0 boolean expression, with the SUNAT code and the reason. They cover:

- the totals arithmetic SUNAT recomputes (3278, 3279, 3280, 3294)
- per-line values and rates (3271, 3103, 3292)
- gratuito rules (2640, 3224, 3234, 2416)
- the tributo subtotals that must exist (2638)
- currency (2071, 3208)
- leyenda format (3006, 3014)
- the signature slot
- UBL versions (2074/2072)
- RC/RA identifiers (2220)

They run on every case, so a new case inherits them for free. The one invariant outside XPath is the prolog: the bytes must start with `<?xml version="1.0" encoding="ISO-8859-1"`.

## Pending cases

A case with `pending:` states what SUNAT requires and what the builder does not do yet. The runner reports it as **SKIP**, with the first failing assertion, so CI stays green while the gap stays visible. If a pending case starts passing, the runner **fails** and tells you to remove `pending` and run `-update`. That way a fix can't go unnoticed. Pending cases have no `expected.xml`, because their current output is known to be wrong.

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
| F1 | Exonerado / inafecto / exportación lines never get their document `cac:TaxSubtotal` (9997/9998/9995). A document with no gravado line falls back to an IGV 1000 subtotal whose base is the whole subtotal. | ERROR 2638, 3277, 3107 | `invoice.go` `buildDocumentTaxTotal` | invoice/03–06 |
| F2 | Exonerado / inafecto / exportación line subtotals omit `cbc:Percent` (SUNAT requires it on every tributo except 7152). | ERROR 2992 | `sunat.Cat07Percent` via `newLineTaxCategory` | invoice/03–06 |
| F3 | ISC: missing from the document `cbc:TaxAmount` and from the line `cbc:TaxAmount`; the ISC line subtotal has no `cbc:Percent`; the document ISC base is `req.Subtotal`, not Σ ISC line bases. | ERROR 3294, 3292, 2992, 3296 | `buildDocumentTaxTotal`, `buildLineTaxTotal` | invoice/09–10 |
| F4 | The issue pipeline sends every free-text draft note as leyenda **1000** ("monto en letras"). Multi-line or >200-character notes break 3006, and the real monto en letras is never produced. | ERROR 3006 | `internal/http/routes_pipeline.go` `buildIssueRequestFromDoc` | invoice/36 |
| F5 | IVAP documents lack leyenda 2007. | OBSERV 4264 | `buildInvoiceXML` | invoice/08 |
| F6 | RC lines use the Resumen **1.0** range shape (`sac:DocumentSerialID` + `Start/EndDocumentNumberID`) under CustomizationID **1.1**, which identifies each comprobante with one `cbc:ID` per line (RC 1.1 guide, sample XML p. 27). | ERROR 2512 | `summary.go` | summary/01–06 |
| F7 | RC lines omit the IGV `cac:TaxTotal` when the IGV is 0. | ERROR 2278 | `buildSummaryLine` | summary/06 |
| F8 | GRE por eventos references the superseded guía by Perunio's database UUID (`OriginalGreID`), with the Cat.59 event code in `cbc:DocumentTypeCode`. | — | `despatch_evento.go` | despatch/05 |
| F9 | GRE puts the punto de partida in `cac:Shipment/cac:OriginAddress` and the plate in `ShipmentStage/TransportMeans/RoadTransport`, the GRE 1.0 shapes. GRE 2.0 uses `Delivery/Despatch/DespatchAddress` and `TransportHandlingUnit/TransportEquipment/cbc:ID`; SUNAT's GRE rules and its GRE validator XSL never mention `OriginAddress` or `RoadTransport`. | GRE rules › Guía-Remitente2_0 | `despatch_common.go` `buildShipment` | despatch/06 |
| F10 | Any character outside ISO-8859-1 (curly quotes, em dash, €) makes `BuildDocumentXML` fail, so the issue ends with `XML_BUILD_ERROR`. Validation does not catch it first. | — | `encoding.go` | invoice/35 |
| F11 | RA `cbc:ReferenceDate` is set to the RA's own issue date. It must be the issue date of the voided documents, and `model.VoidRequest` has no field for it. Voiding anything from a previous day is rejected. | ERROR 2375 | `voided.go`, `model.VoidRequest` | voided/03 |

Settled along the way:

- **Gratuito and the document 9996 subtotal.** The old string test said the document must *not* carry a 9996 subtotal. CLAUDE.md (accepted CDRs) and rule 2638 say it must. `invoice/16` pins it.
- **`sac:AdditionalMonetaryTotal` 1004** (total gratuito) is UBL 2.0-era markup. The 2.1 validation workbook never reads it. It sits inside `ext:UBLExtensions`, where it is harmless, so it stays until someone decides to drop it.
- **`cac:TaxCategory/cbc:ID`** (UN/ECE 5305 S/E/O/Z/G) is not validated by SUNAT 2.1, so the cases do not assert it. The builder emits `E` for 9996, while greenter emits `Z`.

## Sources

Everything lives in `resources/`. Its `README.md` lists each file's SUNAT publication date and where to get a newer one.

- `resources/reglas/reglas-validacion-cpe-2026-08-26.xlsx` is SUNAT's CPE validation workbook: every rule with its XPath and return code, one sheet per document type (factura, boleta, NC, ND, Resumen Diario, Comunicación de Baja, …), plus the current catálogos.
- `resources/reglas/reglas-validacion-gre-2026-09-25.xlsx` holds the same for GRE Remitente 2.0 and Transportista 2.0.
- `resources/xsl/cpe/` and `resources/xsl/gre/` are SUNAT's executable validators (`ValidaExprReg*.xsl`), the checks SUNAT and its Facturador run.
- `resources/xsd/2.1/` holds the OASIS UBL 2.1 schemas (Invoice, CreditNote, DebitNote, DespatchAdvice). `resources/xsd/2.0/` holds SUNAT's UBL 2.0 schemas, including `SummaryDocuments`, `VoidedDocuments` and the `sac` extensions.
- `resources/guias/` holds SUNAT's "Guía de elaboración" for factura, boleta, NC and ND (UBL 2.1, May 2017), the Resumen Diario 1.1 guide (Jan 2018) and the manual del programador.
- CLAUDE.md › Critical SUNAT Gotchas lists rules learned from real SUNAT beta CDRs.
- greenter (`github.com/thegreenter/greenter`) is a widely used open-source reference implementation.
