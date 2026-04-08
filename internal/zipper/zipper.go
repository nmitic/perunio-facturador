package zipper

import (
	"archive/zip"
	"bytes"
	"fmt"
)

// CreateZIP creates a ZIP archive containing a single XML file.
func CreateZIP(filename string, xmlContent []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	f, err := w.Create(filename + ".xml")
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
