package validation

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	sunat "github.com/nmitic/perunio-sunat-catalogs/sunat"
	"github.com/perunio/perunio-facturador/internal/model"
)

var (
	despatchRemitenteSeriesRegex     = regexp.MustCompile(`^T[A-Z0-9]{3}$`)
	despatchTransportistaSeriesRegex = regexp.MustCompile(`^V[A-Z0-9]{3}$`)
)

// ValidateDespatch runs pre-submission validation on a GRE before the
// pipeline hits SUNAT. It returns an empty slice when the despatch is
// ready to sign+send.
func ValidateDespatch(d *model.Despatch, lines []model.DespatchLine) []model.ValidationError {
	var errs []model.ValidationError

	if d == nil {
		return []model.ValidationError{{Code: 1000, Message: "falta la guía", Field: "despatch"}}
	}

	errs = append(errs, validateDespatchHeader(d)...)
	errs = append(errs, validateDespatchRecipient(d)...)
	errs = append(errs, validateDespatchShipment(d)...)
	errs = append(errs, validateDespatchTransport(d)...)
	errs = append(errs, validateDespatchLines(lines)...)

	return errs
}

func validateDespatchHeader(d *model.Despatch) []model.ValidationError {
	var errs []model.ValidationError

	switch d.DocType {
	case sunat.GreDocTypeRemitente, sunat.GreDocTypeTransportista:
		// ok
	case sunat.GreDocTypeEvento:
		errs = append(errs, model.ValidationError{
			Code: 3374, Field: "docType",
			Message: "la GRE por evento se emite únicamente a través del SEE-SOL de SUNAT",
		})
	default:
		errs = append(errs, model.ValidationError{
			Code: 1003, Field: "docType",
			Message: fmt.Sprintf("tipo de guía no válido: %s (09 remitente o 31 transportista)", d.DocType),
		})
	}

	// Series format depends on the flavor — Remitente uses T### and
	// Transportista uses V###.
	switch d.DocType {
	case sunat.GreDocTypeRemitente:
		if !despatchRemitenteSeriesRegex.MatchString(d.Series) {
			errs = append(errs, model.ValidationError{Code: 1001, Field: "series", Message: "la serie de una guía remitente empieza con T y tiene 4 caracteres (T001)"})
		}
	case sunat.GreDocTypeTransportista:
		if !despatchTransportistaSeriesRegex.MatchString(d.Series) {
			errs = append(errs, model.ValidationError{Code: 1001, Field: "series", Message: "la serie de una guía transportista empieza con V y tiene 4 caracteres (V001)"})
		}
	}

	if d.Correlative <= 0 {
		errs = append(errs, model.ValidationError{Code: 1036, Field: "correlative", Message: "el número de la guía debe ser mayor que cero"})
	}

	if d.IssueTime == nil || *d.IssueTime == "" {
		errs = append(errs, model.ValidationError{Code: 3437, Field: "issueTime", Message: "la hora de emisión es obligatoria"})
	}

	return errs
}

func validateDespatchRecipient(d *model.Despatch) []model.ValidationError {
	var errs []model.ValidationError
	if d.RecipientDocNumber == "" {
		errs = append(errs, model.ValidationError{Code: 3000, Field: "recipientDocNumber", Message: "falta el documento del destinatario"})
	}
	if d.RecipientName == "" {
		errs = append(errs, model.ValidationError{Code: 3001, Field: "recipientName", Message: "falta el nombre o razón social del destinatario"})
	}
	return errs
}

