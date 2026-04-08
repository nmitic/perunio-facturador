---
description: SUNAT Peru electronic invoicing (facturación electrónica) knowledge base for building a Go backend facturador. Covers UBL 2.1 XML structure, validation rules, digital signature, SOAP services, and SUNAT catalogs.
---

# SUNAT Electronic Invoicing — Go Backend Skill

## 1. Document Types & XML Structure

| Code | Name | Root Element | UBL | CustomizationID | Series Pattern | Submit |
|------|------|-------------|-----|-----------------|----------------|--------|
| 01 | Factura | `/Invoice` | 2.1 | 2.0 | `F[A-Z0-9]{3}` or `[0-9]{1,4}` | sendBill (sync) |
| 03 | Boleta | `/Invoice` | 2.1 | 2.0 | `B[A-Z0-9]{3}` or `[0-9]{1,4}` | sendSummary (async) |
| 07 | Nota de Crédito | `/CreditNote` | 2.1 | 2.0 | `F[C][0-9]{2}` / `B[C][0-9]{2}` | sendBill (sync) |
| 08 | Nota de Débito | `/DebitNote` | 2.1 | 2.0 | `F[D][0-9]{2}` / `B[D][0-9]{2}` | sendBill (sync) |
| RC | Resumen Diario | `/SummaryDocuments` | **2.0** | 1.1 | `RC-YYYYMMDD-NNNNN` | sendSummary (async) |
| RA | Comunicación de Baja | `/VoidedDocuments` | **2.0** | 1.0 | `RA-YYYYMMDD-NNNNN` | sendSummary (async) |

> **WARNING**: RC and RA use UBL **2.0**, NOT 2.1. This is the #1 cause of silent rejections.

### Namespaces

```xml
xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
<!-- CreditNote: ...CreditNote-2, DebitNote: ...DebitNote-2 -->
xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"
xmlns:ext="urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2"
xmlns:ds="http://www.w3.org/2000/09/xmldsig#"
<!-- RC/RA only: -->
xmlns:sac="urn:sunat:names:specification:ubl:peru:schema:xsd:SunatAggregateComponents-1"
```

### Invoice XML Skeleton (annotated for NC/ND differences)

