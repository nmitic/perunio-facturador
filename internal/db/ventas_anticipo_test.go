package db

import (
	"errors"
	"testing"

	"maragu.dev/is"

	"github.com/perunio/perunio-facturador/internal/model"
)

func TestDeriveVentaEstado(t *testing.T) {
	docID := "3f1b0c9e-0000-4000-8000-000000000001"

	tests := []struct {
		name            string
		cancelada       bool
		finalDocumentID *string
		expected        model.VentaAnticipoEstado
	}{
		{
			name:     "should be abierta while no factura final has been accepted",
			expected: model.VentaAbierta,
		},
		{
			name:            "should be regularizada once an accepted factura final exists",
			finalDocumentID: &docID,
			expected:        model.VentaRegularizada,
		},
		{
			name:      "should be cancelada when the user cancelled it",
			cancelada: true,
			expected:  model.VentaCancelada,
		},
		{
			name:            "should stay cancelada even with a factura final, because cancelling is an explicit human decision",
			cancelada:       true,
			finalDocumentID: &docID,
			expected:        model.VentaCancelada,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			is.Equal(t, test.expected, deriveVentaEstado(test.cancelada, test.finalDocumentID))
		})
	}
}

func TestCheckCancelacion(t *testing.T) {
	accepted := ventaAnticipoDoc{ID: "a1", Code: "F001-00000042", Status: "accepted"}
	observed := ventaAnticipoDoc{ID: "a2", Code: "F001-00000043", Status: "accepted_with_observations"}

	tests := []struct {
		name        string
		anticipos   []ventaAnticipoDoc
		cancelacion map[string]model.CancelacionDestino
		expected    error
		waiting     []string
	}{
		{
			name:     "should cancel a venta without anticipos with no destinos",
			expected: nil,
		},
		{
			name:        "should cancel when every anticipo in force has a destino",
			anticipos:   []ventaAnticipoDoc{accepted, observed},
			cancelacion: map[string]model.CancelacionDestino{"a1": model.CancelacionDevuelto, "a2": model.CancelacionOtraCompra},
		},
		{
			name:        "should refuse when an accepted anticipo has no destino",
			anticipos:   []ventaAnticipoDoc{accepted, observed},
			cancelacion: map[string]model.CancelacionDestino{"a1": model.CancelacionRetenido},
			expected:    ErrCancelacionIncompleta,
		},
		{
			name:        "should refuse an unknown destino",
			anticipos:   []ventaAnticipoDoc{accepted},
			cancelacion: map[string]model.CancelacionDestino{"a1": "regalado"},
			expected:    ErrCancelacionIncompleta,
		},
		{
			name:        "should refuse a destino for a comprobante that is not one of its anticipos in force",
			anticipos:   []ventaAnticipoDoc{accepted},
			cancelacion: map[string]model.CancelacionDestino{"a1": model.CancelacionDevuelto, "x9": model.CancelacionDevuelto},
			expected:    ErrCancelacionIncompleta,
		},
		{
			name: "should ask no destino for an anticipo a nota de crédito already reversed",
			anticipos: []ventaAnticipoDoc{
				{ID: "a1", Code: "F001-00000042", Status: "accepted", Reversed: true},
			},
		},
		{
			// A draft never reached SUNAT, and the pipeline refuses to emit it into
			// a cancelled venta.
			name: "should ask no destino for a draft, rejected or voided anticipo",
			anticipos: []ventaAnticipoDoc{
				{ID: "d1", Code: "F001-00000044", Status: "draft"},
				{ID: "r1", Code: "F001-00000045", Status: "rejected"},
				{ID: "v1", Code: "F001-00000046", Status: "voided"},
			},
		},
		{
			name: "should refuse while an anticipo may still be accepted by SUNAT",
			anticipos: []ventaAnticipoDoc{
				accepted,
				{ID: "p1", Code: "F001-00000047", Status: "pending"},
				{ID: "s1", Code: "F001-00000048", Status: "sending"},
			},
			cancelacion: map[string]model.CancelacionDestino{"a1": model.CancelacionDevuelto},
			waiting:     []string{"F001-00000047", "F001-00000048"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := checkCancelacion(test.anticipos, test.cancelacion)
			if test.waiting != nil {
				var enEspera *AnticiposEnEsperaError
				is.True(t, errors.As(err, &enEspera))
				is.EqualSlice(t, test.waiting, enEspera.Codes)
				return
			}
			is.Equal(t, test.expected, err)
		})
	}
}

func TestCountNotasPendientes(t *testing.T) {
	cancelacion := map[string]model.CancelacionDestino{
		"a1": model.CancelacionDevuelto,
		"a2": model.CancelacionDevuelto,
		"a3": model.CancelacionOtraCompra,
		"a4": model.CancelacionRetenido,
	}
	nota := func(anticipoID, status, reason string) model.VentaNotaCredito {
		return model.VentaNotaCredito{AnticipoID: anticipoID, Status: status, ReasonCode: reason}
	}

	tests := []struct {
		name     string
		notas    []model.VentaNotaCredito
		expected int
	}{
		{name: "should count every devuelto anticipo while no nota exists", expected: 2},
		{
			name:     "should settle a devuelto anticipo once its nota 06 is accepted",
			notas:    []model.VentaNotaCredito{nota("a1", "accepted", "06")},
			expected: 1,
		},
		{
			name:     "should settle it with a nota 01 accepted with observations",
			notas:    []model.VentaNotaCredito{nota("a1", "accepted_with_observations", "01")},
			expected: 1,
		},
		{
			name:     "should keep it pending while its nota waits on SUNAT",
			notas:    []model.VentaNotaCredito{nota("a1", "sent", "06")},
			expected: 2,
		},
		{
			name:     "should keep it pending under a partial nota (07 devolución por ítem)",
			notas:    []model.VentaNotaCredito{nota("a1", "accepted", "07")},
			expected: 2,
		},
		{
			name:     "should not count otra compra or retenido anticipos",
			notas:    []model.VentaNotaCredito{nota("a1", "accepted", "06"), nota("a2", "accepted", "06")},
			expected: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			is.Equal(t, test.expected, countNotasPendientes(cancelacion, test.notas))
		})
	}
}
