package cdr_test

import (
	"archive/zip"
	"bytes"
	"testing"

	"maragu.dev/is"

	"github.com/perunio/perunio-facturador/internal/cdr"
)

func createTestCDRZip(t *testing.T, responseCode, description string) []byte {
	t.Helper()

	cdrXML := []byte(`<?xml version="1.0" encoding="ISO-8859-1"?>
<ApplicationResponse xmlns="urn:oasis:names:specification:ubl:schema:xsd:ApplicationResponse-2"
  xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
  xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
  <DocumentResponse>
    <Response>
      <ResponseCode>` + responseCode + `</ResponseCode>
      <Description>` + description + `</Description>
    </Response>
  </DocumentResponse>
</ApplicationResponse>`)

	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create("R-20100113612-01-F001-00000001.xml")
	is.NotError(t, err)
	_, err = f.Write(cdrXML)
	is.NotError(t, err)
	is.NotError(t, w.Close())

	return buf.Bytes()
}

func TestParse(t *testing.T) {
	t.Run("should parse accepted CDR with response code 0", func(t *testing.T) {
		zipBytes := createTestCDRZip(t, "0", "La Factura numero F001-00000001 ha sido aceptada")

		result, err := cdr.Parse(zipBytes)
		is.NotError(t, err)
		is.Equal(t, "0", result.ResponseCode)
		is.True(t, result.Accepted)
		is.True(t, len(result.Description) > 0)
		is.True(t, len(result.RawBytes) > 0)
	})

	t.Run("should parse rejected CDR with response code 99", func(t *testing.T) {
		zipBytes := createTestCDRZip(t, "99", "El comprobante fue rechazado")

		result, err := cdr.Parse(zipBytes)
		is.NotError(t, err)
		is.Equal(t, "99", result.ResponseCode)
		is.True(t, !result.Accepted)
	})

	t.Run("should return error for invalid ZIP", func(t *testing.T) {
		_, err := cdr.Parse([]byte("not a zip"))
		is.True(t, err != nil)
	})
}