```xml
<?xml version="1.0" encoding="ISO-8859-1"?>
<Invoice xmlns="...Invoice-2" xmlns:cac="..." xmlns:cbc="..." xmlns:ext="...">
  <!-- NC: <CreditNote>, ND: <DebitNote> -->
  <ext:UBLExtensions>
    <ext:UBLExtension><ext:ExtensionContent>
      <!-- ds:Signature injected here after signing -->
    </ext:ExtensionContent></ext:UBLExtension>
  </ext:UBLExtensions>
  <cbc:UBLVersionID>2.1</cbc:UBLVersionID>
  <cbc:CustomizationID>2.0</cbc:CustomizationID>
  <cbc:ID>F001-00000001</cbc:ID>           <!-- NC: FC01-1, ND: FD01-1 -->
  <cbc:IssueDate>2024-01-15</cbc:IssueDate>
  <cbc:IssueTime>15:20:30</cbc:IssueTime>
  <cbc:InvoiceTypeCode listAgencyName="PE:SUNAT" listName="Tipo de Documento"
    listURI="urn:pe:gob:sunat:cpe:see:gem:catalogos:catalogo01">01</cbc:InvoiceTypeCode>
  <!-- NC/ND: omit InvoiceTypeCode, no equivalent needed -->
  <cbc:Note languageLocaleID="1000">MIL SOLES</cbc:Note>  <!-- Monto en letras -->
  <cbc:DocumentCurrencyCode listID="ISO 4217 Alpha" listName="Currency"
    listAgencyName="United Nations Economic Commission for Europe">PEN</cbc:DocumentCurrencyCode>
  <!-- NC/ND only: DiscrepancyResponse -->
  <!-- <cac:DiscrepancyResponse>
    <cbc:ReferenceID>F001-00000001</cbc:ReferenceID>  affected doc
    <cbc:ResponseCode>01</cbc:ResponseCode>            Cat.09 for NC, 01|02 for ND
    <cbc:Description>Anulación de la operación</cbc:Description>
  </cac:DiscrepancyResponse>
  <cac:BillingReference><cac:InvoiceDocumentReference>
    <cbc:ID>F001-00000001</cbc:ID>
    <cbc:DocumentTypeCode>01</cbc:DocumentTypeCode>
  </cac:InvoiceDocumentReference></cac:BillingReference> -->
  <cac:Signature><!-- cac:Signature reference, see Section 3 --></cac:Signature>
  <cac:AccountingSupplierParty><!-- Supplier RUC, name, address --></cac:AccountingSupplierParty>
  <cac:AccountingCustomerParty><!-- Customer ID, name --></cac:AccountingCustomerParty>
  <cac:TaxTotal>
    <cbc:TaxAmount currencyID="PEN">259.11</cbc:TaxAmount>
    <cac:TaxSubtotal><!-- One per tax type: IGV, ISC, OTROS --></cac:TaxSubtotal>
  </cac:TaxTotal>
  <cac:LegalMonetaryTotal>
    <cbc:LineExtensionAmount currencyID="PEN">1439.48</cbc:LineExtensionAmount>
    <cbc:TaxInclusiveAmount currencyID="PEN">1698.59</cbc:TaxInclusiveAmount>
    <cbc:AllowanceTotalAmount currencyID="PEN">0.00</cbc:AllowanceTotalAmount>
    <!-- ND: uses ChargeTotalAmount instead of AllowanceTotalAmount -->
    <cbc:PayableAmount currencyID="PEN">1698.59</cbc:PayableAmount>
  </cac:LegalMonetaryTotal>
  <cac:InvoiceLine>  <!-- NC: CreditNoteLine, ND: DebitNoteLine -->
    <cbc:ID>1</cbc:ID>
    <cbc:InvoicedQuantity unitCode="NIU">50</cbc:InvoicedQuantity>
    <!-- NC: CreditedQuantity, ND: DebitedQuantity -->
    <cbc:LineExtensionAmount currencyID="PEN">1439.48</cbc:LineExtensionAmount>
    <cac:PricingReference><cac:AlternativeConditionPrice>
      <cbc:PriceAmount currencyID="PEN">34.99</cbc:PriceAmount>
      <cbc:PriceTypeCode listName="Tipo de Precio" listAgencyName="PE:SUNAT"
        listURI="urn:pe:gob:sunat:cpe:see:gem:catalogos:catalogo16">01</cbc:PriceTypeCode>
    </cac:AlternativeConditionPrice></cac:PricingReference>
    <cac:TaxTotal><!-- Line-level tax, see Section 2 --></cac:TaxTotal>
    <cac:Item><cbc:Description>PRODUCTO X</cbc:Description></cac:Item>
    <cac:Price><cbc:PriceAmount currencyID="PEN">28.79</cbc:PriceAmount></cac:Price>
  </cac:InvoiceLine>
</Invoice>
```

## 2. Validation Rules

Error code ranges: `0100-0999` SUNAT exceptions | `1000-1999` format/structure | `2000-3999` rejections | `4000+` observations (warnings, doc still accepted).

### Header Fields

| Field | Format | Rule | Error |
|-------|--------|------|-------|
| UBLVersionID | `"2.1"` | Must be exactly "2.1" (RC/RA: "2.0") | 2074, 2075 |
| CustomizationID | `"2.0"` | Must be exactly "2.0" (RC: "1.1", RA: "1.0") | 2072, 2073 |
| ID (Factura) | `[F][A-Z0-9]{3}-[0-9]{1,8}` | Must match filename serie+correlativo | 1001, 1035, 1036 |
| ID (Boleta) | `[B][A-Z0-9]{3}-[0-9]{1,8}` | Same regex but starts with B | 1001 |
| IssueDate | `YYYY-MM-DD` | Max 2 days future (err 2329); late submission deadline (err 2108) | 2108, 2329 |
| IssueTime | `hh:mm:ss` | No active validation | — |
| InvoiceTypeCode | Cat.01 | Must match document type in filename | 1003, 1004 |
| DocumentCurrencyCode | ISO 4217 | Must be consistent across ALL amounts in document | 2071 |

