---
name: perunio-facturador
description: Use this skill when building, modifying, or debugging a Peruvian electronic invoicing (SEE — Sistema de Emisión Electrónica) backend in Go. Covers SUNAT UBL 2.1 / UBL 2.0 XML structure for Factura, Boleta, Notas de Crédito y Débito, Resumen Diario (RC), Comunicación de Baja (RA), digital signature (XMLDSig enveloped), file/ZIP naming, SOAP WS-Security envelopes for sendBill / sendSummary / getStatus / getStatusCdr, CDR (ApplicationResponse) parsing, SUNAT catalogs (01, 06, 07, 16, 17, 51), and the validation/contingency rules from RS 097-2012/SUNAT and modificatorias.
---

# SUNAT Electronic Invoicing — Go Backend Skill

Authoritative knowledge base for building a Go facturador against SUNAT's SEE
(Sistema de Emisión Electrónica — Sistemas del Contribuyente). Source of
truth: `Manual del Programador` (May 2021, v2.1) and the `Guía de Elaboración
de Documentos XML — Factura Electrónica UBL 2.1` (May 2017, v1.0).

---

## 1. Document Types & XML Structure

| Code | Name                    | Root Element        | UBL | CustomizationID | ID Pattern (cbc:ID)     | Submit method  |
| ---- | ----------------------- | ------------------- | --- | --------------- | ----------------------- | -------------- |
| 01   | Factura                 | `/Invoice`          | 2.1 | 2.0             | `F[A-Z0-9]{3}-\d{1,8}`  | `sendBill`     |
| 03   | Boleta de venta         | `/Invoice`          | 2.1 | 2.0             | `B[A-Z0-9]{3}-\d{1,8}`  | `sendSummary` (vía RC) |
| 07   | Nota de Crédito         | `/CreditNote`       | 2.1 | 2.0             | `[FB][A-Z0-9]{3}-\d{1,8}` | `sendBill`   |
| 08   | Nota de Débito          | `/DebitNote`        | 2.1 | 2.0             | `[FB][A-Z0-9]{3}-\d{1,8}` | `sendBill`   |
| RC   | Resumen Diario          | `/SummaryDocuments` | 2.0 | 1.1             | `RC-YYYYMMDD-NNNNN`     | `sendSummary` (async) |
| RA   | Comunicación de Baja    | `/VoidedDocuments`  | 2.0 | 1.0             | `RA-YYYYMMDD-NNNNN`     | `sendSummary` (async) |
| RR   | Resumen de Reversión (Ret/Per) | `/VoidedDocuments` | 2.0 | 1.0    | `RR-YYYYMMDD-NNNNN`     | `sendSummary` (async) |
| 09 / 31 | Guía de Remisión (transportista / remitente) | `/DespatchAdvice` | 2.1 | 2.0 | `T[A-Z0-9]{3}-\d{1,8}` | `sendBill` (endpoint distinto) |
| 20 / 40 | Retención / Percepción | (Retention/Perception) | 2.0 | — | `R[A-Z0-9]{3} / P[A-Z0-9]{3}` | `sendBill` (endpoint distinto) |
| LT   | Lote de Facturas/Notas  | n/a (multi-XML zip) | —   | —               | `LT-YYYYMMDD-NNNNN`     | `sendPack` (async, máx 500 docs) |

> **Invariant:** RC, RA, RR, and Retención/Percepción use **UBL 2.0**.
> Factura/Boleta/Notas/Guía use **UBL 2.1**.

---

## 2. Endpoints (WSDL)

### Production
| Service | URL |
| --- | --- |
| Factura, Boleta, Notas, RC, RA, Lotes | `https://e-factura.sunat.gob.pe/ol-ti-itcpfegem/billService?wsdl` |
| Retención / Percepción / RR | `https://e-factura.sunat.gob.pe/ol-ti-itemision-otroscpe-gem/billService?wsdl` |
| Guía de Remisión | `https://e-guiaremision.sunat.gob.pe/ol-ti-itemision-guia-gem/billService?wsdl` |
| Consulta validez CPE | `https://e-factura.sunat.gob.pe/ol-it-wsconsvalidcpe/billValidService?wsdl` |
| Consulta CDR / estado | `https://e-factura.sunat.gob.pe/ol-it-wsconscpegem/billConsultService?wsdl` |

