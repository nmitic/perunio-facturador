package http

import (
	"testing"

	"maragu.dev/is"
)

func TestValidMonto(t *testing.T) {
	tests := []struct {
		monto    string
		expected bool
	}{
		{monto: "1180.00", expected: true},
		{monto: "0.01", expected: true},
		{monto: "10", expected: true},
		{monto: "0"},
		{monto: "0.00"},
		{monto: "00.0"},
		{monto: ""},
		{monto: "-5"},
		{monto: "abc"},
	}
	for _, test := range tests {
		t.Run("should judge "+test.monto, func(t *testing.T) {
			is.Equal(t, test.expected, validMonto(test.monto))
		})
	}
}

func TestVentaAnticipoRequestValidate(t *testing.T) {
	monto := func(s string) *string { return &s }
	valid := func() ventaAnticipoRequest {
		return ventaAnticipoRequest{
			Nombre: "Obra casa Pérez", CustomerDocNumber: "20100070970", CustomerName: "ACME SAC",
			MontoAcordado: monto("1180.00"), FormData: map[string]any{"items": []any{}},
		}
	}

	t.Run("should accept a venta with a monto acordado", func(t *testing.T) {
		is.Equal(t, "", valid().validate())
	})

	t.Run("should accept a venta whose monto is por definir", func(t *testing.T) {
		req := valid()
		req.MontoAcordado = nil
		is.Equal(t, "", req.validate())
	})

	t.Run("should refuse a zero monto acordado", func(t *testing.T) {
		req := valid()
		req.MontoAcordado = monto("0.00")
		is.Equal(t, "El monto acordado debe ser mayor a 0", req.validate())
	})

	t.Run("should refuse a venta without a cliente", func(t *testing.T) {
		req := valid()
		req.CustomerName = ""
		is.Equal(t, "Seleccione el cliente de la venta", req.validate())
	})
}