### Supplier (AccountingSupplierParty)

- RUC: 11-digit numeric, `schemeID="6"` (err 1007, 1008)
- Must match RUC in XML filename (err 1034)
- Contributor must be active: `ind_estado = "00"` (err 2010)
- Contributor must be habido: `ind_condicion != "12"` (err 2011)
- `AddressTypeCode`: 4-digit establishment code, "0000" for main (err 3030)
- `RegistrationName`: 1-1500 chars, no whitespace chars except space (err 1037)

### Customer (AccountingCustomerParty)

- **Factura**: customer doc type must be `"6"` (RUC) in most cases (err 2800)
  - Exceptions: exports allow `"-"`, type 0112 allows `"1"` or `"6"`, type 2106 allows `"7"`, `"B"`, `"G"`
- **Boleta**: allows types `0,1,4,6,7,A,B,C,D,E,G` (Cat.06)
- If type `"6"` (RUC): must be 11-digit numeric (err 2017), must exist in SUNAT registry (err 1083)
- If type `"1"` (DNI): must be 8-digit numeric (err 2801)
- **Boleta >S/700**: customer identity document is required

### Amount Formats

- Document totals: `n(12,2)` — 12 integer digits, up to 2 decimals (err 3021)
- Unit prices: `n(12,10)` — 12 integer digits, up to 10 decimals (err 2369)
- All `@currencyID` attributes must match `DocumentCurrencyCode` (err 2071)

### Line Item Rules

- `cbc:ID`: unique numeric, max 3 digits, > 0 (err 2023, 2752)
- `InvoicedQuantity`: positive decimal `n(12,10)`, cannot be 0 (err 2024, 2025)
- Must have exactly one `cac:TaxTotal` per line (err 3195, 3026)
- Must have at least one IGV-affecting `TaxSubtotal` with `TaxableAmount > 0` (err 3105)
- `PriceTypeCode`: `"01"` = unit price (onerosa), `"02"` = referential (gratuita) (err 2410)
- No duplicate `PriceTypeCode` in same line (err 2409)

### Tax Code Combinations per Line (TaxableAmount > 0)

Only these combinations of `TaxScheme/cbc:ID` are valid per line (err 3223):

| Primary | Optional | Meaning |
|---------|----------|---------|
| 1000 (IGV) | 2000, 9999 | Gravado (taxed) |
| 1016 (IVAP) | 9999 | Rice tax |
| 9995 (Exportación) | 9999 | Export |
| 9996 (Gratuita) | 2000, 9999 | Free transfer |
| 9997 (Exonerado) | 2000, 9999 | Exonerated |
| 9998 (Inafecto) | 2000, 9999 | Unaffected |

### Tax Calculation

- **IGV = 18%** of (`TaxableAmount` + ISC amount if present)
- Tolerance: +/- 1 (one cent) on computed vs declared (err 3103)
- If `TaxExemptionReasonCode` indicates gratuita (11-17, 21, 31-37): IGV TaxAmount depends on type
  - Codes 11-17 (gravado gratuita): IGV amount must be > 0 (err 3111)
  - Codes 21, 31-37 (exonerado/inafecto gratuita): IGV amount must be 0 (err 3110)
  - Code 40 (exportación): IGV amount must be 0 (err 3110)

### Common Attribute Constants (OBSERV 4251-4257 if wrong)

| Context | Attribute | Value |
|---------|-----------|-------|
| Identity | `@schemeName` / `@schemeAgencyName` / `@schemeURI` | `"Documento de Identidad"` / `"PE:SUNAT"` / `"urn:pe:gob:sunat:cpe:see:gem:catalogos:catalogo06"` |
| Doc type | `@listAgencyName` / `@listName` / `@listURI` | `"PE:SUNAT"` / `"Tipo de Documento"` / `"urn:pe:gob:sunat:cpe:see:gem:catalogos:catalogo01"` |
| Currency | `@listID` / `@listAgencyName` | `"ISO 4217 Alpha"` / `"United Nations Economic Commission for Europe"` |
| Unit code | `@unitCodeListID` | `"UN/ECE rec 20"` |