### Homologación
`https://www.sunat.gob.pe/ol-ti-itcpgem-sqa/billService`

### Beta (testing only — no real cert needed)
| Service | URL |
| --- | --- |
| Factura | `https://e-beta.sunat.gob.pe/ol-ti-itcpfegem-beta/billService?wsdl` |
| Guía    | `https://e-beta.sunat.gob.pe/ol-ti-itemision-guia-gem-beta/billService?wsdl` |
| Retenciones | `https://e-beta.sunat.gob.pe/ol-ti-itemision-otroscpe-gem-beta/billService?wsdl` |

**Beta credentials:** `Username = {RUC}MODDATOS`, `Password = MODDATOS`.

---

## 3. File & ZIP Naming (STRICT)

### Comprobantes (01, 03, 07, 08, 09, 31)
```
{RUC11}-{TT}-{SERIE4}-{CORR}.{xml|zip}
```
* `RUC11` — 11-digit RUC of the issuer
* `TT` — document type code (01, 03, 07, 08, 09, 31)
* `SERIE4` — `F###`, `B###`, `T###` (4 chars total)
* `CORR` — correlative, 1–8 digits, **no left padding required**

Examples:
```
20100066603-01-F001-1.xml
20100066603-07-F001-00000001.xml
20100066603-08-F001-1.zip
```

### Resumen / Baja / Reversión (RC, RA, RR)
```
{RUC11}-{RC|RA|RR}-{YYYYMMDD}-{CORR}.{xml|zip}
```
`CORR` is 1–5 digits. Since 2018-01-01, **RC must be sent in blocks of max 500 lines per file**, each block as a different correlative (envíos are complementary, not replacements).

Examples:
```
20100066603-RC-20110522-1.xml
20100066603-RA-20110522-1.zip
```

### Lotes (LT)
```
{RUC11}-LT-{YYYYMMDD}-{CORR}.zip
```
ZIP contains up to **500** XML files (mix of 01/07/08), all from the same emisor RUC.

### Contingencia impresa (TXT, not XML)
```
{RUC11}-RF-{DDMMYYYY}-{NN}.{txt|zip}
```
`NN` = 01..99. Pipe-delimited (`|`) records per row.

---

## 4. ZIP Structure

Each ZIP contains only the signed XML — nothing else:

```
{RUC}-{TT}-{SERIE}-{CORR}.zip
└── {RUC}-{TT}-{SERIE}-{CORR}.xml
```

> **Note:** Older SUNAT manuals reference an empty `dummy/` directory alongside the XML. In practice, current SUNAT validators count `dummy/` as an extra entry and reject the submission with error **0158** (*"El archivo ZIP contiene demasiados comprobantes"*). Submit the XML alone.

Rules:
* Single-comprobante ZIPs (sendBill, sendSummary): exactly **one XML**, no extra entries.
* Lote ZIPs (sendPack): up to **500 XMLs**, all matching the issuer RUC and consistent with the LT filename.
* XML filename (minus extension) must match the ZIP filename (minus extension).
* Filenames are case-sensitive on the SUNAT side — stick to the convention shown above.

---

## 5. Mandatory XML Header

