package xmlbuilder

import (
	"fmt"

	"github.com/perunio/perunio-facturador/internal/model"
)

// BuildDocumentXML generates UBL XML bytes for the given document request.
// The XML is encoded in ISO-8859-1 with the ext:ExtensionContent left empty
// for signature injection.
func BuildDocumentXML(req model.IssueRequest) ([]byte, error) {
	switch req.DocType {
	case model.DocTypeFactura, model.DocTypeBoleta:
		return buildInvoiceXML(req)
	case model.DocTypeNotaCredito:
		return buildCreditNoteXML(req)
	case model.DocTypeNotaDebito:
		return buildDebitNoteXML(req)
	default:
		return nil, fmt.Errorf("unsupported document type: %s", req.DocType)
	}
}

// Filename returns the SUNAT-compliant filename (without extension).
// Format: {RUC}-{TT}-{SERIE}-{CORRELATIVO}
func Filename(ruc, docType, series string, correlative int) string {
	return fmt.Sprintf("%s-%s-%s-%08d", ruc, docType, series, correlative)
}