## 3. Digital Signature

### Algorithm URIs (exact strings required)

```
Canonicalization: http://www.w3.org/TR/2001/REC-xml-c14n-20010315#WithComments
Signature:       http://www.w3.org/2000/09/xmldsig#rsa-sha1
Digest:          http://www.w3.org/2000/09/xmldsig#sha1
Transform:       http://www.w3.org/2000/09/xmldsig#enveloped-signature
```

### Certificate Requirements

- X.509 v3, minimum 1024-bit RSA key
- RUC must appear in the OU (Organizational Unit) field of the Subject Name
- Must be registered with SUNAT via Menú SOL before use
- Beta environment does NOT require a certificate

### Dual Signature Locations

Two XML structures are required — the actual `ds:Signature` AND a `cac:Signature` reference:

```xml
<!-- Location 1: Actual XMLDSig in UBLExtensions -->
<ext:UBLExtensions><ext:UBLExtension><ext:ExtensionContent>
  <ds:Signature Id="signatureKG">
    <ds:SignedInfo>
      <ds:CanonicalizationMethod Algorithm="http://www.w3.org/TR/2001/REC-xml-c14n-20010315#WithComments"/>
      <ds:SignatureMethod Algorithm="http://www.w3.org/2000/09/xmldsig#rsa-sha1"/>
      <ds:Reference URI="">
        <ds:Transforms>
          <ds:Transform Algorithm="http://www.w3.org/2000/09/xmldsig#enveloped-signature"/>
        </ds:Transforms>
        <ds:DigestMethod Algorithm="http://www.w3.org/2000/09/xmldsig#sha1"/>
        <ds:DigestValue>...</ds:DigestValue>
      </ds:Reference>
    </ds:SignedInfo>
    <ds:SignatureValue>...</ds:SignatureValue>
    <ds:KeyInfo><ds:X509Data>
      <ds:X509Certificate>...base64 cert...</ds:X509Certificate>
    </ds:X509Data></ds:KeyInfo>
  </ds:Signature>
</ext:ExtensionContent></ext:UBLExtension></ext:UBLExtensions>

<!-- Location 2: cac:Signature reference (after DocumentCurrencyCode, before parties) -->
<cac:Signature>
  <cbc:ID>IDSignKG</cbc:ID>
  <cac:SignatoryParty>
    <cac:PartyIdentification><cbc:ID>20100113612</cbc:ID></cac:PartyIdentification>
    <cac:PartyName><cbc:Name><![CDATA[Company Name]]></cbc:Name></cac:PartyName>
  </cac:SignatoryParty>
  <cac:DigitalSignatureAttachment>
    <cac:ExternalReference><cbc:URI>#signatureKG</cbc:URI></cac:ExternalReference>
  </cac:DigitalSignatureAttachment>
</cac:Signature>
```

### Go Notes

- **Encoding**: Marshal UTF-8 with `encoding/xml`, transcode via `charmap.ISO8859_1.NewEncoder()`, replace declaration to `encoding="ISO-8859-1"`.
- **Signing**: Use `github.com/beevik/etree` to inject `ds:Signature` into `ext:ExtensionContent` post-marshal. Do NOT modify after signing.

## 4. SOAP Web Service Flow

### Flow

```
Build XML → Sign (XMLDSig) → ZIP (one XML per ZIP) → SOAP call → Parse CDR
```

### Endpoints

| Environment | URL |
|-------------|-----|
| Beta | `https://e-beta.sunat.gob.pe/ol-ti-itcpfegem-beta/billService` |
| Production | `https://e-factura.sunat.gob.pe/ol-ti-itcpfegem/billService` |
| Consultation | `https://e-factura.sunat.gob.pe/ol-it-wsconscpegem/billConsultService` |

