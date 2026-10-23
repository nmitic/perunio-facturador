package http

import (
	"testing"

	"maragu.dev/is"
)

// A line's price as the user typed it travels with its con/sin IGV flag: a nota
// or a repetir re-enters the pair, and one without the other cannot be.
func TestDocumentItemValidateEntry(t *testing.T) {
	price := func(s string) *string { return &s }
	flag := func(b bool) *bool { return &b }

	cases := []struct {
		name  string
		item  createDocumentItemBody
		valid bool
	}{
		{"a con-IGV price and its flag", createDocumentItemBody{EnteredUnitPrice: price("100"), PriceIncludesTax: flag(true)}, true},
		{"a sin-IGV price with decimals", createDocumentItemBody{EnteredUnitPrice: price("84.745762"), PriceIncludesTax: flag(false)}, true},
		{"neither, from a client that predates them", createDocumentItemBody{}, true},
		{"a price without its flag", createDocumentItemBody{EnteredUnitPrice: price("100")}, false},
		{"a flag without its price", createDocumentItemBody{PriceIncludesTax: flag(true)}, false},
		{"a price that is not a decimal", createDocumentItemBody{EnteredUnitPrice: price("1e2"), PriceIncludesTax: flag(true)}, false},
		{"a negative price", createDocumentItemBody{EnteredUnitPrice: price("-100"), PriceIncludesTax: flag(true)}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			is.Equal(t, c.valid, c.item.validateEntry() == "")
		})
	}
}
