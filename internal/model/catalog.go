package model

// Cat.01 (tipo de documento) has moved to sunat-catalogs: sunat.Cat01Factura,
// Cat01Boleta, Cat01NotaCredito, Cat01NotaDebito and the rest, with
// sunat.Cat01Void replacing the VoidableDocTypes map.
//
// DocTypeRetencion and DocTypePercepcion went with it unreferenced: this service
// emits neither, so they had no callers.

// Cat.05 (tributos) has moved to sunat-catalogs: sunat.Cat05IGV, Cat05Name,
// Cat05TaxTypeCode, Cat05TaxCategoryId, Cat05Rate.

// Cat.06 (tipo de documento de identidad) has moved to sunat-catalogs:
// sunat.Cat06RUC, Cat06DNI, Cat06DocTribNoDomSinRUC and the rest.
//
// The block here named ten códigos and stopped at E; SUNAT's catálogo has
// thirteen (F permiso temporal de permanencia, G salvoconducto, H carné CPP were
// missing). It also mislabelled E as NITE — it is the Tarjeta Andina de
// Migración, so the constant is now sunat.Cat06TAM.

// Consumidor final is the anonymous buyer used on boletas with no identified
// customer: Cat.06 doc type IdentityDocTribNoRUC ("0"), with this number and
// name. Boletas > S/700 may not use it — they require a real DNI or RUC.
const (
	ConsumidorFinalDocNumber = "0"
	ConsumidorFinalName      = "CLIENTES VARIOS"
)

// Cat07 — IGV Affectation Types
// Cat.07 (afectación del IGV) has moved to sunat-catalogs. The Cat.07 -> Cat.05
// mapping this file used to compute with range comparisons is now
// sunat.Cat07TaxSchemeCode, a declared prop on each código; the rate a line
// declares is sunat.Cat07Percent; and the two gratuito predicates are
// sunat.Cat07Gratuito and sunat.Cat07GravadoGratuito.
//
// The Affect* constants went with it. They were dead — nothing outside this file
// ever referred to one, and two of them ("11 // through 16", "31 // through 37")
// stood for ranges a constant cannot express.

// Cat.09 and Cat.10 (motivos de nota) have moved to sunat-catalogs.
// ValidNCType/ValidNDType are sunat.Cat09Emit/sunat.Cat10Emit — `emit` rather
// than `Valid` on purpose: SUNAT defines 13 NC motivos and 5 ND motivos, and
// this pipeline supports 9 and 2 of them. The rest stay displayable so an
// imported or historical nota still renders a motivo name.
//
// NCTypesNotAllowedOnBoleta was a negative map of {04, 05, 08}. It is the
// positive sunat.Cat09OnBoleta now: asking whether a motivo IS allowed reads the
// same way the validator uses it.

// Cat16 — Price Type
const (
	PriceTypeUnitWithIGV = "01"
	PriceTypeReferential = "02"
)

// Cat51 — Operation Types.
//
// SUNAT retired the original 01xx block (beyond 0101) from catálogo 51; sending
// one now fails validation with fault 3206. OpExportBienes/OpNoDomiciliados are
// kept only to document the old codes — use OpExportBienes2/OpExportServ. See
// sunatOperationType in internal/xmlbuilder for how OpAnticipos survives as an
// internal-only marker that never reaches the wire.
const (
	OpVentaInterna = "0101"

	// OpAnticipos marks a factura de anticipo. Retired from catálogo 51, so it
	// is stored and reported on but mapped to OpVentaInterna when building XML.
	OpAnticipos = "0104"

	OpExportBienes   = "0102" // retired — SUNAT rejects it
	OpNoDomiciliados = "0103" // retired — SUNAT rejects it
	OpExportBienes2  = "0200"
	OpExportServ     = "0201"

	// The four detracción operation types. Each of the three specialised ones is
	// pinned to exactly one Cat.54 código — SUNAT rejects any other pairing with
	// fault 3129 ("el dato ingresado como codigo de BBSS de detracción no
	// corresponde al valor esperado"). See detraccionOperationType.
	OpDetraccion                = "1001" // Operación sujeta a detracción (general)
	OpDetraccionHidrobiologicos = "1002" // …recursos hidrobiológicos    — Cat.54 004
	OpDetraccionPasajeros       = "1003" // …transporte de pasajeros     — Cat.54 028
	OpDetraccionTransporteCarga = "1004" // …transporte de carga         — Cat.54 027
)

// Cat54 — Bienes y servicios sujetos a detracción (SPOT).
//
// Only the códigos that change behaviour are named here; the full list lives in
// the frontend catalogue (src/data/sunat/detracciones.ts), which is what
// resolves código, porcentaje and umbral from the productos of a comprobante.
//
// These three are the ones that do NOT declare catálogo 51 "1001". Each also
// drags extra mandatory item-level markup along with it — see
// detraccionOperationType and validateDetraccion.
const (
	DetraccionHidrobiologicos = "004" // → 1002
	DetraccionTransporteCarga = "027" // → 1004
	DetraccionTransportePasaj = "028" // → 1003
)

// Cat52 — Legends
const (
	LegendMontoLetras    = "1000"
	LegendTransfGratuita = "1002"
	LegendPercepcion     = "2000"
	LegendDetraccion     = "2006" // Operación sujeta a detracción
	LegendCodInterno     = "3000"
)

// LegendDetraccionText is the fixed legend text SUNAT expects for a detracción.
const LegendDetraccionText = "Operación sujeta a detracción"

// IGVRate was the IGV rate as a decimal string ("0.18"). Dead — declared here and
// read nowhere. The rate a line declares is sunat.Cat07Percent (a percentage
// string for cbc:Percent); the multiplier the totals use is xmlbuilder.igvRate.

// The Attr* block that used to live here was dead: every constant was declared
// and none was ever read — internal/xmlbuilder spells the attribute values out at
// the element that carries them. It also mislabelled one of them, naming
// "UN/ECE 5305" (the tax *category* code list) AttrTaxSchemeID. Deleted rather
// than migrated: XML attribute boilerplate is not a catálogo.