### Factura / Boleta (UBL 2.1)
```xml
<?xml version="1.0" encoding="ISO-8859-1" standalone="no"?>
<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
         xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
         xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"
         xmlns:ext="urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2"
         xmlns:ds="http://www.w3.org/2000/09/xmldsig#">

  <ext:UBLExtensions>
    <ext:UBLExtension>
      <ext:ExtensionContent>
        <!-- ds:Signature inserted here AFTER everything else is built -->
      </ext:ExtensionContent>
    </ext:UBLExtension>
  </ext:UBLExtensions>

  <cbc:UBLVersionID>2.1</cbc:UBLVersionID>
  <cbc:CustomizationID>2.0</cbc:CustomizationID>
  <cbc:ProfileID
      schemeName="SUNAT:Identificador de Tipo de Operación"
      schemeAgencyName="PE:SUNAT"
      schemeURI="urn:pe:gob:sunat:cpe:see:gem:catalogos:catalogo17">0101</cbc:ProfileID>
  <cbc:ID>F001-1</cbc:ID>
  <cbc:IssueDate>2026-05-04</cbc:IssueDate>
  <cbc:IssueTime>10:30:00</cbc:IssueTime>
  <cbc:InvoiceTypeCode
      listAgencyName="PE:SUNAT"
      listName="SUNAT:Identificador de Tipo de Documento"
      listURI="urn:pe:gob:sunat:cpe:see:gem:catalogos:catalogo01">01</cbc:InvoiceTypeCode>
  <cbc:DocumentCurrencyCode>PEN</cbc:DocumentCurrencyCode>

  <cac:Signature>...</cac:Signature>          <!-- UBL signature reference, MANDATORY -->
  <cac:AccountingSupplierParty>...</cac:AccountingSupplierParty>
  <cac:AccountingCustomerParty>...</cac:AccountingCustomerParty>
  <cac:TaxTotal>...</cac:TaxTotal>
  <cac:LegalMonetaryTotal>...</cac:LegalMonetaryTotal>
  <cac:InvoiceLine>...</cac:InvoiceLine>
</Invoice>
```

### Nota de Crédito (UBL 2.1, root `/CreditNote`)
Use `cbc:CreditNoteTypeCode` (catalog 09) instead of `InvoiceTypeCode`. Add `cac:DiscrepancyResponse` (with `cbc:ResponseCode` from catalog 09) and `cac:BillingReference` pointing to the original document.

### Nota de Débito (UBL 2.1, root `/DebitNote`)
Use `cbc:DebitNoteTypeCode` (catalog 10). Same `DiscrepancyResponse` + `BillingReference` pattern.

> **Document type code (`01`, `03`, `07`, `08`) lives in different elements depending on root.** Don't conflate with `ProfileID` (which is the operation type, e.g. `0101 = Venta interna`).

---

## 6. SUNAT Catalogs (most-used)

| Catalog | Purpose                              | Used in element |
| ------- | ------------------------------------ | --------------- |
| 01      | Tipo de documento (CPE)              | `cbc:InvoiceTypeCode`, `cbc:DocumentTypeCode` |
| 02      | Monedas (ISO 4217)                   | `currencyID` attrs |
| 05      | Tipos de tributo                     | `cac:TaxScheme/cbc:ID` |
| 06      | Tipo de documento de identidad       | `schemeID` of `PartyIdentification/cbc:ID` |
| 07      | Tipos de afectación al IGV           | `cbc:TaxExemptionReasonCode` |
| 08      | Tipos de sistema de cálculo del ISC  | `cbc:TierRange` |
| 09      | Tipo de nota de crédito              | `cbc:ResponseCode` (en CreditNote) |
| 10      | Tipo de nota de débito               | `cbc:ResponseCode` (en DebitNote) |
| 12      | Tipo de operación (guía)             | — |
| 16      | Tipo de precio                       | `cbc:PriceTypeCode` (`01` = con IGV, `02` = gratuito) |
| 17      | Tipo de operación (factura)          | `cbc:ProfileID` (`0101` = Venta interna) |
| 51      | Tipo de operación (catálogo extendido) | `cbc:ProfileID` (algunos casos) |
| 52      | Códigos de leyendas                  | `cbc:Note/@languageLocaleID` (`1000`, `2000`, etc.) |

### Catalog 06 — document identity (`schemeID`)
| Code | Doc type |
| --- | --- |
| 0 | Doc. trib. no domiciliado, sin RUC |
| 1 | DNI |
| 4 | Carnet de extranjería |
| 6 | RUC |
| 7 | Pasaporte |
| A | Cédula diplomática |

