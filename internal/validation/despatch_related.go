package validation

import (
	"fmt"
	"regexp"
	"strings"

	sunat "github.com/nmitic/perunio-sunat-catalogs/sunat"
	"github.com/perunio/perunio-facturador/internal/model"
)

// relatedNumberFormats is the cbc:ID format SUNAT checks per Cat.61 type
// (ERROR 3441, reglas-validacion-gre-2026-09-25.xlsx › Guía-Remitente2_0 items
// 13). A type with no entry has no format rule. The DAM / DS (50, 52) and the
// manifiesto (91) have motivo-specific régimen rules on top, in
// relatedRegimenFormat.
var relatedNumberFormats = map[string]*regexp.Regexp{
	sunat.Cat61Factura:                      regexp.MustCompile(`^(F[A-Z0-9]{3}|E001|[0-9]{1,4})-0*[1-9][0-9]{0,7}$`),
	sunat.Cat61Boleta:                       regexp.MustCompile(`^(B[A-Z0-9]{3}|EB01|[0-9]{1,4})-0*[1-9][0-9]{0,7}$`),
	sunat.Cat61LiquidacionCompra:            regexp.MustCompile(`^(L[A-Z0-9]{3}|E001|[0-9]{1,4})-0*[1-9][0-9]{0,7}$`),
	sunat.Cat61GuiaRemitente:                regexp.MustCompile(`^(T[A-Z0-9]{3}|EG07|EG02)-0*[1-9][0-9]{0,7}$`),
	sunat.Cat61Ticket:                       regexp.MustCompile(`^[a-zA-Z0-9-]{1,20}-[a-zA-Z0-9-]{1,20}$`),
	sunat.Cat61ComprobanteOperaciones:       regexp.MustCompile(`^[0-9]{1,4}-0*[1-9][0-9]{0,6}$`),
	sunat.Cat61ConstanciaDepositoIVAP:       regexp.MustCompile(`^0*[1-9][0-9]{0,14}$`),
	sunat.Cat61ConstanciaDetraccion:         regexp.MustCompile(`^0*[1-9][0-9]{0,14}$`),
	sunat.Cat61AutorizacionSCOP:             regexp.MustCompile(`^[A-Za-z0-9]{1,20}$`),
	sunat.Cat61DeclaracionAduanera:          regexp.MustCompile(`^[0-9]{3}-[0-9]{4}-[0-9]{2}-[1-9][0-9]{0,5}$`),
	sunat.Cat61DeclaracionSimplificada:      regexp.MustCompile(`^[0-9]{3}-[0-9]{4}-[0-9]{2}-[1-9][0-9]{0,5}$`),
	sunat.Cat61CitaTerminalPortuario:        regexp.MustCompile(`^[0-9]{1,50}$`),
	sunat.Cat61ResolucionAdjudicacion:       regexp.MustCompile(`^\S{1,100}$`),
	sunat.Cat61ResolucionComiso:             regexp.MustCompile(`^\S{1,100}$`),
	sunat.Cat61GuiaForestalSerfor:           regexp.MustCompile(`^\S{1,100}$`),
	sunat.Cat61GuiaTransitoSucamec:          regexp.MustCompile(`^\S{1,100}$`),
	sunat.Cat61AutorizacionSaneamientoMinsa: regexp.MustCompile(`^\S{1,100}$`),
	sunat.Cat61AutorizacionResiduosSolidos:  regexp.MustCompile(`^\S{1,100}$`),
	sunat.Cat61CertificadoFitosanitario:     regexp.MustCompile(`^\S{1,100}$`),
	sunat.Cat61RegistroAlcoholEtilico:       regexp.MustCompile(`^\S{1,100}$`),
}

