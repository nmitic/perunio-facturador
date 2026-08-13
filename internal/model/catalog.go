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

// Cat.51, Cat.52 and Cat.54 have moved to sunat-catalogs. The anticipo marker
// 0104 keeps its "stored and reported on, but sent as 0101" behaviour as
// props.wireEquivalent; the detracción -> operación pinning that fault 3129
// enforces is props.operationType on each Cat.54 código; and the fixed leyenda
// text is sunat.Cat52Texto.

// Cat16 — Price Type
const (
	PriceTypeUnitWithIGV = "01"
	PriceTypeReferential = "02"
)

// IGVRate was the IGV rate as a decimal string ("0.18"). Dead — declared here and
// read nowhere. The rate a line declares is sunat.Cat07Percent (a percentage
// string for cbc:Percent); the multiplier the totals use is xmlbuilder.igvRate.

// The Attr* block that used to live here was dead: every constant was declared
// and none was ever read — internal/xmlbuilder spells the attribute values out at
// the element that carries them. It also mislabelled one of them, naming
// "UN/ECE 5305" (the tax *category* code list) AttrTaxSchemeID. Deleted rather
// than migrated: XML attribute boilerplate is not a catálogo.