### Catalog 07 — IGV affectation (most common)
| Code | Meaning |
| --- | --- |
| 10 | Gravado — Operación onerosa |
| 20 | Exonerado — Operación onerosa |
| 30 | Inafecto — Operación onerosa |
| 11–17 | Gravado — Retiro / bonificación / premios (gratuitas) |
| 21 | Exonerado — Transferencia gratuita |
| 31–37 | Inafecto — gratuitos / retiros |
| 40 | Exportación |

### TaxScheme IDs (catalog 05) — used in `cac:TaxScheme/cbc:ID`
| Tributo | ID  | UN/ECE 5153 | Name |
| ------- | --- | ----------- | ---- |
| IGV     | 1000 | VAT  | IGV |
| ISC     | 2000 | EXC  | ISC |
| ICBPER  | 7152 | OTH  | ICBPER (bolsas plásticas) |
| Otros   | 9999 | OTH  | Otros tributos |
| Exoneración | 9997 | VAT | EXO |
| Inafecto    | 9998 | FRE | INA |
| Exportación | 9995 | FRE | EXP |
| Gratuito    | 9996 | FRE | GRA |

---

## 7. Tax Rules

* **IGV nominal rate:** 18% (as of 2026-05; verify current rate before deploy).
* SUNAT validates against **declared values**, not by recomputing — but it cross-checks for internal consistency. Keep your own computation precise.
* **Tolerance:** ±0.01 (1 céntimo) on totals.
* **Number formats:**
  * Monetary totals: `n(12,2)` — up to 12 integer digits + 2 decimals.
  * Unit values / `cbc:PriceAmount`: `n(12,10)` — up to 10 decimals allowed.
  * Quantities: `n(23,10)`.
* **Currency must match across the document** — `DocumentCurrencyCode`, every `@currencyID`, and `LegalMonetaryTotal` must agree.
* **`cbc:PriceTypeCode` (catalog 16):**
  * `01` — normal billable price (includes IGV in `PricingReference/AlternativeConditionPrice`).
  * `02` — free/gratuito (use unit value `0.00` in `cbc:PriceAmount` of `cac:Price`, declare reference value separately).

### Lines — invariants
* At least one `cac:InvoiceLine` (or `cac:CreditNoteLine` / `cac:DebitNoteLine`).
* Each line has at least one `cac:TaxTotal` with one or more `cac:TaxSubtotal`.
* `cbc:InvoicedQuantity` (or equivalent) must be `> 0`.
* Each line must declare its `cac:TaxCategory/cac:TaxScheme` (IGV scheme + affectation code from catalog 07).

---

## 8. Customer Identification Rules

| Document | Customer identity | Notes |
| -------- | ----------------- | ----- |
| Factura (01) | RUC required (`schemeID="6"`) | Exception: export operations |
| Boleta (03) | Any catalog 06 type allowed | DNI typical |
| Boleta > S/ 700 | Identity **mandatory** | Below this threshold, identity may be omitted (with restrictions) |
| Notas (07/08) | Same as referenced doc | Plus `cac:BillingReference` to original |

---

## 9. Digital Signature (XMLDSig, enveloped)

### Cert requirements
* **X.509 v3**, RSA, **min 2048-bit private key** (manual still cites 1024 from 2012; current SUNAT enforcement is 2048+).
* RUC must appear in **OU (Organizational Unit)** of the Subject DN.
* Cert must be **registered with SUNAT** via Menú SOL → "Actualización de certificado digital" before production use.
* Cert must be valid (not expired, not revoked) at signing time.

### Where the signature goes
1. Build the entire XML *including* `cac:Signature` (UBL reference block) and an **empty** `<ext:ExtensionContent>` placeholder inside `<ext:UBLExtensions>/<ext:UBLExtension>`.
2. Compute the enveloped XMLDSig over the **whole document** (root element: `Invoice` / `CreditNote` / `DebitNote` / `SummaryDocuments` / `VoidedDocuments`).
3. Insert the resulting `<ds:Signature>` inside the empty `<ext:ExtensionContent>`.
4. **Do not modify the XML after signing** — any whitespace change invalidates the signature.