// relatedRegimenFormat is the stricter 3441 format of a DAM / DS (régimen
// aduanero) or a manifiesto de carga under a given motivo de traslado, or nil.
func relatedRegimenFormat(docType, motivo string) *regexp.Regexp {
	switch {
	case docType == sunat.Cat61DeclaracionAduanera && motivo == sunat.Cat20Importacion:
		return regexp.MustCompile(`^[0-9]{3}-[0-9]{4}-10-[1-9][0-9]{0,5}$`)
	case docType == sunat.Cat61DeclaracionSimplificada && motivo == sunat.Cat20Importacion:
		return regexp.MustCompile(`^[0-9]{3}-[0-9]{4}-18-[1-9][0-9]{0,5}$`)
	case docType == sunat.Cat61DeclaracionAduanera && motivo == sunat.Cat20Exportacion:
		return regexp.MustCompile(`^[0-9]{3}-[0-9]{4}-40-[1-9][0-9]{0,5}$`)
	case docType == sunat.Cat61DeclaracionSimplificada && motivo == sunat.Cat20Exportacion:
		return regexp.MustCompile(`^[0-9]{3}-[0-9]{4}-48-[1-9][0-9]{0,5}$`)
	case docType == sunat.Cat61DeclaracionAduanera && motivo == sunat.Cat20ZonaPrimaria:
		return regexp.MustCompile(`^[0-9]{3}-[0-9]{4}-(10|20|21|30|36|70|80)-[1-9][0-9]{0,5}$`)
	case docType == sunat.Cat61DeclaracionSimplificada && motivo == sunat.Cat20ZonaPrimaria:
		return regexp.MustCompile(`^[0-9]{3}-[0-9]{4}-18-[1-9][0-9]{0,5}$`)
	case docType == sunat.Cat61ManifiestoCarga && motivo == sunat.Cat20ZonaPrimaria:
		return regexp.MustCompile(`^01-[0-9]{3}-[0-9]-[0-9]{4}-[1-9][0-9]{0,5}$`)
	}
	return nil
}

// validateRelatedDocuments checks a guía's documentos relacionados against the
// GRE 2.0 rules (reglas-validacion-gre-2026-09-25.xlsx): each one's Cat.61 type
// is valid for the guía (2692: gre-r on a 09, gre-t on a 31), has a number in
// its type's format (3403, 3441), is not repeated (3340), and carries its
// issuer's RUC where SUNAT requires it (3380/3382). Then the rules that tie the
// set to the motivo de traslado (remitente) or cap its size (transportista).
func validateRelatedDocuments(d *model.Despatch) []model.ValidationError {
	var errs []model.ValidationError
	remitente := d.DocType == sunat.GreDocTypeRemitente
	seen := map[string]bool{}
	for i, doc := range d.RelatedDocuments {
		field := fmt.Sprintf("relatedDocuments[%d]", i)
		allowed := sunat.Cat61GreR(doc.DocType)
		if !remitente {
			allowed = sunat.Cat61GreT(doc.DocType)
		}
		if !allowed {
			errs = append(errs, model.ValidationError{Code: 2692, Field: field + ".docType",
				Message: fmt.Sprintf("el tipo de documento relacionado %q no es válido para esta guía (catálogo 61)", doc.DocType)})
			continue
		}
		number := strings.TrimSpace(doc.Number)
		if number == "" {
			errs = append(errs, model.ValidationError{Code: 3403, Field: field + ".number",
				Message: "indique el número del documento relacionado"})
			continue
		}
		if key := doc.DocType + "|" + number; seen[key] {
			errs = append(errs, model.ValidationError{Code: 3340, Field: field + ".number",
				Message: fmt.Sprintf("el documento relacionado %s está repetido", number)})
		} else {
			seen[key] = true
		}
		format := relatedRegimenFormat(doc.DocType, d.TransferReason)
		if format == nil {
			format = relatedNumberFormats[doc.DocType]
		}
		if format != nil && !format.MatchString(number) {
			errs = append(errs, model.ValidationError{Code: 3441, Field: field + ".number",
				Message: fmt.Sprintf("el número %s no tiene el formato de un %s", number, cat61Label(doc.DocType))})
		}
		requiresIssuer := sunat.Cat61IssuerRemitente(doc.DocType)
		if !remitente {
			requiresIssuer = sunat.Cat61IssuerTransportista(doc.DocType)
		}
		switch issuer := strings.TrimSpace(doc.IssuerRUC); {
		case issuer == "" && requiresIssuer:
			errs = append(errs, model.ValidationError{Code: 3380, Field: field + ".issuerRuc",
				Message: fmt.Sprintf("indique el RUC del emisor del %s", cat61Label(doc.DocType))})
		case issuer != "" && !rucRegex.MatchString(issuer):
			errs = append(errs, model.ValidationError{Code: 3382, Field: field + ".issuerRuc",
				Message: "el emisor del documento relacionado se identifica con su RUC (11 dígitos)"})
		}
	}
	if remitente {
		errs = append(errs, validateRelatedForMotivo(d)...)
	} else {
		errs = append(errs, validateRelatedCountTransportista(d)...)
	}
	return errs
}

