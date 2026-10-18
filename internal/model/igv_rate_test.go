package model_test

import (
	"testing"

	"maragu.dev/is"

	"github.com/perunio/perunio-facturador/internal/model"
)

func TestIsIGVReducidoRate(t *testing.T) {
	cases := []struct {
		name      string
		igv, base float64
		want      bool
	}{
		{"18 %", 180, 1000, false},
		{"10.5 %", 105, 1000, true},
		{"10.5 % with rounding", 10.51, 100, true},
		{"18 % with rounding", 0.18, 1.01, false},
		{"no base", 0, 0, false},
	}
	for _, c := range cases {
		is.Equal(t, c.want, model.IsIGVReducidoRate(c.igv, c.base), c.name)
	}
}
