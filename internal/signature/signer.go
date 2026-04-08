package signature

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"fmt"

	"github.com/beevik/etree"
)

const (
	nsDS              = "http://www.w3.org/2000/09/xmldsig#"
	algC14N           = "http://www.w3.org/TR/2001/REC-xml-c14n-20010315"
	algC14NWithComments = "http://www.w3.org/TR/2001/REC-xml-c14n-20010315#WithComments"
	algRSASHA1        = "http://www.w3.org/2000/09/xmldsig#rsa-sha1"
	algSHA1           = "http://www.w3.org/2000/09/xmldsig#sha1"
	algEnvelopedSig   = "http://www.w3.org/2000/09/xmldsig#enveloped-signature"
	signatureID       = "signatureKG"
)

// SignXML takes unsigned ISO-8859-1 XML bytes (with empty ext:ExtensionContent)
// and returns signed XML bytes with the ds:Signature injected.
func SignXML(xmlBytes []byte, cert *x509.Certificate, key *rsa.PrivateKey) ([]byte, error) {
	doc := etree.NewDocument()
	doc.ReadSettings.CharsetReader = nil // accept ISO-8859-1
	if err := doc.ReadFromBytes(xmlBytes); err != nil {
		return nil, fmt.Errorf("parse XML for signing: %w", err)
	}

	// Find ext:ExtensionContent element to inject signature
	extContent := doc.FindElement("//ext:ExtensionContent")
	if extContent == nil {
		return nil, fmt.Errorf("ext:ExtensionContent not found in XML")
	}

	// Step 1: Compute digest of the document (before adding signature)
	// We need to serialize the document as-is for the digest
	canonicalBytes, err := canonicalize(doc)
	if err != nil {
		return nil, fmt.Errorf("canonicalize for digest: %w", err)
	}

	digestHash := sha1.Sum(canonicalBytes)
	digestValue := base64.StdEncoding.EncodeToString(digestHash[:])

	// Step 2: Build SignedInfo
	signedInfo := buildSignedInfo(digestValue)

	// Step 3: Canonicalize SignedInfo and sign it
	siDoc := etree.NewDocument()
	siDoc.SetRoot(signedInfo.Copy())
	siBytes, err := canonicalizeElement(siDoc)
	if err != nil {
		return nil, fmt.Errorf("canonicalize SignedInfo: %w", err)
	}

	siHash := sha1.Sum(siBytes)
	sigBytes, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA1, siHash[:])
	if err != nil {
		return nil, fmt.Errorf("RSA sign: %w", err)
	}
	signatureValue := base64.StdEncoding.EncodeToString(sigBytes)

	// Step 4: Build complete ds:Signature element
	dsSignature := buildDSSignature(signedInfo, signatureValue, cert)

	// Step 5: Inject into ext:ExtensionContent
	extContent.AddChild(dsSignature)

	// Step 6: Serialize back
	doc.Indent(0)
	result, err := doc.WriteToBytes()
	if err != nil {
		return nil, fmt.Errorf("serialize signed XML: %w", err)
	}

	return result, nil
}

// DigestValue extracts the ds:DigestValue from a signed XML document (for QR code).
func DigestValue(signedXML []byte) (string, error) {
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(signedXML); err != nil {
		return "", fmt.Errorf("parse signed XML: %w", err)
	}

	el := doc.FindElement("//ds:DigestValue")
	if el == nil {
		return "", fmt.Errorf("ds:DigestValue not found")
	}
	return el.Text(), nil
}

func buildSignedInfo(digestValue string) *etree.Element {
	si := etree.NewElement("ds:SignedInfo")

	cm := si.CreateElement("ds:CanonicalizationMethod")
	cm.CreateAttr("Algorithm", algC14NWithComments)

	sm := si.CreateElement("ds:SignatureMethod")
	sm.CreateAttr("Algorithm", algRSASHA1)

	ref := si.CreateElement("ds:Reference")
	ref.CreateAttr("URI", "")

	transforms := ref.CreateElement("ds:Transforms")
	transform := transforms.CreateElement("ds:Transform")
	transform.CreateAttr("Algorithm", algEnvelopedSig)

	dm := ref.CreateElement("ds:DigestMethod")
	dm.CreateAttr("Algorithm", algSHA1)

	dv := ref.CreateElement("ds:DigestValue")
	dv.SetText(digestValue)

	return si
}

func buildDSSignature(signedInfo *etree.Element, signatureValue string, cert *x509.Certificate) *etree.Element {
	sig := etree.NewElement("ds:Signature")
	sig.CreateAttr("xmlns:ds", nsDS)
	sig.CreateAttr("Id", signatureID)

	sig.AddChild(signedInfo)

	sv := sig.CreateElement("ds:SignatureValue")
	sv.SetText(signatureValue)

	ki := sig.CreateElement("ds:KeyInfo")
	x509Data := ki.CreateElement("ds:X509Data")
	x509Cert := x509Data.CreateElement("ds:X509Certificate")
	x509Cert.SetText(base64.StdEncoding.EncodeToString(cert.Raw))

	return sig
}

func canonicalize(doc *etree.Document) ([]byte, error) {
	doc.WriteSettings.CanonicalEndTags = true
	doc.WriteSettings.CanonicalText = true
	doc.WriteSettings.CanonicalAttrVal = true
	return doc.WriteToBytes()
}

func canonicalizeElement(doc *etree.Document) ([]byte, error) {
	doc.WriteSettings.CanonicalEndTags = true
	doc.WriteSettings.CanonicalText = true
	doc.WriteSettings.CanonicalAttrVal = true
	return doc.WriteToBytes()
}