### Required transforms / algorithms
```xml
<ds:CanonicalizationMethod Algorithm="http://www.w3.org/TR/2001/REC-xml-c14n-20010315"/>
<ds:SignatureMethod        Algorithm="http://www.w3.org/2000/09/xmldsig#rsa-sha1"/>  <!-- minimum -->
<ds:Reference URI="">
  <ds:Transforms>
    <ds:Transform Algorithm="http://www.w3.org/2000/09/xmldsig#enveloped-signature"/>
  </ds:Transforms>
  <ds:DigestMethod Algorithm="http://www.w3.org/2000/09/xmldsig#sha1"/>
  <ds:DigestValue>...</ds:DigestValue>
</ds:Reference>
```
> SHA-1/RSA-SHA1 are what the manual examples ship. Newer cert authorities sign with SHA-256; SUNAT accepts `rsa-sha256` + `sha256` digests. Stick to whatever your cert chain permits.

### Encoding rule
The signature must be computed using the **same encoding** as the document (`ISO-8859-1`). Canonicalize, then sign — do **not** re-encode after canonicalization.

### `cac:Signature` block (UBL reference, mandatory regardless)
```xml
<cac:Signature>
  <cbc:ID>SignSUNAT</cbc:ID>
  <cac:SignatoryParty>
    <cac:PartyIdentification><cbc:ID>{RUC}</cbc:ID></cac:PartyIdentification>
    <cac:PartyName><cbc:Name>{RAZON_SOCIAL}</cbc:Name></cac:PartyName>
  </cac:SignatoryParty>
  <cac:DigitalSignatureAttachment>
    <cac:ExternalReference><cbc:URI>#SignSUNAT</cbc:URI></cac:ExternalReference>
  </cac:DigitalSignatureAttachment>
</cac:Signature>
```
The `ds:Signature/@Id` (in the actual signature placed in `ExtensionContent`) must match this URI fragment (`SignSUNAT`).

---

## 10. SOAP Flow

```
build XML  →  sign  →  zip  →  base64  →  SOAP envelope  →  POST  →  CDR (zip → ApplicationResponse)
```

### WS-Security header (UsernameToken)
```xml
<soapenv:Header>
  <wsse:Security>
    <wsse:UsernameToken>
      <wsse:Username>{RUC}{USUARIO_SOL}</wsse:Username>   <!-- concatenated, no separator -->
      <wsse:Password>{CLAVE_SOL}</wsse:Password>
    </wsse:UsernameToken>
  </wsse:Security>
</soapenv:Header>
```
Clave SOL must be a **secondary user** with profile `"Envío de documentos electrónicos - Grandes emisores"`. All transport via HTTPS/TLS.

### Methods

#### `sendBill` — synchronous
**In:** `fileName` (string), `contentFile` (base64 ZIP).
**Out:** base64 ZIP containing the CDR (`R-{originalName}.xml` ApplicationResponse).
**Used for:** Factura, Notas, Retención, Percepción, Guía.

#### `sendSummary` — asynchronous
**In:** same as sendBill.
**Out:** `ticket` string (poll later with `getStatus`).
**Used for:** RC (Resumen Diario de Boletas), RA (Comunicación de Baja), RR.

#### `sendPack` — asynchronous (lotes)
**In:** ZIP with up to 500 XMLs.
**Out:** `ticket` (poll with `getStatus`, returns ZIP with one CDR per document plus a summary report).

#### `getStatus` — ticket polling
**In:** `ticket`.
**Out:** `StatusResponse { statusCode, content }`:
| `statusCode` | Meaning |
| --- | --- |
| `0`  | Procesado correctamente — `content` = ZIP with CDR |
| `98` | En proceso — retry later |
| `99` | Procesado con errores — `content` = ZIP with CDR (rechazado) |

