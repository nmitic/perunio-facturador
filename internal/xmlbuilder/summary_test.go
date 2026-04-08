package xmlbuilder_test

import (
	"strings"
	"testing"

	"maragu.dev/is"

	"github.com/perunio/perunio-facturador/internal/model"
	"github.com/perunio/perunio-facturador/internal/xmlbuilder"
)

func TestBuildSummaryXML(t *testing.T) {
	t.Run("should generate valid UBL 2.0 SummaryDocuments with correct version and customization", func(t *testing.T) {
		req := model.SummaryRequest{
			SupplierRUC:   "20100113612",
			SupplierName:  "EMPRESA TEST SAC",
			IssueDate:     "2024-01-16",
			ReferenceDate: "2024-01-15",
			Correlative:   1,
			Items: []model.SummaryItem{
				{
					LineNumber: 1, DocType: "03", Series: "B001",
					StartCorrelative: 1, EndCorrelative: 10,
					ConditionCode: "1", CurrencyCode: "PEN",
					TotalAmount: "1180.00", TotalIGV: "180.00",
					CustomerDocType: "1", CustomerDocNumber: "12345678",
				},
			},
		}

		xmlBytes, err := xmlbuilder.BuildSummaryXML(req)
		is.NotError(t, err)
		xml := string(xmlBytes)

		// UBL 2.0, NOT 2.1
		is.True(t, strings.Contains(xml, `<cbc:UBLVersionID>2.0</cbc:UBLVersionID>`), "should have UBL 2.0")
		is.True(t, strings.Contains(xml, `<cbc:CustomizationID>1.1</cbc:CustomizationID>`), "should have CustomizationID 1.1")

		// Root element
		is.True(t, strings.Contains(xml, `<SummaryDocuments`), "should have SummaryDocuments root")
		is.True(t, strings.Contains(xml, `xmlns:sac="`), "should have sac namespace")

		// Document ID format: RUC-RC-YYYYMMDD-NNNNN
		is.True(t, strings.Contains(xml, `<cbc:ID>20100113612-RC-20240116-00001</cbc:ID>`), "should have correct summary ID")

		// Reference date
		is.True(t, strings.Contains(xml, `<cbc:ReferenceDate>2024-01-15</cbc:ReferenceDate>`), "should have reference date")

		// Summary line
		is.True(t, strings.Contains(xml, `<sac:SummaryDocumentsLine>`), "should have summary line")
		is.True(t, strings.Contains(xml, `<cbc:ConditionCode>1</cbc:ConditionCode>`), "should have condition code")
	})
}

func TestBuildVoidedXML(t *testing.T) {
	t.Run("should generate valid UBL 2.0 VoidedDocuments with correct version", func(t *testing.T) {
		req := model.VoidRequest{
			SupplierRUC:  "20100113612",
			SupplierName: "EMPRESA TEST SAC",
			IssueDate:    "2024-01-16",
			Correlative:  1,
			Items: []model.VoidItem{
				{
					LineNumber: 1, DocType: "01", Series: "F001",
					Correlative: 1, VoidReason: "Error en documento",
				},
			},
		}

		xmlBytes, err := xmlbuilder.BuildVoidedXML(req)
		is.NotError(t, err)
		xml := string(xmlBytes)

		// UBL 2.0 with CustomizationID 1.0
		is.True(t, strings.Contains(xml, `<cbc:UBLVersionID>2.0</cbc:UBLVersionID>`), "should have UBL 2.0")
		is.True(t, strings.Contains(xml, `<cbc:CustomizationID>1.0</cbc:CustomizationID>`), "should have CustomizationID 1.0")

		// Root element
		is.True(t, strings.Contains(xml, `<VoidedDocuments`), "should have VoidedDocuments root")

		// Document ID format: RUC-RA-YYYYMMDD-NNNNN
		is.True(t, strings.Contains(xml, `<cbc:ID>20100113612-RA-20240116-00001</cbc:ID>`), "should have correct void ID")

		// Voided line
		is.True(t, strings.Contains(xml, `<sac:VoidedDocumentsLine>`), "should have voided line")
		is.True(t, strings.Contains(xml, `Error en documento`), "should have void reason")
	})
}

func TestSummaryFilename(t *testing.T) {
	t.Run("should format RC filename per SUNAT spec", func(t *testing.T) {
		name := xmlbuilder.SummaryFilename("20100113612", "2024-01-16", 1)
		is.Equal(t, "20100113612-RC-20240116-00001", name)
	})
}

func TestVoidFilename(t *testing.T) {
	t.Run("should format RA filename per SUNAT spec", func(t *testing.T) {
		name := xmlbuilder.VoidFilename("20100113612", "2024-01-16", 1)
		is.Equal(t, "20100113612-RA-20240116-00001", name)
	})
}
