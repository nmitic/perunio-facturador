package db

import (
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