#### `getStatusCdr` — direct CDR query (sync, by document identity)
**In:** `rucComprobante`, `tipoComprobante` (01/07/08), `serieComprobante`, `numeroComprobante`.
**Out:** `StatusCdr { statusCode, content, statusMessage }`. `content` is the CDR ZIP.
> Only Factura / Notas (series starting with `F`) are supported by `getStatusCdr`.

#### `billConsultService` — validity / status query
| `statusCode` | Meaning |
| --- | --- |
| `0001` | Aceptado |
| `0002` | Rechazado |
| `0003` | Dado de baja |
| `0004`–`0012` | Validation / lookup errors (see manual Anexo 2) |

---

## 11. CDR (ApplicationResponse) — parsing

The CDR is a UBL 2.0 `ApplicationResponse`, signed by SUNAT, returned inside a ZIP named `R-{originalName}.zip` containing `R-{originalName}.xml`.

### Key fields
| XPath | Meaning |
| --- | --- |
| `/ApplicationResponse/cbc:ID` | SUNAT reception process ID (15 digits) |
| `/ApplicationResponse/cbc:UBLVersionID` | Always `2.0` |
| `/ApplicationResponse/cbc:CustomizationID` | Always `1.0` |
| `/ApplicationResponse/cbc:ResponseDate`, `cbc:ResponseTime` | When CDR was issued |
| `/ApplicationResponse/cbc:Note` | **Observaciones** — warnings (`code-description`), accepted-with-warnings only |
| `/ApplicationResponse/cac:DocumentResponse/cac:Response/cbc:ResponseCode` | `0` = aceptado, otherwise rejected |
| `/ApplicationResponse/cac:DocumentResponse/cac:Response/cbc:Description` | Human-readable status |
| `/ApplicationResponse/cac:DocumentResponse/cac:DocumentReference/cbc:ID` | Echo of the processed doc ID |

### Three CDR outcomes
1. **Aceptado** — `ResponseCode = 0`, no `cbc:Note`. Document is registered.
2. **Aceptado con observaciones** — `ResponseCode = 0`, with one or more `cbc:Note` (warning codes ≥ 4000). Document is **valid**; observations are advisory.
3. **Rechazado** — `ResponseCode != 0` (codes 2000–3999). Document is **not** registered.

---

## 12. Error Code Ranges

| Range       | Class | Behavior |
| ----------- | ----- | -------- |
| `0100`–`0999` | Excepciones SUNAT | SOAP Fault, no CDR. Server-side problem; retry is reasonable. |
| `1000`–`1999` | Excepciones contribuyente (formato/estructura) | SOAP Fault. Fix the XML/ZIP and resend. |
| `2000`–`3999` | Errores → CDR rechazado | CDR returned, document not registered. **For Factura/Notas the correlative is consumed** — must issue a new number. For Retención/Percepción/Guía/RC/RA/RR the whole document is rejected and you may resend with the same name. |
| `4000`+     | Observaciones | CDR aceptado **con advertencia**. Document is valid. |

### SOAP Fault shape
```xml
<soap-env:Fault>
  <faultcode>soap-env:Server.{code}</faultcode>   <!-- or Client.{code} -->
  <faultstring>{description}</faultstring>
</soap-env:Fault>
```
* `Server.*` — SUNAT-side issue.
* `Client.*` — request malformed (auth, schema, signature, encoding).

---

## 13. Common Gotchas (battle-tested)

