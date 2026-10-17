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

// transcodeToISO8859 converts UTF-8 XML bytes to ISO-8859-1 with correct
// declaration. A rune ISO-8859-1 cannot hold (curly quotes, em dash, €, pasted
// from a word processor) is written as an XML numeric character reference, so
// the text round-trips instead of failing the whole issue. xml.Marshal only
// emits such runes inside text and attribute values, where a reference is legal.
func transcodeToISO8859(utf8Bytes []byte) ([]byte, error) {
	var buf bytes.Buffer
	buf.Grow(len(utf8Bytes) + 64)
	buf.WriteString(`<?xml version="1.0" encoding="ISO-8859-1" standalone="no"?>` + "\n")

	for _, r := range string(utf8Bytes) {
		if b, ok := charmap.ISO8859_1.EncodeRune(r); ok {
			buf.WriteByte(b)
			continue
		}
		fmt.Fprintf(&buf, "&#x%X;", r)
	}
	return buf.Bytes(), nil
}
