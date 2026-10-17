package xmlbuilder

import "testing"

func TestMontoEnLetras(t *testing.T) {
	cases := []struct {
		amount, currency, want string
	}{
		{"0.00", "PEN", "CERO CON 00/100 SOLES"},
		{"1.00", "PEN", "UNO CON 00/100 SOLES"},
		{"21.05", "PEN", "VEINTIUNO CON 05/100 SOLES"},
		{"100.00", "PEN", "CIEN CON 00/100 SOLES"},
		{"101.50", "PEN", "CIENTO UNO CON 50/100 SOLES"},
		{"141.60", "PEN", "CIENTO CUARENTA Y UNO CON 60/100 SOLES"},
		{"520.00", "PEN", "QUINIENTOS VEINTE CON 00/100 SOLES"},
		{"1000.00", "PEN", "MIL CON 00/100 SOLES"},
		{"1180.00", "PEN", "MIL CIENTO OCHENTA CON 00/100 SOLES"},
		{"1180.00", "USD", "MIL CIENTO OCHENTA CON 00/100 DOLARES AMERICANOS"},
		{"1298.00", "PEN", "MIL DOSCIENTOS NOVENTA Y OCHO CON 00/100 SOLES"},
		{"21000.00", "PEN", "VEINTIUN MIL CON 00/100 SOLES"},
		{"31999.99", "EUR", "TREINTA Y UN MIL NOVECIENTOS NOVENTA Y NUEVE CON 99/100 EUROS"},
		{"100000.00", "PEN", "CIEN MIL CON 00/100 SOLES"},
		{"1000000.00", "PEN", "UN MILLON CON 00/100 SOLES"},
		{"2500001.10", "PEN", "DOS MILLONES QUINIENTOS MIL UNO CON 10/100 SOLES"},
		{"21000000.00", "PEN", "VEINTIUN MILLONES CON 00/100 SOLES"},
	}
	for _, c := range cases {
		if got := montoEnLetras(c.amount, c.currency); got != c.want {
			t.Errorf("montoEnLetras(%q, %q) = %q, want %q", c.amount, c.currency, got, c.want)
		}
	}
}