### Methods

| Method | Use For | Params | Response | Mode |
|--------|---------|--------|----------|------|
| `sendBill` | Factura, NC, ND | `fileName`, `contentFile` (base64 ZIP) | `applicationResponse` (base64 ZIP w/ CDR) | Sync |
| `sendSummary` | RC, RA | `fileName`, `contentFile` (base64 ZIP) | `ticket` (string) | Async |
| `getStatus` | Check ticket | `ticket` (string) | `statusCode`, `content` | Poll |
| `getStatusCdr` | Query any CDR | `rucComprobante`, `tipoComprobante`, `serieComprobante`, `numeroComprobante` | `statusCdr` | Sync |
| `sendPack` | Batch (max 500) | `fileName`, `contentFile` | `ticket` | Async |

### getStatus Response Codes

- `0` = Procesado (done, CDR in `content`)
- `98` = En proceso (still processing, retry)
- `99` = Error processing

### WS-Security SOAP Header

```xml
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"
  xmlns:ser="http://service.sunat.gob.pe" xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd">
  <soapenv:Header>
    <wsse:Security>
      <wsse:UsernameToken>
        <wsse:Username>20100113612MODDATOS</wsse:Username>
        <wsse:Password>moddatos</wsse:Password>
      </wsse:UsernameToken>
    </wsse:Security>
  </soapenv:Header>
  <soapenv:Body>
    <ser:sendBill>
      <fileName>20100113612-01-F001-00000001.zip</fileName>
      <contentFile>...base64...</contentFile>
    </ser:sendBill>
  </soapenv:Body>
</soapenv:Envelope>
```

### Beta Environment

- Username: `{RUC}MODDATOS` (e.g., `20100113612MODDATOS`)
- Password: `moddatos`
- No digital certificate needed

### CDR (Constancia de Recepción)

- `ApplicationResponse` UBL 2.0 inside ZIP, named `R-{original-filename}.xml`
- Key field: `cac:DocumentResponse/cac:Response/cbc:ResponseCode` — `0` = accepted
- Error details in `cbc:Description`, same code ranges as validation errors

## 5. File Nomenclature

| Type | XML Filename | ZIP Filename | CDR Filename |
|------|-------------|-------------|-------------|
| Factura/NC/ND | `{RUC}-{TT}-{SERIE}-{CORR}.xml` | `.zip` | `R-{RUC}-{TT}-{SERIE}-{CORR}.xml` |
| Resumen Diario | `{RUC}-RC-{YYYYMMDD}-{NNNNN}.xml` | `.zip` | `R-{RUC}-RC-{YYYYMMDD}-{NNNNN}.xml` |
| Com. de Baja | `{RUC}-RA-{YYYYMMDD}-{NNNNN}.xml` | `.zip` | `R-{RUC}-RA-{YYYYMMDD}-{NNNNN}.xml` |

Example: `20100113612-01-F001-00000001.xml` → zipped → CDR: `R-20100113612-01-F001-00000001.xml`

ZIP contains exactly **one** XML for `sendBill`. Correlativo: up to 8 digits (RC/RA: 5), starts at 1.

## 6. Business Rules & Gotchas

