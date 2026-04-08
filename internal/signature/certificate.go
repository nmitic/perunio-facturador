package signature

import (
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/perunio/perunio-facturador/internal/model"
	"software.sslmate.com/src/go-pkcs12"
)

// ParsedCertificate holds the extracted key and certificate from a PFX file.
type ParsedCertificate struct {
	PrivateKey  *rsa.PrivateKey
	Certificate *x509.Certificate
}

// LoadCertificateFromURL downloads a PFX from a presigned URL and extracts the key and cert.
func LoadCertificateFromURL(url, password string) (*ParsedCertificate, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download certificate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download certificate: status %d", resp.StatusCode)
	}

	pfxData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read certificate body: %w", err)
	}

	return ParsePFX(pfxData, password)
}

// ParsePFX extracts the private key and certificate from PFX/P12 data.
func ParsePFX(pfxData []byte, password string) (*ParsedCertificate, error) {
	privateKey, cert, err := pkcs12.Decode(pfxData, password)
	if err != nil {
		return nil, fmt.Errorf("decode PFX: %w", err)
	}

	rsaKey, ok := privateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("certificate key is not RSA")
	}

	return &ParsedCertificate{
		PrivateKey:  rsaKey,
		Certificate: cert,
	}, nil
}

// CertificateMetadata extracts metadata from a PFX file for validation.
func CertificateMetadata(pfxData []byte, password string) (*model.CertificateInfo, error) {
	parsed, err := ParsePFX(pfxData, password)
	if err != nil {
		return nil, err
	}

	cert := parsed.Certificate
	now := time.Now()

	return &model.CertificateInfo{
		SerialNumber: cert.SerialNumber.Text(16),
		Issuer:       cert.Issuer.String(),
		Subject:      cert.Subject.String(),
		ValidFrom:    cert.NotBefore,
		ValidTo:      cert.NotAfter,
		IsExpired:    now.After(cert.NotAfter),
		DaysUntilExp: int(cert.NotAfter.Sub(now).Hours() / 24),
	}, nil
}
