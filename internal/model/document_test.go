package model

import (
	"testing"

	"maragu.dev/is"
)

func TestSameAdquirente(t *testing.T) {
	tests := []struct {
		name     string
		a, b     string
		expected bool
	}{
		{name: "should match the same RUC", a: "20601327318", b: "20601327318", expected: true},
		{name: "should match the same DNI", a: "45678901", b: "45678901", expected: true},
		{name: "should match a DNI with the RUC 10 that embeds it", a: "45678901", b: "10456789012", expected: true},
		{name: "should match a RUC 10 with the DNI it embeds", a: "10456789012", b: "45678901", expected: true},
		{name: "should ignore surrounding spaces", a: " 20601327318", b: "20601327318 ", expected: true},
		{name: "should not match two different RUCs", a: "20601327318", b: "20100070970"},
		{name: "should not match a DNI with another person's RUC 10", a: "45678901", b: "10123456789"},
		{name: "should not match a DNI inside a RUC 20, which names a company", a: "60132731", b: "20601327318"},
		{name: "should not match an empty document", a: "", b: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			is.Equal(t, test.expected, SameAdquirente(test.a, test.b))
		})
	}
}
