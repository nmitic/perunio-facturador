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
