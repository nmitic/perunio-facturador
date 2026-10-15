package xmlbuilder_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestSpecXSD validates every non-pending spec case against its schema:
// OASIS UBL 2.1 (resources/xsd/2.1) for Invoice, CreditNote, DebitNote and
// DespatchAdvice, and SUNAT's UBL 2.0 schemas (resources/xsd/2.0) for the
// Resumen Diario and the Comunicación de Baja. It catches what XPath assertions
// cannot see well: element order inside a sequence, wrong parents, unknown
// elements.
//
// It needs xmllint (Ubuntu: `sudo apt install libxml2-utils`) and skips without
// it, the same way signing needs xmlsec1.
func TestSpecXSD(t *testing.T) {
	xmllint, err := exec.LookPath("xmllint")
	if err != nil {
		t.Skip("xmllint not installed (apt install libxml2-utils); skipping XSD validation")
	}
	xsd := filepath.Join("..", "..", "resources", "xsd")
	schemas := map[string]string{
		"Invoice":          filepath.Join(xsd, "2.1", "maindoc", "UBL-Invoice-2.1.xsd"),
		"CreditNote":       filepath.Join(xsd, "2.1", "maindoc", "UBL-CreditNote-2.1.xsd"),
		"DebitNote":        filepath.Join(xsd, "2.1", "maindoc", "UBL-DebitNote-2.1.xsd"),
		"DespatchAdvice":   filepath.Join(xsd, "2.1", "maindoc", "UBL-DespatchAdvice-2.1.xsd"),
		"SummaryDocuments": filepath.Join(xsd, "2.0", "maindoc", "UBLPE-SummaryDocuments-1.0.xsd"),
		"VoidedDocuments":  filepath.Join(xsd, "2.0", "maindoc", "UBLPE-VoidedDocuments-1.0.xsd"),
	}

	goldens, err := filepath.Glob(filepath.Join(specRoot, "*", "*", "expected.xml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range goldens {
		name, _ := filepath.Rel(specRoot, filepath.Dir(path))
		t.Run(filepath.ToSlash(name), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			root := rootElementName(data)
			schema, ok := schemas[root]
			if !ok {
				t.Skipf("no schema in resources/xsd for <%s>", root)
			}
			cmd := exec.Command(xmllint, "--noout", "--nonet", "--schema", schema, path)
			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out
			if err := cmd.Run(); err != nil {
				t.Errorf("%s does not validate against %s:\n%s", path, filepath.Base(schema), out.String())
			}
		})
	}
}

// rootElementName returns the local name of the first element in an XML file.
func rootElementName(data []byte) string {
	for i := 0; i < len(data)-1; i++ {
		if data[i] != '<' || data[i+1] == '?' || data[i+1] == '!' {
			continue
		}
		j := i + 1
		for j < len(data) && data[j] != ' ' && data[j] != '>' && data[j] != '/' && data[j] != '\n' {
			j++
		}
		return string(data[i+1 : j])
	}
	return ""
}
