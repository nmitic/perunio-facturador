package validation_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"maragu.dev/is"

	"github.com/perunio/perunio-facturador/internal/model"
	"github.com/perunio/perunio-facturador/internal/validation"
)

// relatedGuia is the remitente guía of spec case despatch/14 (a venta with its
// factura), so only the documentos relacionados under test vary.
func relatedGuia(t *testing.T) (*model.Despatch, []model.DespatchLine) {
	t.Helper()
	raw, err := os.ReadFile("../xmlbuilder/spec/despatch/14-documento-relacionado/input.json")
	is.NotError(t, err)
	var in struct {
		Despatch model.Despatch       `json:"despatch"`
		Lines    []model.DespatchLine `json:"lines"`
	}
	is.NotError(t, json.Unmarshal(raw, &in))
	return &in.Despatch, in.Lines
}

// relatedCodes returns the codes of the errors on relatedDocuments fields.
func relatedCodes(d *model.Despatch, lines []model.DespatchLine) []int {
	var out []int
	for _, e := range validation.ValidateDespatch(d, lines) {
		if strings.HasPrefix(e.Field, "relatedDocuments") {
			out = append(out, e.Code)
		}
	}
	return out
}

func TestValidateRelatedDocuments(t *testing.T) {
	t.Run("a venta with its own factura passes", func(t *testing.T) {
		d, lines := relatedGuia(t)
		is.Equal(t, 0, len(relatedCodes(d, lines)))
	})

	t.Run("a code outside the guía's Cat.61 list is refused", func(t *testing.T) {
		d, lines := relatedGuia(t)
		d.RelatedDocuments[0].DocType = "31" // gre-t only
		is.EqualSlice(t, []int{2692}, relatedCodes(d, lines))
	})

	t.Run("a factura number in the wrong format is refused", func(t *testing.T) {
		d, lines := relatedGuia(t)
		d.RelatedDocuments[0].Number = "B001-123"
		is.EqualSlice(t, []int{3441}, relatedCodes(d, lines))
	})

	t.Run("a factura without its issuer is refused", func(t *testing.T) {
		d, lines := relatedGuia(t)
		d.RelatedDocuments[0].IssuerRUC = ""
		is.EqualSlice(t, []int{3380}, relatedCodes(d, lines))
	})

	t.Run("the same document twice is refused", func(t *testing.T) {
		d, lines := relatedGuia(t)
		d.RelatedDocuments = append(d.RelatedDocuments, d.RelatedDocuments[0])
		is.EqualSlice(t, []int{3340}, relatedCodes(d, lines))
	})

	t.Run("an importación without its DAM is refused", func(t *testing.T) {
		d, lines := relatedGuia(t)
		d.TransferReason = "08"
		codes := relatedCodes(d, lines)
		is.True(t, contains(codes, 3440), "3440: motivo 08 needs a DAM or DS")
		is.True(t, contains(codes, 3445), "3445: a factura does not belong on an importación")
	})

	t.Run("an importación with its DAM passes, and the régimen must be 10", func(t *testing.T) {
		d, lines := relatedGuia(t)
		d.TransferReason = "08"
		d.RelatedDocuments = []model.RelatedDocument{{DocType: "50", Number: "118-2024-10-123456"}}
		is.Equal(t, 0, len(relatedCodes(d, lines)))

		d.RelatedDocuments[0].Number = "118-2024-40-123456"
		is.EqualSlice(t, []int{3441}, relatedCodes(d, lines))
	})

	t.Run("a DAM on a venta is refused", func(t *testing.T) {
		d, lines := relatedGuia(t)
		d.RelatedDocuments = append(d.RelatedDocuments, model.RelatedDocument{DocType: "50", Number: "118-2024-10-123456"})
		is.EqualSlice(t, []int{3445}, relatedCodes(d, lines))
	})

	t.Run("a transportista lists one document unless one is a GRE remitente or an authorization", func(t *testing.T) {
		d, lines := relatedGuia(t)
		d.DocType = "31"
		d.Series = "V001"
		d.RelatedDocuments = []model.RelatedDocument{
			{DocType: "01", Number: "F001-123", IssuerRUC: "20100113612"},
			{DocType: "03", Number: "B001-9", IssuerRUC: "20100113612"},
		}
		is.True(t, contains(relatedCodes(d, lines), 3346), "two comprobantes alone on a 31")

		d.RelatedDocuments[1] = model.RelatedDocument{DocType: "09", Number: "T001-45", IssuerRUC: "20601327318"}
		is.True(t, !contains(relatedCodes(d, lines), 3346), "an electronic GRE remitente allows more")
	})
}

func contains(codes []int, code int) bool {
	for _, c := range codes {
		if c == code {
			return true
		}
	}
	return false
}
