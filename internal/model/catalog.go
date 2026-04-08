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

// TaxSchemeType holds Cat.05 tax scheme metadata.
type TaxSchemeType struct {
	Code          string
	Name          string
	UNECEID       string
	TaxTypeCode   string
	TaxCategoryID string
}

var (
	TaxIGV         = TaxSchemeType{"1000", "IGV", "1000", "VAT", "S"}
	TaxIVAP        = TaxSchemeType{"1016", "IVAP", "1016", "VAT", "S"}
	TaxISC         = TaxSchemeType{"2000", "ISC", "2000", "EXC", "S"}
	TaxICBPER      = TaxSchemeType{"7152", "ICBPER", "7152", "OTH", "S"}
	TaxExportacion = TaxSchemeType{"9995", "Exportación", "9995", "FRE", "G"}
	TaxGratuita    = TaxSchemeType{"9996", "Gratuita", "9996", "FRE", "E"}
	TaxExonerado   = TaxSchemeType{"9997", "Exonerado", "9997", "VAT", "E"}
	TaxInafecto    = TaxSchemeType{"9998", "Inafecto", "9998", "FRE", "O"}
	TaxOtros       = TaxSchemeType{"9999", "Otros tributos", "9999", "OTH", "S"}
)

var taxSchemeByCode = map[string]TaxSchemeType{
	"1000": TaxIGV,
	"1016": TaxIVAP,
	"2000": TaxISC,
	"7152": TaxICBPER,
	"9995": TaxExportacion,
	"9996": TaxGratuita,
	"9997": TaxExonerado,
	"9998": TaxInafecto,
	"9999": TaxOtros,
}

// TaxSchemeByCode returns the tax scheme for a given Cat.05 code.
func TaxSchemeByCode(code string) (TaxSchemeType, bool) {
	ts, ok := taxSchemeByCode[code]
	return ts, ok
}

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

// Cat51 — Operation Types
const (
	OpVentaInterna   = "0101"
	OpExportBienes   = "0102"
	OpNoDomiciliados = "0103"
	OpAnticipos      = "0104"
	OpExportBienes2  = "0200"
	OpExportServ     = "0201"
)

// Cat52 — Legends
const (
	LegendMontoLetras    = "1000"
	LegendTransfGratuita = "1002"
	LegendPercepcion     = "2000"
	LegendCodInterno     = "3000"
)

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

// XML attribute constants required by SUNAT.
const (
	AttrIdentitySchemeName       = "Documento de Identidad"
	AttrIdentitySchemeAgencyName = "PE:SUNAT"
	AttrIdentitySchemeURI        = "urn:pe:gob:sunat:cpe:see:gem:catalogos:catalogo06"

	AttrDocTypeListAgencyName = "PE:SUNAT"
	AttrDocTypeListName       = "Tipo de Documento"
	AttrDocTypeListURI        = "urn:pe:gob:sunat:cpe:see:gem:catalogos:catalogo01"

	AttrCurrencyListID     = "ISO 4217 Alpha"
	AttrCurrencyListName   = "Currency"
	AttrCurrencyListAgency = "United Nations Economic Commission for Europe"

	AttrUnitCodeListID = "UN/ECE rec 20"

	AttrTaxSchemeID       = "UN/ECE 5305"
	AttrTaxSchemeAgencyID = "6"

	AttrPriceTypeListName   = "Tipo de Precio"
	AttrPriceTypeListAgency = "PE:SUNAT"
	AttrPriceTypeListURI    = "urn:pe:gob:sunat:cpe:see:gem:catalogos:catalogo16"
)
