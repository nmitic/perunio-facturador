package xmlbuilder

import (
	"errors"
	"fmt"

	sunat "github.com/nmitic/perunio-sunat-catalogs/sunat"
	"github.com/perunio/perunio-facturador/internal/model"
)

// BuildDocumentXML generates UBL XML bytes for the given document request.
// The XML is encoded in ISO-8859-1 with the ext:ExtensionContent left empty
// for signature injection.
func BuildDocumentXML(req model.IssueRequest) ([]byte, error) {
	switch req.DocType {
	case sunat.Cat01Factura, sunat.Cat01Boleta:
		return buildInvoiceXML(req)
	case sunat.Cat01NotaCredito:
		return buildCreditNoteXML(req)
	case sunat.Cat01NotaDebito:
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
}

// ErrDespatchEvento refuses a GRE por evento. R.S. 123-2022/SUNAT (numeral 5.2,
// adding to art. 32 of R.S. 097-2012): "La GRE - por evento se emite
// únicamente a través del SEE - SOL", and both GRE validators reject the
// event-type tag it would carry (ERROR 3374). A Sistema del Contribuyente can
// only produce a por evento guía SUNAT rejects.
var ErrDespatchEvento = errors.New("la GRE por evento se emite únicamente a través del SEE-SOL (R.S. 123-2022/SUNAT, numeral 5.2); no puede enviarse desde Perunio")

// BuildDespatchXML generates UBL 2.1 DespatchAdvice XML for a Remitente
// (09), Transportista (31), or por-Eventos (EV) guía. The XML is
// ISO-8859-1 encoded with ext:ExtensionContent empty for signature
// injection.
func BuildDespatchXML(in DespatchXMLInput) ([]byte, error) {
	if in.Despatch == nil {
		return nil, fmt.Errorf("despatch is required")
	}
	switch in.Despatch.DocType {
	case sunat.GreDocTypeRemitente:
		return buildDespatchRemitenteXML(in.Despatch, in.Lines, in.RUC, in.CompanyName, in.CompanyAddress)
	case sunat.GreDocTypeTransportista:
		return buildDespatchTransportistaXML(in.Despatch, in.Lines, in.RUC, in.CompanyName, in.CompanyAddress)
	case sunat.GreDocTypeEvento:
		return nil, ErrDespatchEvento
	default:
		return nil, fmt.Errorf("unsupported despatch type: %s", in.Despatch.DocType)
	}
}

// DespatchFilename returns the SUNAT GRE filename (without extension).
// Format: {RUC}-{TT}-{SERIE}-{CORRELATIVO}, TT being 09 or 31 (a por evento
// guía is never built: see ErrDespatchEvento).
func DespatchFilename(ruc, docType, series string, correlative int) string {
	return fmt.Sprintf("%s-%s-%s-%08d", ruc, docType, series, correlative)
}
