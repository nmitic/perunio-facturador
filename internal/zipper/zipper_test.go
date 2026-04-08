package zipper_test

import (
	"archive/zip"
	"bytes"
	"testing"

	"maragu.dev/is"

	"github.com/perunio/perunio-facturador/internal/zipper"
)

func TestCreateZIP(t *testing.T) {
	t.Run("should create a valid ZIP with one XML file", func(t *testing.T) {
		content := []byte(`<?xml version="1.0"?><Invoice/>`)
		filename := "20100113612-01-F001-00000001"

		zipBytes, err := zipper.CreateZIP(filename, content)
		is.NotError(t, err)
		is.True(t, len(zipBytes) > 0)

		// Verify it's a valid ZIP with the correct entry
		reader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
		is.NotError(t, err)
		is.Equal(t, 1, len(reader.File))
		is.Equal(t, filename+".xml", reader.File[0].Name)
	})
}
