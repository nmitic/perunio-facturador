package cdr

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"golang.org/x/text/encoding/charmap"

	"github.com/perunio/perunio-facturador/internal/model"
)

// applicationResponse is the UBL 2.0 ApplicationResponse for CDR parsing.
type applicationResponse struct {
	XMLName          xml.Name          `xml:"ApplicationResponse"`
	DocumentResponse []documentResponse `xml:"DocumentResponse"`
}

type documentResponse struct {
	Response response `xml:"Response"`
}

type response struct {
	ResponseCode string `xml:"ResponseCode"`
	Description  string `xml:"Description"`
}

// Parse extracts and parses a CDR from a ZIP byte slice.
func Parse(zipBytes []byte) (*model.CDR, error) {
	reader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, fmt.Errorf("open CDR ZIP: %w", err)
	}

	// Find the R-*.xml file
	var cdrXMLBytes []byte
	for _, f := range reader.File {
		if strings.HasPrefix(f.Name, "R-") && strings.HasSuffix(f.Name, ".xml") {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("open CDR XML: %w", err)
			}
			cdrXMLBytes, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("read CDR XML: %w", err)
			}
			break
		}
	}

	if cdrXMLBytes == nil {
		return nil, fmt.Errorf("no R-*.xml file found in CDR ZIP")
	}

	return parseApplicationResponse(cdrXMLBytes, zipBytes)
}

func parseApplicationResponse(xmlBytes, rawZipBytes []byte) (*model.CDR, error) {
	var appResp applicationResponse
	decoder := xml.NewDecoder(bytes.NewReader(xmlBytes))
	decoder.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		if strings.EqualFold(charset, "ISO-8859-1") {
			return charmap.ISO8859_1.NewDecoder().Reader(input), nil
		}
		return nil, fmt.Errorf("unsupported charset: %s", charset)
	}
	if err := decoder.Decode(&appResp); err != nil {
		return nil, fmt.Errorf("parse ApplicationResponse XML: %w", err)
	}

	cdr := &model.CDR{
		RawBytes: rawZipBytes,
	}

	if len(appResp.DocumentResponse) > 0 {
		resp := appResp.DocumentResponse[0].Response
		cdr.ResponseCode = resp.ResponseCode
		cdr.Description = resp.Description
	}

	cdr.Accepted = cdr.ResponseCode == "0"

	// Extract notes/observations from additional DocumentResponses
	for i := 1; i < len(appResp.DocumentResponse); i++ {
		resp := appResp.DocumentResponse[i].Response
		if resp.Description != "" {
			cdr.Notes = append(cdr.Notes, resp.Description)
		}
	}

	return cdr, nil
}
