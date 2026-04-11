package r2

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// CertificateKey returns the canonical R2 object key for a company certificate.
// Matches the convention used by perunio-backend/src/services/facturador-r2.service.ts.
func CertificateKey(tenantID, companyID, certID string) string {
	return fmt.Sprintf("certificates/%s/%s/%s.pfx", tenantID, companyID, certID)
}

// GetCertificate fetches the raw PFX bytes for the given object key.
func (c *Client) GetCertificate(ctx context.Context, key string) ([]byte, error) {
	out, err := c.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.certificatesBucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("get certificate %q: %w", key, err)
	}
	defer out.Body.Close()

	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, fmt.Errorf("read certificate %q: %w", key, err)
	}
	return data, nil
}

// UploadCertificate writes a PFX file to the certificates bucket.
func (c *Client) UploadCertificate(ctx context.Context, key string, pfx []byte) error {
	_, err := c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.certificatesBucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(pfx),
		ContentType: aws.String("application/x-pkcs12"),
	})
	if err != nil {
		return fmt.Errorf("upload certificate %q: %w", key, err)
	}
	return nil
}

// DeleteCertificate removes a PFX file from the certificates bucket.
func (c *Client) DeleteCertificate(ctx context.Context, key string) error {
	_, err := c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.certificatesBucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("delete certificate %q: %w", key, err)
	}
	return nil
}
