package validation_test

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	sunat "github.com/nmitic/perunio-sunat-catalogs/sunat"
	"maragu.dev/is"

	"github.com/perunio/perunio-facturador/internal/model"
	"github.com/perunio/perunio-facturador/internal/validation"
)

// specGuia reads a despatch spec case's input.
func specGuia(t *testing.T, dir string) (*model.Despatch, []model.DespatchLine) {
	t.Helper()
	raw, err := os.ReadFile("../xmlbuilder/spec/despatch/" + dir + "/input.json")
	is.NotError(t, err)
	var in struct {
		Despatch model.Despatch       `json:"despatch"`
		Lines    []model.DespatchLine `json:"lines"`
	}
	is.NotError(t, json.Unmarshal(raw, &in))
	return &in.Despatch, in.Lines
}

func errorFields(d *model.Despatch, lines []model.DespatchLine) []string {
	var out []string
	for _, e := range validation.ValidateDespatch(d, lines) {
		out = append(out, fmt.Sprintf("%d %s", e.Code, e.Field))
	}
	return out
}

// An M1 or L vehicle (car, motorcycle) needs no plate, conductor or
// transportista: SUNAT skips 2558, 2566 and 2568 under its indicador.
func TestDespatchVehicleM1L(t *testing.T) {
	t.Run("vehículo propio without plate or conductor passes", func(t *testing.T) {
		d, lines := specGuia(t, "19-vehiculo-m1l")
		d.VehiclePlate, d.DriverLicense, d.DriverDocNumber = nil, nil, nil
		is.Equal(t, 0, len(errorFields(d, lines)))
	})

	t.Run("transporte público without transportista passes", func(t *testing.T) {
		d, lines := specGuia(t, "20-vehiculo-m1l-transporte-publico")
		d.CarrierRUC, d.CarrierName = nil, nil
		is.Equal(t, 0, len(errorFields(d, lines)))
	})

	t.Run("without the indicador the plate and licence are required again", func(t *testing.T) {
		d, lines := specGuia(t, "19-vehiculo-m1l")
		d.VehicleM1L = false
		d.VehiclePlate, d.DriverLicense = nil, nil
		is.EqualSlice(t, []string{"2820 driverLicense", "2821 vehiclePlate"}, errorFields(d, lines))
	})

	t.Run("a guía transportista cannot use it", func(t *testing.T) {
		d, lines := specGuia(t, "03-transportista")
		d.VehicleM1L = true
		is.Equal(t, true, d.DocType == sunat.GreDocTypeTransportista)
		is.EqualSlice(t, []string{"2811 vehicleM1l"}, errorFields(d, lines))
	})
}

// A GRE transportista names its remitente (3383), needs no motivo, and cites
// the remitente's guía as issued by that remitente (3381).
func TestDespatchTransportista(t *testing.T) {
	t.Run("without a motivo it passes", func(t *testing.T) {
		d, lines := specGuia(t, "21-transportista-sin-motivo")
		is.Equal(t, 0, len(errorFields(d, lines)))
	})

	t.Run("without a remitente it is refused", func(t *testing.T) {
		d, lines := specGuia(t, "03-transportista")
		d.CarrierRUC = nil
		is.EqualSlice(t, []string{"3383 carrierRuc"}, errorFields(d, lines))
	})

	t.Run("the remitente's guía issued by someone else is refused", func(t *testing.T) {
		d, lines := specGuia(t, "18-transportista-documento-relacionado")
		d.RelatedDocuments[0].IssuerRUC = "20601327318"
		is.EqualSlice(t, []string{"3381 relatedDocuments[0].issuerRuc"}, errorFields(d, lines))
	})

	t.Run("a remitente guía still needs its motivo", func(t *testing.T) {
		d, lines := specGuia(t, "01-remitente-transporte-privado")
		d.TransferReason = ""
		is.EqualSlice(t, []string{"2800 transferReason"}, errorFields(d, lines))
	})
}
