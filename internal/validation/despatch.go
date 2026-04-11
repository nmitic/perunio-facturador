package validation

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/perunio/perunio-facturador/internal/model"
)

var (
	ubigeoRegex               = regexp.MustCompile(`^\d{6}$`)
	despatchRemitenteSeriesRegex     = regexp.MustCompile(`^T[A-Z0-9]{3}$`)
	despatchTransportistaSeriesRegex = regexp.MustCompile(`^V[A-Z0-9]{3}$`)
)

// validTransferReasons is Cat.20 (motivo de traslado). Codes 01–19
// are defined; SUNAT may add new codes — we validate the known set
// and reject the rest.
var validTransferReasons = map[string]struct{}{
	"01": {}, "02": {}, "04": {}, "08": {}, "09": {},
	"13": {}, "14": {}, "18": {}, "19": {},
}

// ValidateDespatch runs pre-submission validation on a GRE before the
// pipeline hits SUNAT. It returns an empty slice when the despatch is
// ready to sign+send.
func ValidateDespatch(d *model.Despatch, lines []model.DespatchLine) []model.ValidationError {
	var errs []model.ValidationError

	if d == nil {
		return []model.ValidationError{{Code: 1000, Message: "despatch is nil", Field: "despatch"}}
	}

	errs = append(errs, validateDespatchHeader(d)...)
	errs = append(errs, validateDespatchRecipient(d)...)
	errs = append(errs, validateDespatchShipment(d)...)
	errs = append(errs, validateDespatchTransport(d)...)
	errs = append(errs, validateDespatchLines(lines)...)

	if d.DocType == model.DespatchTypeEvento {
		errs = append(errs, validateDespatchEvento(d)...)
	}

	return errs
}

func validateDespatchHeader(d *model.Despatch) []model.ValidationError {
	var errs []model.ValidationError

	switch d.DocType {
	case model.DespatchTypeRemitente, model.DespatchTypeTransportista, model.DespatchTypeEvento:
		// ok
	default:
		errs = append(errs, model.ValidationError{
			Code: 1003, Field: "docType",
			Message: fmt.Sprintf("invalid despatch type: %s", d.DocType),
		})
	}

	// Series format depends on the flavor — Remitente uses T### and
	// Transportista uses V###. por-Eventos uses whichever underlying
	// flavor it supersedes, so we accept either.
	switch d.DocType {
	case model.DespatchTypeRemitente:
		if !despatchRemitenteSeriesRegex.MatchString(d.Series) {
			errs = append(errs, model.ValidationError{Code: 1001, Field: "series", Message: "remitente series must match T[A-Z0-9]{3}"})
		}
	case model.DespatchTypeTransportista:
		if !despatchTransportistaSeriesRegex.MatchString(d.Series) {
			errs = append(errs, model.ValidationError{Code: 1001, Field: "series", Message: "transportista series must match V[A-Z0-9]{3}"})
		}
	case model.DespatchTypeEvento:
		if !despatchRemitenteSeriesRegex.MatchString(d.Series) &&
			!despatchTransportistaSeriesRegex.MatchString(d.Series) {
			errs = append(errs, model.ValidationError{Code: 1001, Field: "series", Message: "por-eventos series must match T[A-Z0-9]{3} or V[A-Z0-9]{3}"})
		}
	}

	if d.Correlative <= 0 {
		errs = append(errs, model.ValidationError{Code: 1036, Field: "correlative", Message: "correlative must be > 0"})
	}

	return errs
}

func validateDespatchRecipient(d *model.Despatch) []model.ValidationError {
	var errs []model.ValidationError
	if d.RecipientDocNumber == "" {
		errs = append(errs, model.ValidationError{Code: 3000, Field: "recipientDocNumber", Message: "recipient document number is required"})
	}
	if d.RecipientName == "" {
		errs = append(errs, model.ValidationError{Code: 3001, Field: "recipientName", Message: "recipient name is required"})
	}
	return errs
}