1. **TRAP: Resumen Diario uses UBL 2.0**, not 2.1. `UBLVersionID="2.0"`, `CustomizationID="1.1"`. Uses `sac:` namespace for SUNAT-specific elements.
2. **TRAP: Comunicación de Baja uses UBL 2.0**. `CustomizationID="1.0"`. Root is `VoidedDocuments`.
3. **NC on boletas cannot use motivos 04, 05, 08** (Descuento global, Descuento por item, Bonificación).
4. **ND only has 2 motivos**: `01` = Intereses por mora, `02` = Aumento en el valor. No others.
5. **Boleta customer identity required when total > S/700.00**.
6. **NC tipo 10 ("Otros conceptos") and ND tipo 03 ("Penalidad/Otros") are NOT included** in Resumen Diario.
7. **Resumen Diario deadline**: must be sent within 7 calendar days of document emission date.
8. **Resumen ConditionCode**: `1` = Adicionar (add new), `2` = Modificar (update), `3` = Anular (void).
9. **Resumen is complementary**: new submissions don't replace previous ones. Each adds/modifies/voids specific documents.
10. **Max 500 lines per Resumen** block. Split into multiple if needed.
11. **All Resumen amounts must be in PEN** (Soles), even if original boleta was in USD.
12. **Resumen ReferenceDate**: all documents in one Resumen must share the same emission date.
13. **Series prefixes**: `F___` = Factura, `B___` = Boleta, `FC__`/`BC__` = NC, `FD__`/`BD__` = ND.
14. **CreditNote uses `CreditNoteLine`/`CreditedQuantity`**, not InvoiceLine/InvoicedQuantity.
15. **DebitNote uses `DebitNoteLine`/`DebitedQuantity`**.
16. **NC DiscrepancyResponse**: `ResponseCode` from Catálogo 09 (01-09). `ReferenceID` = affected document number.
17. **ND DiscrepancyResponse**: `ResponseCode` only `01` or `02`.
18. **Comunicación de Baja**: only doc types `01, 07, 08, 30, 34, 42` can be voided. 7-day limit from emission.
19. **Encoding MUST be ISO-8859-1** — not UTF-8. The XML declaration and actual encoding must match.
20. **IssueDate**: cannot be more than 2 days in the future from submission date.
21. **InvoiceTypeCode must match the filename** type code (err 1003). Common copy-paste bug.
22. **Duplicate document check**: SUNAT tracks all sent document IDs. Resending a previously accepted factura with different data = error 1033.

## 7. Go Architecture Patterns

### Recommended Package Structure

```
internal/
  document/    # Invoice, CreditNote, DebitNote, SummaryDocuments, VoidedDocuments builders
  xml/         # XML marshaling, ISO-8859-1 encoding, namespace constants
  signature/   # XMLDSig signing: Sign(xmlBytes, cert, key) -> signedXML
  soap/        # SOAP client + WS-Security UsernameToken
  catalog/     # SUNAT catalog constants (Cat01, Cat05, Cat06, Cat07, etc.)
  validation/  # Pre-submission validation: Validate(doc) -> []ValidationError
  zip/         # ZIP packaging and CDR extraction
```

### Key Interfaces

```go
// DocumentBuilder builds a signed XML document ready for submission
type DocumentBuilder interface {
    Build() ([]byte, error)  // Returns signed ISO-8859-1 XML bytes
}

// Signer signs an XML document with XMLDSig
type Signer interface {
    Sign(doc []byte, cert *x509.Certificate, key *rsa.PrivateKey) ([]byte, error)
}

// SUNATClient communicates with SUNAT SOAP services
type SUNATClient interface {
    SendBill(filename string, zipContent []byte) (*CDR, error)
    SendSummary(filename string, zipContent []byte) (ticket string, err error)
    GetStatus(ticket string) (*CDR, error)
    GetStatusCdr(ruc, tipo, serie string, numero int) (*CDR, error)
}

// CDR represents a parsed Constancia de Recepción
type CDR struct {
    ResponseCode int      // 0 = accepted
    Description  string   // Success/error message
    Notes        []string // Additional observations (4000+ codes)
    Accepted     bool
}

// ValidationError carries a SUNAT error code for programmatic handling
type ValidationError struct {
    Code    int    // SUNAT error code (e.g., 2074)
    Message string
    Field   string // XPath of the offending field
}
```

### ISO-8859-1 Encoding Strategy

Marshal to UTF-8 via `encoding/xml`, then transcode with `charmap.ISO8859_1.NewEncoder().Bytes(utf8XML)`. Replace `encoding="UTF-8"` with `encoding="ISO-8859-1"` in the XML declaration.

### Signature Injection Pattern

1. Marshal structs to UTF-8 XML (leave `ext:ExtensionContent` empty)
2. Parse with `github.com/beevik/etree`, find `ext:ExtensionContent`
3. Compute digest, build `ds:Signature`, insert into `ext:ExtensionContent`
4. Serialize, transcode to ISO-8859-1. **Never modify after signing.**

