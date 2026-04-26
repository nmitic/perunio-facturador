package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// ActiveCertificateForSigning is the minimal set of fields the signing pipeline
// reads for a tenant's active certificate. Cert management (upload, list,
// activate, delete) lives in perunio-backend now; this service only signs.
type ActiveCertificateForSigning struct {
	CertID                  string
	EncryptedPrivateKeyPEM  string
	CertificatePEM          string
}

// GetActiveCertificateForSigning loads the active cert for a company. The
// private key is AES-256-GCM encrypted (same wire format as Node's
// EncryptionUtils.encryptCredential — `iv:authTag:ciphertext` hex).
// Returns (nil, nil) when no active cert exists.
func (p *Pool) GetActiveCertificateForSigning(ctx context.Context, companyID string) (*ActiveCertificateForSigning, error) {
	var out *ActiveCertificateForSigning
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			SELECT id, encrypted_private_key_pem, certificate_pem
			FROM company_certificates
			WHERE company_id = $1 AND is_active = true
			LIMIT 1
		`, companyID)

		var got ActiveCertificateForSigning
		if scanErr := row.Scan(&got.CertID, &got.EncryptedPrivateKeyPEM, &got.CertificatePEM); scanErr != nil {
			if errors.Is(scanErr, pgx.ErrNoRows) {
				return nil
			}
			return scanErr
		}
		out = &got
		return nil
	})
	return out, err
}