func validateDespatchShipment(d *model.Despatch) []model.ValidationError {
	var errs []model.ValidationError

	if _, ok := validTransferReasons[d.TransferReason]; !ok {
		errs = append(errs, model.ValidationError{
			Code: 2800, Field: "transferReason",
			Message: fmt.Sprintf("invalid transfer reason %q (Cat.20)", d.TransferReason),
		})
	}

	if !ubigeoRegex.MatchString(d.StartUbigeo) {
		errs = append(errs, model.ValidationError{Code: 2801, Field: "startUbigeo", Message: "start ubigeo must be 6 digits"})
	}
	if !ubigeoRegex.MatchString(d.ArrivalUbigeo) {
		errs = append(errs, model.ValidationError{Code: 2802, Field: "arrivalUbigeo", Message: "arrival ubigeo must be 6 digits"})
	}
	if d.StartAddress == "" {
		errs = append(errs, model.ValidationError{Code: 2803, Field: "startAddress", Message: "start address is required"})
	}
	if d.ArrivalAddress == "" {
		errs = append(errs, model.ValidationError{Code: 2804, Field: "arrivalAddress", Message: "arrival address is required"})
	}

	if d.TotalWeightKg == "" {
		errs = append(errs, model.ValidationError{Code: 2805, Field: "totalWeightKg", Message: "weight is required"})
	} else {
		w, err := strconv.ParseFloat(d.TotalWeightKg, 64)
		if err != nil || w <= 0 {
			errs = append(errs, model.ValidationError{Code: 2805, Field: "totalWeightKg", Message: "weight must be > 0"})
		}
	}

	unit := d.WeightUnitCode
	if unit == "" {
		unit = "KGM"
	}
	if unit != "KGM" {
		errs = append(errs, model.ValidationError{Code: 2806, Field: "weightUnitCode", Message: "weight unit must be KGM"})
	}

	return errs
}

func validateDespatchTransport(d *model.Despatch) []model.ValidationError {
	var errs []model.ValidationError

	switch d.TransportModality {
	case model.TransportModalityPublic:
		// Public: third-party carrier RUC is mandatory.
		if d.CarrierRUC == nil || *d.CarrierRUC == "" {
			errs = append(errs, model.ValidationError{Code: 2810, Field: "carrierRuc", Message: "carrier RUC is required for public transport"})
		} else if !rucRegex.MatchString(*d.CarrierRUC) {
			errs = append(errs, model.ValidationError{Code: 2810, Field: "carrierRuc", Message: "carrier RUC must be 11 digits"})
		}
	case model.TransportModalityPrivate:
		// Private: driver license + at least one vehicle plate are
		// hard-required by SUNAT.
		if d.DriverLicense == nil || *d.DriverLicense == "" {
			errs = append(errs, model.ValidationError{Code: 2820, Field: "driverLicense", Message: "driver license is required for private transport"})
		}
		if d.VehiclePlate == nil || *d.VehiclePlate == "" {
			errs = append(errs, model.ValidationError{Code: 2821, Field: "vehiclePlate", Message: "at least one vehicle plate is required for private transport"})
		}
	default:
		errs = append(errs, model.ValidationError{
			Code: 2811, Field: "transportModality",
			Message: fmt.Sprintf("invalid transport modality %q (must be 01 or 02)", d.TransportModality),
		})
	}

	// Transportista (31) always carries goods on the carrier's own
	// vehicle, so driver + plate are mandatory regardless of the
	// modality field.
	if d.DocType == model.DespatchTypeTransportista {
		if d.DriverLicense == nil || *d.DriverLicense == "" {
			errs = append(errs, model.ValidationError{Code: 2820, Field: "driverLicense", Message: "driver license is required for transportista guías"})
		}
		if d.VehiclePlate == nil || *d.VehiclePlate == "" {
			errs = append(errs, model.ValidationError{Code: 2821, Field: "vehiclePlate", Message: "at least one vehicle plate is required for transportista guías"})
		}
	}

	return errs
}

func validateDespatchLines(lines []model.DespatchLine) []model.ValidationError {
	var errs []model.ValidationError
	if len(lines) == 0 {
		errs = append(errs, model.ValidationError{Code: 2830, Field: "lines", Message: "at least one despatch line is required"})
		return errs
	}
	for i, l := range lines {
		if l.Description == "" {
			errs = append(errs, model.ValidationError{Code: 2831, Field: fmt.Sprintf("lines[%d].description", i), Message: "line description is required"})
		}
		if l.UnitCode == "" {
			errs = append(errs, model.ValidationError{Code: 2832, Field: fmt.Sprintf("lines[%d].unitCode", i), Message: "line unit code is required"})
		}
		if l.Quantity == "" {
			errs = append(errs, model.ValidationError{Code: 2833, Field: fmt.Sprintf("lines[%d].quantity", i), Message: "line quantity is required"})
		} else {
			q, err := strconv.ParseFloat(l.Quantity, 64)
			if err != nil || q <= 0 {
				errs = append(errs, model.ValidationError{Code: 2833, Field: fmt.Sprintf("lines[%d].quantity", i), Message: "line quantity must be > 0"})
			}
		}
	}
	return errs
}

func validateDespatchEvento(d *model.Despatch) []model.ValidationError {
	var errs []model.ValidationError
	if d.EventCode == nil || *d.EventCode == "" {
		errs = append(errs, model.ValidationError{Code: 2840, Field: "eventCode", Message: "event code (Cat.59) is required for por-eventos guías"})
	}
	if d.OriginalGreID == nil || *d.OriginalGreID == "" {
		errs = append(errs, model.ValidationError{Code: 2841, Field: "originalGreId", Message: "original GRE reference is required for por-eventos guías"})
	}
	return errs
}
