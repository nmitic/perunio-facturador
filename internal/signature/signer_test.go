package signature_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"strings"
	"testing"
	"time"

	"maragu.dev/is"
	"software.sslmate.com/src/go-pkcs12"

	"github.com/perunio/perunio-facturador/internal/model"
	"github.com/perunio/perunio-facturador/internal/signature"
	"github.com/perunio/perunio-facturador/internal/xmlbuilder"
)

func generateTestPFX(t *testing.T) ([]byte, string) {
	t.Helper()
	password := "test123"

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	is.NotError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:       []string{"Test Company SAC"},
			OrganizationalUnit: []string{"20100113612"},
			Country:            []string{"PE"},
		},
		NotBefore: time.Now().Add(-24 * time.Hour),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	is.NotError(t, err)

	cert, err := x509.ParseCertificate(certDER)
	is.NotError(t, err)

	pfxData, err := pkcs12.Encode(rand.Reader, key, cert, nil, password)
	is.NotError(t, err)

	return pfxData, password
}

func TestSignXML(t *testing.T) {
	t.Run("should inject ds:Signature into ext:ExtensionContent", func(t *testing.T) {
		req := model.IssueRequest{
			SupplierRUC:       "20100113612",
			SupplierName:      "TEST COMPANY SAC",
			SupplierAddress:   "AV TEST 123",
			EstablishmentCode: "0000",
			DocType:           "01",
			Series:            "F001",
			Correlative:       1,
			IssueDate:         "2024-01-15",
			IssueTime:         "10:00:00",
			CurrencyCode:      "PEN",
			OperationType:     "0101",
			CustomerDocType:   "6",
			CustomerDocNumber: "20601327318",
			CustomerName:      "CLIENTE TEST SRL",
			Subtotal:          "100.00",
			TotalIGV:          "18.00",
			TotalAmount:       "118.00",
			TaxInclusiveAmount: "118.00",
			Items: []model.LineItem{
				{
					LineNumber: 1, Description: "ITEM 1", Quantity: "1",
					UnitCode: "NIU", UnitPrice: "100.00", UnitPriceWithTax: "118.00",
					TaxExemptionReasonCode: "10", IGVAmount: "18.00",
					LineTotal: "100.00", PriceTypeCode: "01",
				},
			},
		}

		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)

		pfxData, password := generateTestPFX(t)
		parsed, err := signature.ParsePFX(pfxData, password)
		is.NotError(t, err)

		signed, err := signature.SignXML(xmlBytes, parsed.Certificate, parsed.PrivateKey)
		is.NotError(t, err)

		xml := string(signed)

		is.True(t, strings.Contains(xml, "ds:Signature"), "should contain ds:Signature element")
		is.True(t, strings.Contains(xml, "ds:SignedInfo"), "should contain ds:SignedInfo")
		is.True(t, strings.Contains(xml, "ds:SignatureValue"), "should contain ds:SignatureValue")
		is.True(t, strings.Contains(xml, "ds:X509Certificate"), "should contain ds:X509Certificate")
		is.True(t, strings.Contains(xml, "ds:DigestValue"), "should contain ds:DigestValue")
		is.True(t, strings.Contains(xml, `Id="signatureKG"`), "should have signature ID")
	})
}

func TestDigestValue(t *testing.T) {
	t.Run("should extract ds:DigestValue from signed XML", func(t *testing.T) {
		req := model.IssueRequest{
			SupplierRUC: "20100113612", SupplierName: "TEST", DocType: "01",
			Series: "F001", Correlative: 1, IssueDate: "2024-01-15",
			CurrencyCode: "PEN", CustomerDocType: "6", CustomerDocNumber: "20601327318",
			CustomerName: "CLI", Subtotal: "100.00", TotalIGV: "18.00",
			TotalAmount: "118.00", TaxInclusiveAmount: "118.00",
			Items: []model.LineItem{
				{LineNumber: 1, Description: "X", Quantity: "1", UnitCode: "NIU",
					UnitPrice: "100.00", UnitPriceWithTax: "118.00",
					TaxExemptionReasonCode: "10", IGVAmount: "18.00",
					LineTotal: "100.00", PriceTypeCode: "01"},
			},
		}
		xmlBytes, err := xmlbuilder.BuildDocumentXML(req)
		is.NotError(t, err)

		pfxData, password := generateTestPFX(t)
		parsed, err := signature.ParsePFX(pfxData, password)
		is.NotError(t, err)

		signed, err := signature.SignXML(xmlBytes, parsed.Certificate, parsed.PrivateKey)
		is.NotError(t, err)

		digest, err := signature.DigestValue(signed)
		is.NotError(t, err)
		is.True(t, len(digest) > 0, "digest should not be empty")
	})
}

func TestCertificateMetadata(t *testing.T) {
	t.Run("should extract certificate metadata from PFX", func(t *testing.T) {
		pfxData, password := generateTestPFX(t)

		info, err := signature.CertificateMetadata(pfxData, password)
		is.NotError(t, err)
		is.True(t, !info.IsExpired, "test cert should not be expired")
		is.True(t, info.DaysUntilExp > 300, "should have >300 days until expiry")
		is.True(t, strings.Contains(info.Subject, "20100113612"), "subject should contain RUC")
	})
}
