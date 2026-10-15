package http

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/perunio/perunio-facturador/internal/model"
)

// The XML spec suite (internal/xmlbuilder/spec) starts from a model.IssueRequest.
// This test pins the one step before it: a draft row exactly as the DB returns
// it — every optional column NULL — must become the input of the base spec case,
// so the defaults the pipeline fills in (0101, contado, afectación 10, precio
// tipo 01, establecimiento 0000, …) are the ones the specs assume.
func TestBuildIssueRequestFromDoc_MatchesBaseSpecInput(t *testing.T) {
	raw, err := os.ReadFile("../xmlbuilder/spec/invoice/01-factura-gravada/input.json")
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var want model.IssueRequest
	if err := dec.Decode(&want); err != nil {
		t.Fatal(err)
	}

	notes := "MIL CIENTO OCHENTA CON 00/100 SOLES"
	issueTime := "15:20:30"
	doc := &model.IssuedDocument{
		DocType:           "01",
		Series:            "F001",
		Correlative:       1,
		IssueDate:         time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		IssueTime:         &issueTime,
		CurrencyCode:      "PEN",
		CustomerDocType:   "6",
		CustomerDocNumber: "20601327318",
		CustomerName:      "CLIENTE TEST SRL",
		CustomerAddress:   ptr("AV. CLIENTE 456"),
		Subtotal:          "1000.00",
		TotalIgv:          "180.00",
		TotalAmount:       "1180.00",
		Notes:             &notes,
	}
	items := []model.IssuedDocumentItem{{
		LineNumber:  1,
		Description: "PRODUCTO TEST",
		Quantity:    "10",
		UnitCode:    "NIU",
		UnitPrice:   "100.00",
		// Stored explicitly: the fallback (UnitPrice) is the IGV-exclusive price,
		// which would be wrong for PricingReference on a gravado line.
		UnitPriceWithTax: ptr("118.00"),
		IgvAmount:        "180.00",
		LineTotal:        "1000.00",
	}}

	got := buildIssueRequestFromDoc(doc, items, "20100113612", "EMPRESA TEST SAC", "AV. TEST 123", "")

	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		t.Errorf("draft row does not map to spec/invoice/01 input.json\ngot:\n%s", gotJSON)
	}
}

func ptr(s string) *string { return &s }