func validateDespatchShipment(d *model.Despatch) []model.ValidationError {
	var errs []model.ValidationError

	// A GRE transportista states no motivo: the carrier moves someone else's
	// goods, and the Transportista sheet and XSL have no HandlingCode rule.
	// The remitente, which SUNAT requires there (3383), takes its place.
	if d.DocType == sunat.GreDocTypeTransportista {
		if d.CarrierRUC == nil || !rucRegex.MatchString(strings.TrimSpace(*d.CarrierRUC)) {
			errs = append(errs, model.ValidationError{Code: 3383, Field: "carrierRuc",
				Message: "una guía transportista indica el RUC del remitente (11 dígitos)"})
		}
		if d.TransferReason != "" && !sunat.Cat20Emit(d.TransferReason) {
			errs = append(errs, model.ValidationError{Code: 2800, Field: "transferReason",
				Message: fmt.Sprintf("motivo de traslado no válido: %q (catálogo 20)", d.TransferReason)})
		}
	} else if !sunat.Cat20Emit(d.TransferReason) {
		errs = append(errs, model.ValidationError{
			Code: 2800, Field: "transferReason",
			Message: fmt.Sprintf("motivo de traslado no válido: %q (catálogo 20)", d.TransferReason),
		})
	}
	// The fecha de entrega de bienes al transportista may not precede the
	// issue date (ERROR 3618).
	if d.DeliveryDate != nil && d.DeliveryDate.Format("2006-01-02") < d.IssueDate.Format("2006-01-02") {
		errs = append(errs, model.ValidationError{
			Code: 3618, Field: "deliveryDate",
			Message: "la fecha de entrega al transportista no puede ser anterior a la fecha de emisión",
		})
	}
	errs = append(errs, validateRelatedDocuments(d)...)
	// A declared driver carries nombres and apellidos (ERROR 3360 / 3361).
	// driverName alone, from drafts older than driverFamilyName, must hold
	// both so the builder can split it.
	if d.DriverDocNumber != nil && *d.DriverDocNumber != "" {
		first := strings.Fields(derefString(d.DriverName))
		family := strings.TrimSpace(derefString(d.DriverFamilyName))
		switch {
		case len(first) == 0:
			errs = append(errs, model.ValidationError{Code: 3360, Field: "driverName", Message: "los nombres del conductor son obligatorios"})
		case family == "" && len(first) < 2:
			errs = append(errs, model.ValidationError{Code: 3361, Field: "driverFamilyName", Message: "los apellidos del conductor son obligatorios"})
		}
	}
	// Motivo 13 (Otros) must describe the traslado (ERROR 3457).
	if d.TransferReason == sunat.Cat20Otros &&
		(d.TransferReasonDesc == nil || strings.TrimSpace(*d.TransferReasonDesc) == "") {
		errs = append(errs, model.ValidationError{
			Code: 3457, Field: "transferReasonDesc",
			Message: "el motivo de traslado 13 (Otros) requiere una descripción",
		})
	}

	// Membership, not just shape: a syntactically valid ubigeo that INEI does not
	// define reached SUNAT before and came back as a rejection.
	if !sunat.UbigeoDistritoValid(d.StartUbigeo) {
		errs = append(errs, model.ValidationError{Code: 2801, Field: "startUbigeo", Message: "el distrito del punto de partida no es válido"})
	}
	if !sunat.UbigeoDistritoValid(d.ArrivalUbigeo) {
		errs = append(errs, model.ValidationError{Code: 2802, Field: "arrivalUbigeo", Message: "el distrito del punto de llegada no es válido"})
	}
	if d.StartAddress == "" {
		errs = append(errs, model.ValidationError{Code: 2803, Field: "startAddress", Message: "falta la dirección del punto de partida"})
	}
	if d.ArrivalAddress == "" {
		errs = append(errs, model.ValidationError{Code: 2804, Field: "arrivalAddress", Message: "falta la dirección del punto de llegada"})
	}

	if d.TotalWeightKg == "" {
		errs = append(errs, model.ValidationError{Code: 2805, Field: "totalWeightKg", Message: "falta el peso bruto total"})
	} else {
		w, err := strconv.ParseFloat(d.TotalWeightKg, 64)
		if err != nil || w <= 0 {
			errs = append(errs, model.ValidationError{Code: 2805, Field: "totalWeightKg", Message: "el peso bruto total debe ser mayor que cero"})
		}
	}

	// The unidad a guía's peso bruto may use is the Cat.03 `gre-weight` subset,
	// which is KGM alone. Widening it is a data change in the catálogo rather
	// than an edit here — and the issuance form reads the same subset, so it can
	// no longer offer a unidad this rejects.
	unit := d.WeightUnitCode
	if unit == "" {
		unit = sunat.Cat03KGM
	}
	if !sunat.Cat03GreWeight(unit) {
		errs = append(errs, model.ValidationError{Code: 2806, Field: "weightUnitCode", Message: fmt.Sprintf("la unidad del peso debe ser una de %v", sunat.Cat03GreWeightCodes)})
	}

	return errs
}

