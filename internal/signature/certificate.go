package signature

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

// ParsedCertificate holds the RSA private key and X.509 certificate used for
// XMLDSig signing. perunio-backend extracts these from the user's PFX and
// stores them as PEM in the DB; we parse them on first use and cache the
// result (see cache.go).
type ParsedCertificate struct {
	PrivateKey  *rsa.PrivateKey
	Certificate *x509.Certificate
}

// ParsePEMKeyAndCert parses a PKCS#8 private key PEM and an X.509 certificate
// PEM into a ParsedCertificate. The key must be RSA — SUNAT XMLDSig is
// RSA-SHA1 only.
func ParsePEMKeyAndCert(privateKeyPEM, certPEM []byte) (*ParsedCertificate, error) {
	keyBlock, _ := pem.Decode(privateKeyPEM)
	if keyBlock == nil {
		return nil, fmt.Errorf("decode private key PEM: no PEM block found")
	}
	parsedKey, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse PKCS#8 private key: %w", err)
	}
	rsaKey, ok := parsedKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not RSA")
	}

	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, fmt.Errorf("decode certificate PEM: no PEM block found")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse X.509 certificate: %w", err)
	}

	return &ParsedCertificate{
		PrivateKey:  rsaKey,
		Certificate: cert,
	}, nil
}
