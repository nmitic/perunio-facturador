package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/perunio/perunio-facturador/internal/auth"
	"github.com/perunio/perunio-facturador/internal/model"
)

// CreateCertificateInput is the payload for inserting a new certificate row.
// R2 upload happens outside the tx; this is just the metadata insert.
type CreateCertificateInput struct {
	ID                string // pre-generated UUID so the R2 key can be built first
	Label             string
	R2CertificateKey  string
	EncryptedPassword string
	FingerprintSha256 string
	FileSizeBytes     int
}

// FindCertificateByFingerprint returns a cert id if one already exists for
// (companyId, fingerprint). Used for upload-time dedup.
func (p *Pool) FindCertificateByFingerprint(ctx context.Context, companyID, fingerprint string) (string, error) {
	var id string
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			SELECT id FROM company_certificates
			WHERE company_id = $1 AND fingerprint_sha256 = $2
			LIMIT 1
		`, companyID, fingerprint)
		if scanErr := row.Scan(&id); scanErr != nil {
			if errors.Is(scanErr, pgx.ErrNoRows) {
				return nil
			}
			return scanErr
		}
		return nil
	})
	return id, err
}

// CreateCertificate inserts a new cert row with the given pre-uploaded R2 key
// and encrypted password. Returns the inserted row.
func (p *Pool) CreateCertificate(ctx context.Context, companyID string, in CreateCertificateInput) (*model.Certificate, error) {
	tenantID, ok := auth.TenantIDFromContext(ctx)
	if !ok {
		return nil, ErrTenantContextMissing
	}
	var c model.Certificate
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO company_certificates (
				id, tenant_id, company_id, label, r2_certificate_key,
				encrypted_password, fingerprint_sha256, file_size_bytes
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id, tenant_id, company_id, label, r2_certificate_key,
			          serial_number, issuer, subject, valid_from, valid_to,
			          is_active, fingerprint_sha256, file_size_bytes,
			          created_at, updated_at
		`, in.ID, tenantID, companyID, in.Label, in.R2CertificateKey,
			in.EncryptedPassword, in.FingerprintSha256, in.FileSizeBytes)
		return row.Scan(&c.ID, &c.TenantID, &c.CompanyID, &c.Label, &c.R2CertificateKey,
			&c.SerialNumber, &c.Issuer, &c.Subject, &c.ValidFrom, &c.ValidTo,
			&c.IsActive, &c.FingerprintSha256, &c.FileSizeBytes,
			&c.CreatedAt, &c.UpdatedAt)
	})
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ActivateCertificate sets the given cert as active and deactivates every
// other cert for the same company, in a single transaction. Returns
// ErrNotFound when the cert doesn't exist.
func (p *Pool) ActivateCertificate(ctx context.Context, companyID, certID string) error {
	return p.WithTenant(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE company_certificates
			SET is_active = false, updated_at = now()
			WHERE company_id = $1
		`, companyID); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			UPDATE company_certificates
			SET is_active = true, updated_at = now()
			WHERE company_id = $1 AND id = $2
		`, companyID, certID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// DeleteCertificate removes a cert row and returns its R2 key so the caller
// can clean up the object. Returns ErrNotFound when the cert doesn't exist.
func (p *Pool) DeleteCertificate(ctx context.Context, companyID, certID string) (string, error) {
	var r2Key string
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			DELETE FROM company_certificates
			WHERE company_id = $1 AND id = $2
			RETURNING r2_certificate_key
		`, companyID, certID)
		if scanErr := row.Scan(&r2Key); scanErr != nil {
			if errors.Is(scanErr, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return scanErr
		}
		return nil
	})
	return r2Key, err
}

// ListCertificates returns every certificate row for the given company,
// newest first, scoped to the tenant from context.
func (p *Pool) ListCertificates(ctx context.Context, companyID string) ([]model.Certificate, error) {
	var out []model.Certificate
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, tenant_id, company_id, label, r2_certificate_key,
			       serial_number, issuer, subject, valid_from, valid_to,
			       is_active, fingerprint_sha256, file_size_bytes,
			       created_at, updated_at
			FROM company_certificates
			WHERE company_id = $1
			ORDER BY created_at DESC
		`, companyID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var c model.Certificate
			if err := rows.Scan(&c.ID, &c.TenantID, &c.CompanyID, &c.Label, &c.R2CertificateKey,
				&c.SerialNumber, &c.Issuer, &c.Subject, &c.ValidFrom, &c.ValidTo,
				&c.IsActive, &c.FingerprintSha256, &c.FileSizeBytes,
				&c.CreatedAt, &c.UpdatedAt); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

// GetCertificate looks up a single certificate by id, scoped to the company
// (and the tenant from context). Returns nil when not found.
func (p *Pool) GetCertificate(ctx context.Context, companyID, certID string) (*model.Certificate, error) {
	var c *model.Certificate
	err := p.WithTenant(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			SELECT id, tenant_id, company_id, label, r2_certificate_key,
			       serial_number, issuer, subject, valid_from, valid_to,
			       is_active, fingerprint_sha256, file_size_bytes,
			       created_at, updated_at
			FROM company_certificates
			WHERE company_id = $1 AND id = $2
			LIMIT 1
		`, companyID, certID)

		var got model.Certificate
		if err := row.Scan(&got.ID, &got.TenantID, &got.CompanyID, &got.Label, &got.R2CertificateKey,
			&got.SerialNumber, &got.Issuer, &got.Subject, &got.ValidFrom, &got.ValidTo,
			&got.IsActive, &got.FingerprintSha256, &got.FileSizeBytes,
			&got.CreatedAt, &got.UpdatedAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		c = &got
		return nil
	})
	return c, err
}

// GetActiveCertificateWithSecret loads the active cert for a company along
// with its R2 key and encrypted password. Used by the issue pipeline; not
// exposed to the HTTP API. Returns nil when no active cert exists.
func (p *Pool) GetActiveCertificateWithSecret(ctx context.Context, companyID string) (
	cert *model.Certificate, r2Key string, encryptedPassword string, err error,
) {
	err = p.WithTenant(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			SELECT id, tenant_id, company_id, label, r2_certificate_key,
			       serial_number, issuer, subject, valid_from, valid_to,
			       is_active, fingerprint_sha256, file_size_bytes,
			       created_at, updated_at, encrypted_password
			FROM company_certificates
			WHERE company_id = $1 AND is_active = true
			LIMIT 1
		`, companyID)

		var got model.Certificate
		if scanErr := row.Scan(&got.ID, &got.TenantID, &got.CompanyID, &got.Label, &got.R2CertificateKey,
			&got.SerialNumber, &got.Issuer, &got.Subject, &got.ValidFrom, &got.ValidTo,
			&got.IsActive, &got.FingerprintSha256, &got.FileSizeBytes,
			&got.CreatedAt, &got.UpdatedAt, &encryptedPassword); scanErr != nil {
			if errors.Is(scanErr, pgx.ErrNoRows) {
				return nil
			}
			return scanErr
		}
		cert = &got
		r2Key = got.R2CertificateKey
		return nil
	})
	return cert, r2Key, encryptedPassword, err
}
