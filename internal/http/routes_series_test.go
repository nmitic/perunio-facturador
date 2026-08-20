package http

import "testing"

// SUNAT's correlativo is 8 digits. A value past that is accepted by the DB
// (plain integer column) and only fails much later, when the XML carries a
// 9-digit número and SUNAT rejects the comprobante — so the bound is enforced
// here or nowhere.
func TestValidCorrelative(t *testing.T) {
	n := func(v int) *int { return &v }

	cases := []struct {
		name string
		in   *int
		want bool
	}{
		{"absent field is always fine", nil, true},
		{"first number of a brand-new serie", n(1), true},
		{"resuming from another facturador", n(4313), true},
		{"last 8-digit number", n(maxCorrelative), true},
		{"zero", n(0), false},
		{"negative", n(-1), false},
		{"past 8 digits", n(maxCorrelative + 1), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validCorrelative(tc.in); got != tc.want {
				t.Errorf("validCorrelative(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
