package xmlbuilder

import (
	"testing"

	"github.com/perunio/perunio-facturador/internal/model"
)

func TestFillSummaryTotals(t *testing.T) {
	lines := []model.LineItem{
		// gravado 1000, after a global discount the document subtotal is 1450
		{TaxExemptionReasonCode: "10", LineTotal: "1000.00", IGVAmount: "180.00"},
		{TaxExemptionReasonCode: "20", LineTotal: "200.00"},
		{TaxExemptionReasonCode: "30", LineTotal: "300.00"},
		// gratuitos: referential base = unitPriceWithTax × qty − IGV
		{TaxExemptionReasonCode: "11", Quantity: "1", UnitPriceWithTax: "59.00", IGVAmount: "9.00"},
		{TaxExemptionReasonCode: "21", Quantity: "2", UnitPriceWithTax: "10.00", IGVAmount: "0.00"},
		{TaxExemptionReasonCode: "31", Quantity: "1", UnitPriceWithTax: "5.00", IGVAmount: "0.00"},
	}
	var got model.SummaryItem
	FillSummaryTotals(&got, "1450.00", lines)

	want := model.SummaryItem{
		TotalGravada:        "950.00",
		TotalExonerated:     "200.00",
		TotalUnaffected:     "300.00",
		TotalExport:         "0.00",
		TotalFree:           "75.00",
		TotalFreeGravada:    "50.00",
		TotalFreeExonerated: "20.00",
		TotalFreeUnaffected: "5.00",
		TotalFreeIGV:        "9.00",
	}
	if got != want {
		t.Errorf("FillSummaryTotals =\n%+v\nwant\n%+v", got, want)
	}
}