1. **Encoding lock-in.** XML declaration must be `ISO-8859-1`. Bytes-on-the-wire must match. UTF-8 with `ñ` or accents → rejected.
2. **Sign last, change nothing after.** Any whitespace, attribute reorder, or namespace prefix change after signing breaks the digest.
3. **Series doesn't encode the doc type.** `F001` can be a Factura *or* a Nota de Crédito on a Factura. `cbc:InvoiceTypeCode` / `cbc:CreditNoteTypeCode` is the source of truth — use it to route, not the series prefix.
4. **`ProfileID` ≠ document type.** ProfileID is the *operation* (catalog 17, e.g. `0101` Venta interna). The doc type code is in `InvoiceTypeCode` (catalog 01).
5. **`UBLVersionID` mismatch by root.** Invoice/CreditNote/DebitNote/DespatchAdvice = `2.1`; SummaryDocuments/VoidedDocuments = `2.0`. `CustomizationID`: comprobantes = `2.0`, RC = `1.1`, RA/RR = `1.0`.
6. **Boletas don't go via `sendBill`.** Submit them in batches via Resumen Diario (RC) — unless you're using SEE-SOL portal mode.
7. **Filename casing.** `.XML`/`.ZIP` uppercase or `.xml`/`.zip` lowercase both work, but the **base name** (RUC-TT-SERIE-CORR) must match between zip and xml exactly.
8. **Duplicate IDs.** Once a `{RUC, TipoDoc, Serie, Correlativo}` is **accepted**, that number is consumed forever. Rechazos on Factura/Nota also burn the number — generate a new one.
9. **Tolerance is per-line and per-total.** ±0.01 each. Don't accumulate rounding across many lines without compensating on the totals.
10. **`cac:Signature` is mandatory in the body** even though the actual XMLDSig lives in `<ext:UBLExtensions>`. They are two different blocks.
11. **WS-Security `Username` is concatenated.** `{RUC}{usuario_sol}` with **no separator**. The 11-digit RUC is glued directly to the SOL user.
12. **Resumen Diario blocks ≤ 500 lines** since 2018-01-01. Split larger days into multiple correlatives — they accumulate, they don't replace.
13. **Lote `sendPack`** ≤ 500 docs per ZIP, all from same RUC. Mix of 01/07/08 OK.

---

## 14. Go Implementation Patterns

### Suggested package layout
```
/internal
  /ubl          — XML structs (Invoice, CreditNote, DebitNote, SummaryDocuments, VoidedDocuments, ApplicationResponse)
  /catalogs     — typed enums for cat 01, 06, 07, 16, 17, 51, 52, TaxSchemes
  /sign         — XMLDSig enveloped signer (RSA-SHA1 + RSA-SHA256), c14n
  /pack         — ISO-8859-1 marshaling, ZIP packaging (signed XML only)
  /soap         — WS-Security UsernameToken, sendBill/sendSummary/getStatus/getStatusCdr clients
  /cdr          — ApplicationResponse parser, error classification
  /validate     — pre-send schema + business-rule checks
```

### Encoding pipeline
```go
// 1. Marshal to UTF-8
buf, _ := xml.MarshalIndent(invoice, "", "")

// 2. Convert to ISO-8859-1 (use golang.org/x/text/encoding/charmap)
encoded, err := charmap.ISO8859_1.NewEncoder().Bytes(buf)

// 3. Replace XML declaration
encoded = bytes.Replace(encoded,
    []byte(`<?xml version="1.0" encoding="UTF-8"?>`),
    []byte(`<?xml version="1.0" encoding="ISO-8859-1" standalone="no"?>`),
    1)

// 4. Sign (enveloped XMLDSig over the full document)
signed, err := signer.Sign(encoded, certKey, certX509)

// 5. Zip the signed XML alone (no dummy/ folder — triggers SUNAT error 0158)
zipBytes, err := pack.Build(filename, signed)

// 6. Base64-wrap into SOAP and POST
```

### XMLDSig — practical advice
* **Use `github.com/beevik/etree`** for tree manipulation when injecting `<ds:Signature>` into the placeholder.
* **Don't roll your own canonicalization.** Use `github.com/russellhaering/goxmldsig` or wrap a tested C14N implementation. Exclusive C14N (`xml-exc-c14n`) is what most CAs deliver, but **SUNAT examples use inclusive C14N (`xml-c14n-20010315`)** — match what your cert/library expects and what the SUNAT validator accepts (inclusive is safer).
* **Sign over the whole document**, `URI=""`, with one `enveloped-signature` transform.

