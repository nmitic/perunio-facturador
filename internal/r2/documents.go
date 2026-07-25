package r2

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// DocumentFileType is one of the artifact kinds emitted by the SUNAT pipeline.
type DocumentFileType string

const (
	FileXML       DocumentFileType = "xml"
	FileSignedXML DocumentFileType = "signedXml"
	FileZIP       DocumentFileType = "zip"
	FileCDR       DocumentFileType = "cdr"
	FilePDF       DocumentFileType = "pdf"
)

// DocumentKey returns the canonical R2 object key for a generated SUNAT
// artifact. Matches facturador-r2.service.ts so the Node.js backend and the
// Go service produce identical layouts:
//
//	documents/{tenantId}/{companyId}/{docId}/{fileType}.{ext}
func DocumentKey(tenantID, companyID, docID string, fileType DocumentFileType) string {
	ext := documentExt(fileType)
	return fmt.Sprintf("documents/%s/%s/%s/%s.%s", tenantID, companyID, docID, fileType, ext)
}

func documentExt(fileType DocumentFileType) string {
	switch fileType {
	case FileXML, FileSignedXML:
		return "xml"
	case FileZIP, FileCDR:
		return "zip"
	case FilePDF:
		return "pdf"
	default:
		return "bin"
	}
}

func documentContentType(fileType DocumentFileType) string {
	switch fileType {
	case FileXML, FileSignedXML:
		return "application/xml"
	case FileZIP, FileCDR:
		return "application/zip"
	case FilePDF:
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}

// UploadDocumentFile writes one of the SUNAT artifacts to the documents bucket.
func (c *Client) UploadDocumentFile(ctx context.Context, key string, fileType DocumentFileType, data []byte) error {
	_, err := c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.documentsBucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(documentContentType(fileType)),
	})
	if err != nil {
		return fmt.Errorf("upload document %q: %w", key, err)
	}
	return nil
}

// GetDocumentFile fetches a previously uploaded artifact (used by the pipeline
// when re-issuing or polling).
func (c *Client) GetDocumentFile(ctx context.Context, key string) ([]byte, error) {
	out, err := c.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.documentsBucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("get document %q: %w", key, err)
	}
	defer out.Body.Close()

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(out.Body); err != nil {
		return nil, fmt.Errorf("read document %q: %w", key, err)
	}
	return buf.Bytes(), nil
}

// DocumentPresignedURL returns a time-limited download URL the browser can use
// directly. Default expiry mirrors the Node.js helper (24 hours).
//
// When downloadName is non-empty, the presigned URL carries a
// response-content-disposition=attachment override so the browser downloads the
// file (rather than rendering PDF/XML inline in a tab) under that filename, plus
// a matching response-content-type. The SUNAT object key basename is
// meaningless ("pdf.pdf", "zip.zip"), so callers pass the canonical
// {RUC}-{tipo}-{serie}-{correlativo} name here (see DocumentDownloadName).
func (c *Client) DocumentPresignedURL(ctx context.Context, key, downloadName string, expiry time.Duration) (string, error) {
	if expiry == 0 {
		expiry = 24 * time.Hour
	}
	in := &s3.GetObjectInput{
		Bucket: aws.String(c.documentsBucket),
		Key:    aws.String(key),
	}
	if downloadName != "" {
		in.ResponseContentDisposition = aws.String(
			fmt.Sprintf("attachment; filename=%q", downloadName),
		)
		in.ResponseContentType = aws.String(contentTypeForName(downloadName))
	}
	out, err := c.presigner.PresignGetObject(ctx, in, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", fmt.Errorf("presign document %q: %w", key, err)
	}
	return out.URL, nil
}

// DocumentDownloadName builds the canonical SUNAT filename for a stored artifact,
// matching the convention SUNAT, Nubefact and greenter use:
//
//	{RUC}-{docType}-{series}-{correlativo8}          → xml / signedXml / zip / pdf
//	R-{RUC}-{docType}-{series}-{correlativo8}.zip    → cdr (Constancia de Recepción)
func DocumentDownloadName(ruc, docType, series string, correlative int, fileType DocumentFileType) string {
	base := fmt.Sprintf("%s-%s-%s-%08d", ruc, docType, series, correlative)
	ext := documentExt(fileType)
	if fileType == FileCDR {
		return fmt.Sprintf("R-%s.%s", base, ext)
	}
	return fmt.Sprintf("%s.%s", base, ext)
}

func contentTypeForName(name string) string {
	switch {
	case len(name) >= 4 && name[len(name)-4:] == ".pdf":
		return "application/pdf"
	case len(name) >= 4 && name[len(name)-4:] == ".xml":
		return "application/xml"
	case len(name) >= 4 && name[len(name)-4:] == ".zip":
		return "application/zip"
	default:
		return "application/octet-stream"
	}
}

// DeleteDocumentFile removes a single artifact.
func (c *Client) DeleteDocumentFile(ctx context.Context, key string) error {
	_, err := c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.documentsBucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("delete document %q: %w", key, err)
	}
	return nil
}

// DeleteDocumentsByPrefix deletes every object under the given prefix. Used
// when a draft document is removed and all its artifacts must be cleaned up.
func (c *Client) DeleteDocumentsByPrefix(ctx context.Context, prefix string) error {
	var continuation *string
	for {
		out, err := c.s3.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(c.documentsBucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: continuation,
		})
		if err != nil {
			return fmt.Errorf("list documents under %q: %w", prefix, err)
		}
		if len(out.Contents) == 0 {
			return nil
		}

		ids := make([]types.ObjectIdentifier, 0, len(out.Contents))
		for _, obj := range out.Contents {
			ids = append(ids, types.ObjectIdentifier{Key: obj.Key})
		}
		if _, err := c.s3.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(c.documentsBucket),
			Delete: &types.Delete{Objects: ids, Quiet: aws.Bool(true)},
		}); err != nil {
			return fmt.Errorf("delete documents under %q: %w", prefix, err)
		}

		if out.IsTruncated == nil || !*out.IsTruncated {
			return nil
		}
		continuation = out.NextContinuationToken
	}
}
