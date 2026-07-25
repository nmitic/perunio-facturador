package r2

import (
	"bytes"
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// AttachmentKey returns the R2 object key for a supporting file attached to a
// comprobante. It lives under the same per-document prefix as the SUNAT
// artifacts, so DeleteDocumentsByPrefix cleans it up when the document prefix is
// purged:
//
//	documents/{tenantId}/{companyId}/{docId}/attachments/{attachmentId}.{ext}
func AttachmentKey(tenantID, companyID, docID, attachmentID, ext string) string {
	if ext == "" {
		ext = "bin"
	}
	return fmt.Sprintf("documents/%s/%s/%s/attachments/%s.%s", tenantID, companyID, docID, attachmentID, ext)
}

// UploadAttachment stores an arbitrary supporting file under key with the given
// content type (the sniffed/declared MIME of the uploaded file).
func (c *Client) UploadAttachment(ctx context.Context, key, contentType string, data []byte) error {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, err := c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.documentsBucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("upload attachment %q: %w", key, err)
	}
	return nil
}