### Validate before sending
Run XSD validation locally (libxml2 via cgo or a Go XSD validator) plus business-rule checks (RUC checksum, totals tolerance, catalog code membership) before the ZIP/sign step. Rechazos burn correlatives — fail fast in the app layer.

### Idempotency
Track `(ruc, tipo_doc, serie, correlativo)` → CDR locally. On retry after a network failure, hit `getStatusCdr` (sync, no ticket needed) before resubmitting — SUNAT's response is authoritative and prevents double-burning a correlative.

### Async polling (sendSummary, sendPack)
Backoff: start 5s, double up to ~60s, cap total wait at ~10 min. `statusCode=98` is the only "keep polling" signal; `0` and `99` both have `content` and end the loop.

---

## 15. Contingencia (printed-doc fallback)

When the issuer cannot emit electronically (force majeure, system down), they may issue printed comprobantes from a SUNAT-authorized print shop and later report them via SEE-SOL portal:

1. Build a **TXT file** (pipe-delimited, one record per line) per Anexo 11 of the manual.
2. Filename: `{RUC}-RF-{DDMMYYYY}-{NN}.txt`, then ZIP it (same base name).
3. Upload via SUNAT Operaciones en Línea (manual web flow, not WS).
4. SUNAT returns a **ticket** + later an error file if any rows fail.

> The **last** RF file submitted for a date **completely replaces** all previous submissions for that date — design your local state accordingly.

For Retención/Percepción contingencia: same flow, types `40` / `41` / `20`, filename pattern `{RUC}-{TT}-{YYYYMMDD}-{NN}.txt`.

---

## 16. Quick Reference — XML elements by frequency

| Element | Where | Notes |
| ------- | ----- | ----- |
| `cbc:UBLVersionID`, `cbc:CustomizationID`, `cbc:ProfileID` | header | see §1, §6 |
| `cbc:ID` | header | series-correlativo or RC/RA filename |
| `cbc:IssueDate`, `cbc:IssueTime` | header | `YYYY-MM-DD`, `HH:MM:SS` |
| `cbc:InvoiceTypeCode` / `CreditNoteTypeCode` / `DebitNoteTypeCode` | header | catalog 01 / 09 / 10 |
| `cbc:DocumentCurrencyCode` | header | ISO 4217 |
| `cac:AccountingSupplierParty` | header | emisor: RUC (cat 06 = `6`), razón social, dirección |
| `cac:AccountingCustomerParty` | header | cliente: doc id (cat 06), nombre |
| `cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory/cac:TaxScheme` | totals + lines | tax scheme ID (cat 05), affectation (cat 07) |
| `cac:LegalMonetaryTotal` | totals | `LineExtensionAmount`, `TaxInclusiveAmount`, `PayableAmount`, etc. |
| `cac:InvoiceLine` (or `CreditNoteLine`/`DebitNoteLine`) | per line | qty, price, taxes |
| `cac:PricingReference/cac:AlternativeConditionPrice/cbc:PriceTypeCode` | per line | catalog 16 (`01`/`02`) |
| `cac:BillingReference` | nota only | reference to original Invoice/Boleta |
| `cac:DiscrepancyResponse` | nota only | reason code (cat 09 / 10) |

---

## 17. Verification checklist before going live

- [ ] Cert registered with SUNAT (Menú SOL).
- [ ] Beta endpoint exercised end-to-end (send → CDR aceptado).
- [ ] Homologación endpoint passed (SUNAT certifies the issuer).
- [ ] All XSD validations pass locally.
- [ ] ISO-8859-1 round-trip tested with `ñ` and accents.
- [ ] Signature verifies under both the `xmlsec1` CLI and SUNAT's beta receiver.
- [ ] CDR parser handles aceptado / aceptado-con-observaciones / rechazado / SOAP fault.
- [ ] Idempotent retry via `getStatusCdr` implemented.
- [ ] RC ≤ 500 lines per file enforced; LT ≤ 500 docs per ZIP enforced.
- [ ] Correlative state persisted with row-level locking — no double issuance.