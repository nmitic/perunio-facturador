package xmlbuilder

import (
	"bytes"
	"encoding/xml"
	"fmt"

	"golang.org/x/text/encoding/charmap"
)

// marshalISO8859 marshals v to XML and transcodes to ISO-8859-1.
func marshalISO8859(v any) ([]byte, error) {
	utf8Bytes, err := xml.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("xml marshal: %w", err)
	}
	return transcodeToISO8859(utf8Bytes)
}

// transcodeToISO8859 converts UTF-8 XML bytes to ISO-8859-1 with correct declaration.
func transcodeToISO8859(utf8Bytes []byte) ([]byte, error) {
	// Add XML declaration with ISO-8859-1 encoding
	declaration := []byte(`<?xml version="1.0" encoding="ISO-8859-1"?>` + "\n")

	// Encode body to ISO-8859-1
	encoder := charmap.ISO8859_1.NewEncoder()
	encoded, err := encoder.Bytes(utf8Bytes)
	if err != nil {
		return nil, fmt.Errorf("iso-8859-1 encode: %w", err)
	}

	var buf bytes.Buffer
	buf.Write(declaration)
	buf.Write(encoded)
	return buf.Bytes(), nil
}