func validateDespatchTransport(d *model.Despatch) []model.ValidationError {
	var errs []model.ValidationError

	// An M1 or L vehicle needs no plate, conductor or transportista
	// (ValidaExprRegGreRemitente: 2558, 2566, 2568 skip it). The indicador
	// exists on the remitente only.
	m1l := d.VehicleM1L
	if m1l && d.DocType == sunat.GreDocTypeTransportista {
		errs = append(errs, model.ValidationError{Code: 2811, Field: "vehicleM1l",
			Message: "una guía transportista no puede declarar un vehículo M1 o L (auto o moto)"})
		m1l = false
	}

	switch d.TransportModality {
	case model.TransportModalityPublic:
		// Public: third-party carrier RUC is mandatory.
		if m1l {
			break
		}
		if d.CarrierRUC == nil || *d.CarrierRUC == "" {
			errs = append(errs, model.ValidationError{Code: 2810, Field: "carrierRuc", Message: "falta el RUC de la empresa de transporte"})
		} else if !rucRegex.MatchString(*d.CarrierRUC) {
			errs = append(errs, model.ValidationError{Code: 2810, Field: "carrierRuc", Message: "el RUC de la empresa de transporte tiene 11 dígitos"})
		}
	case model.TransportModalityPrivate:
		// Private: driver license + at least one vehicle plate are
		// hard-required by SUNAT.
		if m1l {
			break
		}
		if d.DriverLicense == nil || *d.DriverLicense == "" {
			errs = append(errs, model.ValidationError{Code: 2820, Field: "driverLicense", Message: "falta la licencia de conducir del conductor"})
		}
		if d.VehiclePlate == nil || *d.VehiclePlate == "" {
			errs = append(errs, model.ValidationError{Code: 2821, Field: "vehiclePlate", Message: "falta la placa del vehículo"})
		}
	default:
		errs = append(errs, model.ValidationError{
			Code: 2811, Field: "transportModality",
			Message: fmt.Sprintf("modalidad de transporte no válida: %q (01 o 02)", d.TransportModality),
		})
	}

	// Transportista (31) always carries goods on the carrier's own
	// vehicle, so driver + plate are mandatory regardless of the
	// modality field.
	if d.DocType == sunat.GreDocTypeTransportista {
		if d.DriverLicense == nil || *d.DriverLicense == "" {
			errs = append(errs, model.ValidationError{Code: 2820, Field: "driverLicense", Message: "una guía transportista declara la licencia del conductor"})
		}
		if d.VehiclePlate == nil || *d.VehiclePlate == "" {
			errs = append(errs, model.ValidationError{Code: 2821, Field: "vehiclePlate", Message: "una guía transportista declara la placa del vehículo"})
		}
	}

	return errs
}

func validateDespatchLines(lines []model.DespatchLine) []model.ValidationError {
	var errs []model.ValidationError
	if len(lines) == 0 {
		errs = append(errs, model.ValidationError{Code: 2830, Field: "lines", Message: "la guía necesita al menos un bien"})
		return errs
	}
	for i, l := range lines {
		if l.Description == "" {
			errs = append(errs, model.ValidationError{Code: 2831, Field: fmt.Sprintf("lines[%d].description", i), Message: "falta la descripción del bien"})
		}
		if l.UnitCode == "" {
			errs = append(errs, model.ValidationError{Code: 2832, Field: fmt.Sprintf("lines[%d].unitCode", i), Message: "falta la unidad de medida del bien"})
		} else if !sunat.Cat03Valid(l.UnitCode) {
			errs = append(errs, model.ValidationError{Code: 2832, Field: fmt.Sprintf("lines[%d].unitCode", i), Message: fmt.Sprintf("%q is not a catálogo 03 unidad de medida", l.UnitCode)})
		}
		if l.Quantity == "" {
			errs = append(errs, model.ValidationError{Code: 2833, Field: fmt.Sprintf("lines[%d].quantity", i), Message: "falta la cantidad del bien"})
		} else {
			q, err := strconv.ParseFloat(l.Quantity, 64)
			if err != nil || q <= 0 {
				errs = append(errs, model.ValidationError{Code: 2833, Field: fmt.Sprintf("lines[%d].quantity", i), Message: "la cantidad del bien debe ser mayor que cero"})
			}
		}
	}
	return errs
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