// validateRelatedForMotivo applies the remitente rules that tie the
// documentos relacionados to the motivo de traslado (Guía-Remitente2_0 item 12).
func validateRelatedForMotivo(d *model.Despatch) []model.ValidationError {
	var errs []model.ValidationError
	count := map[string]int{}
	for _, doc := range d.RelatedDocuments {
		count[doc.DocType]++
	}
	has := func(types ...string) bool {
		for _, t := range types {
			if count[t] > 0 {
				return true
			}
		}
		return false
	}
	others := func(allowed ...string) bool {
		ok := map[string]bool{}
		for _, t := range allowed {
			ok[t] = true
		}
		for t := range count {
			if !ok[t] {
				return true
			}
		}
		return false
	}
	add := func(code int, msg string) {
		errs = append(errs, model.ValidationError{Code: code, Field: "relatedDocuments", Message: msg})
	}
	dam, ds := sunat.Cat61DeclaracionAduanera, sunat.Cat61DeclaracionSimplificada
	manifiesto, cita := sunat.Cat61ManifiestoCarga, sunat.Cat61CitaTerminalPortuario
	switch d.TransferReason {
	case sunat.Cat20Importacion, sunat.Cat20Exportacion:
		if !has(dam, ds) {
			add(3440, "una importación o exportación debe relacionar su DAM o DS")
		}
		if others(sunat.Cat61GuiaRemitente, sunat.Cat61ConstanciaDepositoIVAP, dam, ds, sunat.Cat61ConstanciaDetraccion) {
			add(3445, "en una importación o exportación solo se relacionan guías remitente, DAM o DS y constancias de depósito")
		}
	case sunat.Cat20ZonaPrimaria:
		if !has(dam, ds, manifiesto, cita) {
			add(3493, "el traslado de mercancía extranjera debe relacionar su DAM, DS, manifiesto de carga o cita del terminal portuario")
		}
		if others(dam, ds, manifiesto, cita) {
			add(3445, "en el traslado de mercancía extranjera solo se relacionan DAM, DS, manifiesto de carga o cita del terminal portuario")
		}
		if count[manifiesto] > 1 {
			add(3494, "solo se permite un manifiesto de carga")
		}
		if count[cita] > 1 {
			add(3612, "solo se permite una cita u orden de entrega del terminal portuario")
		}
		if has(manifiesto) && has(dam, ds) {
			add(3463, "con un manifiesto de carga no se relaciona además una DAM o DS")
		}
		if has(cita) && has(dam, ds, manifiesto) {
			add(3613, "con una cita del terminal portuario no se relaciona además una DAM, DS o manifiesto")
		}
	case sunat.Cat20Otros:
		if has(ds) {
			add(3445, "con motivo 'otros' no se relaciona una DS")
		}
		if has(dam) && others(dam) {
			add(3635, "con una DAM y motivo 'otros', todos los documentos relacionados deben ser DAM")
		}
	default:
		if has(dam, ds) {
			add(3445, "una DAM o DS solo se relaciona en una importación, exportación, traslado de mercancía extranjera u otros")
		}
	}
	if d.TransferReason != sunat.Cat20ZonaPrimaria && has(manifiesto, cita) {
		add(3445, "un manifiesto de carga o una cita del terminal portuario solo se relaciona en el traslado de mercancía extranjera")
	}
	return errs
}

// validateRelatedCountTransportista caps how many documentos relacionados a
// GRE transportista lists (Guía-Transportista2_0 item 13): up to two when one
// is a GRE transportista or an authorization (31, 65-69, 3345); otherwise one,
// unless one is an electronic GRE remitente (09 whose serie starts with a
// letter, 3346).
func validateRelatedCountTransportista(d *model.Despatch) []model.ValidationError {
	n := len(d.RelatedDocuments)
	if n <= 1 {
		return nil
	}
	authorization, electronicGRE := false, false
	for _, doc := range d.RelatedDocuments {
		switch doc.DocType {
		case sunat.Cat61GuiaTransportista, sunat.Cat61AutorizacionMatpelCallao, sunat.Cat61AutorizacionCargaLima,
			sunat.Cat61PermisoMatpelMTC, sunat.Cat61HabilitacionSanitariaPesquera, sunat.Cat61PermisoTransporteMercancias:
			authorization = true
		case sunat.Cat61GuiaRemitente:
			if num := strings.TrimSpace(doc.Number); num != "" && (num[0] < '0' || num[0] > '9') {
				electronicGRE = true
			}
		}
	}
	switch {
	case authorization && n > 2:
		return []model.ValidationError{{Code: 3345, Field: "relatedDocuments",
			Message: "con una guía transportista o una autorización de transporte se relacionan hasta dos documentos"}}
	case !authorization && !electronicGRE:
		return []model.ValidationError{{Code: 3346, Field: "relatedDocuments",
			Message: "solo se relaciona más de un documento si uno es una guía remitente electrónica, una guía transportista o una autorización de transporte"}}
	}
	return nil
}

func cat61Label(code string) string {
	if e, ok := sunat.Cat61(code); ok {
		return e.Label
	}
	return code
}
