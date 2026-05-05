package zipper

import (
	"archive/zip"
	"bytes"
	"fmt"
)

// CreateZIP creates a ZIP archive containing the signed XML for SUNAT submission.
// SUNAT validators (sendBill/sendSummary) reject any extra entries — the older
// manual reference to a dummy/ folder is obsolete and triggers error 0158
// ("demasiados comprobantes").
func CreateZIP(filename string, xmlContent []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	f, err := w.CreateHeader(&zip.FileHeader{Name: filename + ".xml", Method: zip.Deflate})
	if err != nil {
		return nil, fmt.Errorf("create ZIP entry: %w", err)
	}

	if _, err := f.Write(xmlContent); err != nil {
		return nil, fmt.Errorf("write ZIP entry: %w", err)
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("close ZIP: %w", err)
	}

	return buf.Bytes(), nil
}
