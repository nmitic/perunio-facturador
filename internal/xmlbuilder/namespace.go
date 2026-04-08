package xmlbuilder

// UBL 2.1 namespaces for Invoice, CreditNote, DebitNote.
const (
	NSInvoice    = "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
	NSCreditNote = "urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"
	NSDebitNote  = "urn:oasis:names:specification:ubl:schema:xsd:DebitNote-2"
	NSCAC        = "urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
	NSCBC        = "urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"
	NSEXT        = "urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2"
	NSDS         = "http://www.w3.org/2000/09/xmldsig#"

	// UBL 2.0 namespaces for RC/RA.
	NSSAC              = "urn:sunat:names:specification:ubl:peru:schema:xsd:SunatAggregateComponents-1"
	NSSummaryDocuments = "urn:sunat:names:specification:ubl:peru:schema:xsd:SummaryDocuments-1"
	NSVoidedDocuments  = "urn:sunat:names:specification:ubl:peru:schema:xsd:VoidedDocuments-1"
)

// UBL version constants.
const (
	UBLVersion21       = "2.1"
	UBLVersion20       = "2.0"
	CustomizationID20  = "2.0"  // For Invoice, CreditNote, DebitNote
	CustomizationIDRC  = "1.1"  // For Resumen Diario
	CustomizationIDRA  = "1.0"  // For Comunicacion de Baja
)