### Testing

- Use beta environment for integration tests, keep sample XMLs in `testdata/`
- Validate XML against XSD schemas in `resources/XSD Schemas 2.1/`

## 8. Catalog Reference

### Cat.01 — Document Types

| Code | Name |
|------|------|
| 01 | Factura |
| 03 | Boleta de Venta |
| 07 | Nota de Crédito |
| 08 | Nota de Débito |
| 09 | Guía de Remisión Remitente |
| 20 | Comprobante de Retención |
| 31 | Guía de Remisión Transportista |
| 40 | Comprobante de Percepción |

### Cat.05 — Tax Schemes

| Code | Name | UN/ECE ID | TaxTypeCode | TaxCategory ID |
|------|------|-----------|-------------|----------------|
| 1000 | IGV | 1000 | VAT | S |
| 1016 | IVAP | 1016 | VAT | S |
| 2000 | ISC | 2000 | EXC | S |
| 7152 | ICBPER | 7152 | OTH | S |
| 9995 | Exportación | 9995 | FRE | G |
| 9996 | Gratuita | 9996 | FRE | E |
| 9997 | Exonerado | 9997 | VAT | E |
| 9998 | Inafecto | 9998 | FRE | O |
| 9999 | Otros tributos | 9999 | OTH | S |

TaxScheme attributes: `schemeID="UN/ECE 5305"`, `schemeAgencyID="6"`.

### Cat.06 — Identity Document Types

| Code | Name |
|------|------|
| 0 | Doc. Trib. No Dom. Sin RUC |
| 1 | DNI |
| 4 | Carnet de Extranjería |
| 6 | RUC |
| 7 | Pasaporte |
| A | Ced. Diplomática de Identidad |
| B | Doc. Ident. País Residencia (No Dom.) |
| C | Tax Identification Number (TIN) |
| D | Identification Number (IN) |
| E | NITE |

### Cat.07 — IGV Affectation Types

| Code | Name | Tax Code | IGV |
|------|------|----------|-----|
| 10 | Gravado - Onerosa | 1000 | 18% |
| 11-16 | Gravado - Gratuita (retiro, donación, bonificación, etc.) | 9996 | Calc |
| 17 | Gravado - IVAP | 1016 | IVAP |
| 20 | Exonerado - Onerosa | 9997 | 0 |
| 21 | Exonerado - Gratuita | 9996 | 0 |
| 30 | Inafecto - Onerosa | 9998 | 0 |
| 31-37 | Inafecto - Gratuita (retiro, bonificación, muestras, etc.) | 9996 | 0 |
| 40 | Exportación | 9995 | 0 |

### Cat.09 — Nota de Crédito Types

| Code | Name | Allowed on Boleta? |
|------|------|--------------------|
| 01 | Anulación de la operación | Yes |
| 02 | Anulación por error en el RUC | Yes |
| 03 | Corrección por error en la descripción | Yes |
| 04 | Descuento global | **No** |
| 05 | Descuento por ítem | **No** |
| 06 | Devolución total | Yes |
| 07 | Devolución por ítem | Yes |
| 08 | Bonificación | **No** |
| 09 | Disminución en el valor | Yes |

### Cat.16 — Price Type

| Code | Name |
|------|------|
| 01 | Precio unitario (incluye IGV) |
| 02 | Valor referencial unitario en operaciones no onerosas (gratuitas) |

### Cat.51 — Operation Types (most common)

| Code | Name |
|------|------|
| 0101 | Venta interna |
| 0102 | Exportación de bienes |
| 0103 | No domiciliados |
| 0104 | Venta interna - Anticipos |
| 0200 | Exportación de bienes |
| 0201 | Exportación de servicios |

### Cat.52 — Legends

| Code | Name |
|------|------|
| 1000 | Monto en letras (optional) |
| 1002 | Transferencia gratuita (when all lines are gratuitas) |
| 2000 | Comprobante de percepción |
| 3000 | Código interno del software de facturación |
