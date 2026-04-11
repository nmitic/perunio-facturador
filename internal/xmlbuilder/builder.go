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

// DespatchXMLInput carries everything the GRE XML builders need in
// a single struct so the pipeline handler doesn't have to juggle
// many positional arguments.
type DespatchXMLInput struct {
	Despatch       *model.Despatch
	Lines          []model.DespatchLine
	RUC            string
	CompanyName    string
	CompanyAddress string

	// EventBaseDocType is only used when Despatch.DocType == "EV".
	// It must be "09" or "31" — the underlying flavor of the
	// superseding por-eventos GRE.
	EventBaseDocType string
}

// BuildDespatchXML generates UBL 2.1 DespatchAdvice XML for a Remitente
// (09), Transportista (31), or por-Eventos (EV) guía. The XML is
// ISO-8859-1 encoded with ext:ExtensionContent empty for signature
// injection.
func BuildDespatchXML(in DespatchXMLInput) ([]byte, error) {
	if in.Despatch == nil {
		return nil, fmt.Errorf("despatch is required")
	}
	switch in.Despatch.DocType {
	case model.DespatchTypeRemitente:
		return buildDespatchRemitenteXML(in.Despatch, in.Lines, in.RUC, in.CompanyName, in.CompanyAddress)
	case model.DespatchTypeTransportista:
		return buildDespatchTransportistaXML(in.Despatch, in.Lines, in.RUC, in.CompanyName, in.CompanyAddress)
	case model.DespatchTypeEvento:
		return buildDespatchEventoXML(in.Despatch, in.Lines, in.RUC, in.CompanyName, in.CompanyAddress, in.EventBaseDocType)
	default:
		return nil, fmt.Errorf("unsupported despatch type: %s", in.Despatch.DocType)
	}
}

// DespatchFilename returns the SUNAT GRE filename (without extension).
// Format: {RUC}-{TT}-{SERIE}-{CORRELATIVO}. For por-Eventos guías the
// base doc type (09 or 31) is used — the "EV" marker is an internal
// discriminator and is never part of the filename SUNAT sees.
func DespatchFilename(ruc, docType, series string, correlative int, eventBaseDocType string) string {
	if docType == model.DespatchTypeEvento {
		docType = eventBaseDocType
	}
	return fmt.Sprintf("%s-%s-%s-%08d", ruc, docType, series, correlative)
}
