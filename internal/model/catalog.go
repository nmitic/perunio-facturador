package model

// Cat01 — Document Types
const (
	DocTypeFactura      = "01"
	DocTypeBoleta       = "03"
	DocTypeNotaCredito  = "07"
	DocTypeNotaDebito   = "08"
	DocTypeGuiaRemision = "09"
	DocTypeRetencion    = "20"
	DocTypePercepcion   = "40"
)

// Cat.05 (tributos) has moved to sunat-catalogs: sunat.Cat05IGV, Cat05Name,
// Cat05TaxTypeCode, Cat05TaxCategoryId, Cat05Rate.

// Cat06 — Identity Document Types
const (
	IdentityDocTribNoRUC      = "0"
	IdentityDNI               = "1"
	IdentityCarnetExtranjeria = "4"
	IdentityRUC               = "6"
	IdentityPasaporte         = "7"
	IdentityCedDiplomatica    = "A"
	IdentityDocPaisResidencia = "B"
	IdentityTIN               = "C"
	IdentityIN                = "D"
	IdentityNITE              = "E"
)

// Consumidor final is the anonymous buyer used on boletas with no identified
// customer: Cat.06 doc type IdentityDocTribNoRUC ("0"), with this number and
// name. Boletas > S/700 may not use it — they require a real DNI or RUC.
const (
	ConsumidorFinalDocNumber = "0"
	ConsumidorFinalName      = "CLIENTES VARIOS"
)

// Cat07 — IGV Affectation Types
const (
	AffectGravadoOnerosa   = "10"
	AffectGravadoGratuita  = "11" // through 16
	AffectGravadoIVAP      = "17"
	AffectExoneradoOnerosa = "20"
	AffectExoneradoGrat    = "21"
	AffectInafectoOnerosa  = "30"
	AffectInafectoGratuita = "31" // through 37
	AffectExportacion      = "40"
)

// TaxCodeForAffectation maps Cat.07 affectation code to the corresponding Cat.05 tax code.
func TaxCodeForAffectation(code string) string {
	switch {
	case code == "10":
		return "1000" // IGV
	case code >= "11" && code <= "16":
		return "9996" // Gratuita
	case code == "17":
		return "1016" // IVAP
	case code == "20":
		return "9997" // Exonerado
	case code == "21":
		return "9996" // Gratuita
	case code == "30":
		return "9998" // Inafecto
	case code >= "31" && code <= "37":
		return "9996" // Gratuita
	case code == "40":
		return "9995" // Exportacion
	default:
		return ""
	}
}

// Cat09 — Nota de Credito Types
var notaCreditoTypes = map[string]string{
	"01": "Anulación de la operación",
	"02": "Anulación por error en el RUC",
	"03": "Corrección por error en la descripción",
	"04": "Descuento global",
	"05": "Descuento por ítem",
	"06": "Devolución total",
	"07": "Devolución por ítem",
	"08": "Bonificación",
	"09": "Disminución en el valor",
}

// NCTypesNotAllowedOnBoleta are NC reason codes that cannot be used on boletas.
var NCTypesNotAllowedOnBoleta = map[string]bool{
	"04": true,
	"05": true,
	"08": true,
}

// Cat10 — Nota de Debito Types (only 2 allowed)
var notaDebitoTypes = map[string]string{
	"01": "Intereses por mora",
	"02": "Aumento en el valor",
}

// ValidNCType checks if the code is a valid Cat.09 NC type.
func ValidNCType(code string) bool {
	_, ok := notaCreditoTypes[code]
	return ok
}

// ValidNDType checks if the code is a valid Cat.10 ND type.
func ValidNDType(code string) bool {
	_, ok := notaDebitoTypes[code]
	return ok
}

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

// IGVRate is the IGV rate as a decimal (18%).
const IGVRate = "0.18"

// VoidableDocTypes are document types that can be included in a Comunicacion de Baja.
var VoidableDocTypes = map[string]bool{
	"01": true,
	"07": true,
	"08": true,
	"30": true,
	"34": true,
	"42": true,
}

// The Attr* block that used to live here was dead: every constant was declared
// and none was ever read — internal/xmlbuilder spells the attribute values out at
// the element that carries them. It also mislabelled one of them, naming
// "UN/ECE 5305" (the tax *category* code list) AttrTaxSchemeID. Deleted rather
// than migrated: XML attribute boilerplate is not a catálogo.
