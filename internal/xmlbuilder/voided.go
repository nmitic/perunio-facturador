package xmlbuilder

import (
	"encoding/xml"
	"fmt"

	"github.com/perunio/perunio-facturador/internal/model"
)

// voidedDocuments is the UBL 2.0 VoidedDocuments root element (Comunicacion de Baja - RA).
type voidedDocuments struct {
	XMLName         xml.Name `xml:"VoidedDocuments"`
	XMLNS           string   `xml:"xmlns,attr"`
	XMLNSCAC        string   `xml:"xmlns:cac,attr"`
	XMLNSCBC        string   `xml:"xmlns:cbc,attr"`
	XMLNSDS         string   `xml:"xmlns:ds,attr"`
	XMLNSEXT        string   `xml:"xmlns:ext,attr"`
	XMLNSSAC        string   `xml:"xmlns:sac,attr"`
	UBLExtensions   ublExtensions
	UBLVersionID    string `xml:"cbc:UBLVersionID"`
	CustomizationID string `xml:"cbc:CustomizationID"`
	ID              string `xml:"cbc:ID"`
	ReferenceDate   string `xml:"cbc:ReferenceDate"`
	IssueDate       string `xml:"cbc:IssueDate"`
	Signature       cacSignature
	SupplierParty   summarySupplierParty
	VoidedLines     []voidedDocumentsLine `xml:"sac:VoidedDocumentsLine"`
}

type voidedDocumentsLine struct {
	LineID           string `xml:"cbc:LineID"`
	DocumentTypeCode string `xml:"cbc:DocumentTypeCode"`
	DocumentSerialID string `xml:"sac:DocumentSerialID"`
	DocumentNumberID string `xml:"sac:DocumentNumberID"`
	VoidReasonDesc   string `xml:"sac:VoidReasonDescription"`
}

// BuildVoidedXML creates UBL 2.0 VoidedDocuments XML bytes.
func BuildVoidedXML(req model.VoidRequest) ([]byte, error) {
	voidID := fmt.Sprintf("%s-RA-%s-%05d",
		req.SupplierRUC,
		formatDateCompact(req.IssueDate),
		req.Correlative)

	doc := voidedDocuments{
		XMLNS:    NSVoidedDocuments,
		XMLNSCAC: NSCAC,
		XMLNSCBC: NSCBC,
		XMLNSDS:  NSDS,
		XMLNSEXT: NSEXT,
		XMLNSSAC: NSSAC,

		UBLExtensions: ublExtensions{
			Extension: []ublExtension{{ExtensionContent: newExtensionContent()}},
		},

		UBLVersionID:    UBLVersion20,
		CustomizationID: CustomizationIDRA,
		ID:              voidID,
		ReferenceDate:   req.IssueDate, // reference date = issue date for RA
		IssueDate:       req.IssueDate,

		Signature: newCACSignature(req.SupplierRUC, req.SupplierName),

		SupplierParty: summarySupplierParty{
			CustomerAssignedID:  req.SupplierRUC,
			AdditionalAccountID: "6",
			Party: summaryParty{
				PartyLegalEntity: summaryPartyLegalEntity{
					RegistrationName: req.SupplierName,
				},
			},
		},
	}

	for _, item := range req.Items {
		doc.VoidedLines = append(doc.VoidedLines, voidedDocumentsLine{
			LineID:           fmt.Sprint(item.LineNumber),
			DocumentTypeCode: item.DocType,
			DocumentSerialID: item.Series,
			DocumentNumberID: fmt.Sprint(item.Correlative),
			VoidReasonDesc:   item.VoidReason,
		})
	}

	return marshalISO8859(&doc)
}

// VoidFilename returns the filename for a Comunicacion de Baja.
func VoidFilename(ruc, issueDate string, correlative int) string {
	return fmt.Sprintf("%s-RA-%s-%05d", ruc, formatDateCompact(issueDate), correlative)
}
