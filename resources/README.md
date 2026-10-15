# SUNAT reference material

The official SUNAT files the XML spec suite (`internal/xmlbuilder/spec/`) is checked against. None of it ships in the service image (`.dockerignore` excludes this folder).

| path | what | SUNAT version / date | where to get a newer one |
|---|---|---|---|
| `reglas/reglas-validacion-cpe-2026-08-26.xlsx` | CPE validation rules: one sheet per document (Factura2_0, Boleta2_0, NotaCredito2_0, NotaDebito2_0, Resumen Diario1_1, Comunicación de Baja1_0, …) with XPath + return code, plus **Catálogos**, CódigosRetorno and Control de Cambios | actualizado al 26.08.2026 | [Guías y manuales](https://cpe.sunat.gob.pe/guias-y-manuales) › Reglas de validación |
| `reglas/reglas-validacion-gre-2026-09-25.xlsx` | GRE validation rules: Guía-Remitente2_0, Guía-Transportista2_0, Catálogos | publicado al 25.09.2026 | [Sistemas de emisión](https://cpe.sunat.gob.pe/node/116) › Reglas de validación |
| `xsl/cpe/` | SUNAT's executable validators `ValidaExprReg*.xsl` for factura/boleta/NC/ND (2.0.1), Resumen Diario (1.1.0, 1.0.1), bajas, percepción/retención | Archivo XSL 2.1, actualizado al 06/09/2022 | [Guías y manuales](https://cpe.sunat.gob.pe/guias-y-manuales) › Archivos XSL - XSD |
| `xsl/gre/` | GRE Remitente / Transportista validators (2.0.1) | Archivos_XSL 2.0.1, actualizado 01.10.2026 | [Sistemas de emisión](https://cpe.sunat.gob.pe/node/116) |
| `xsd/2.1/` | OASIS UBL 2.1 schemas (Invoice, CreditNote, DebitNote, DespatchAdvice, …) | Archivo XSD 2.1, 28/02/2022 (identical to OASIS UBL 2.1 OS) | [Guías y manuales](https://cpe.sunat.gob.pe/guias-y-manuales) |
| `xsd/2.0/` | SUNAT UBL 2.0 schemas: **SummaryDocuments, VoidedDocuments**, `sac` SunatAggregateComponents 1.0/1.1 | Archivo XSD 2.0 | [Guías y manuales](https://cpe.sunat.gob.pe/guias-y-manuales) |
| `guias/guia-xml-{factura,boleta,nota-credito,nota-debito}-2.1-2017-05.pdf` | Guía de elaboración de documentos XML, UBL 2.1 | Versión 1.0, mayo 2017 (latest SUNAT has issued) | [Guías y manuales](https://cpe.sunat.gob.pe/guias-y-manuales) |
| `guias/guia-resumen-diario-1.1-2018-01.pdf` | Guía de elaboración del Resumen Diario 1.1 | Versión 2, enero 2018 | [Guías y manuales](https://cpe.sunat.gob.pe/guias-y-manuales) |
| `guias/manual-programador-2021-05.pdf` | Manual del programador (SOAP services, file naming) | mayo 2021 | [Guías y manuales](https://cpe.sunat.gob.pe/guias-y-manuales) |
| `guias/gre-manual-servicios-rest-2022-10-04.pdf`, `guias/gre-manual-urls-2022-10-25.xlsx` | GRE REST API (token, envío, ticket); for `internal/greclient`, not the XML | 04/10/2022, 25/10/2022 | [Sistemas de emisión](https://cpe.sunat.gob.pe/node/116) |
| `normas/rs-123-2022-sunat.pdf` | R.S. 123-2022/SUNAT: the GRE 2.0 regime | 08.07.2022 | [sunat.gob.pe › legislación](https://www.sunat.gob.pe/legislacion/superin/2022/123-2022.pdf) |

There is no separate GRE "guía de elaboración" PDF for GRE 2.0. The GRE rules workbook and the GRE XSL **are** the structure spec.

## Updating

SUNAT republishes the validation rules several times a year; the guides have not changed since 2017/2018.

1. Drop the new downloads into `resources/incoming/`.
2. Compare each one with what is here: publication date, and for a rules workbook a rule-by-rule diff, including its *Control de Cambios* sheet. Rules with a future effective date are noted in the spec case that depends on them.
3. Replace the outdated file (keep the `name-YYYY-MM-DD` pattern), update this table and every `sources:` reference under `internal/xmlbuilder/spec/`, then delete `incoming/`.

Old versions stay in git history. The February 2026 CPE workbook (`AjustesValidacionesCPEv20260212.xlsx`) and the SFS input-format annex (`AnexosIyII_Formato1.3.4.xlsx`, removed as unrelated to the XML) are in commit `b00a315`.
