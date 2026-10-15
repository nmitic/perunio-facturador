package xmlbuilder_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestSpecXSD validates every non-pending spec case whose root has an OASIS UBL
// 2.1 schema in resources/ against that schema. It catches what XPath
// assertions cannot see well: element order inside a sequence, wrong parents,
// unknown elements. RC and RA are SUNAT's own UBL 2.0 documents and have no
// schema in resources/, so they are not covered.
//
// It needs xmllint (Ubuntu: `sudo apt install libxml2-utils`) and skips without
// it, the same way signing needs xmlsec1.
func TestSpecXSD(t *testing.T) {
	xmllint, err := exec.LookPath("xmllint")
	if err != nil {
		t.Skip("xmllint not installed (apt install libxml2-utils); skipping UBL 2.1 XSD validation")
	}
	schemas := map[string]string{
		"Invoice":        "UBL-Invoice-2.1.xsd",
		"CreditNote":     "UBL-CreditNote-2.1.xsd",
		"DebitNote":      "UBL-DebitNote-2.1.xsd",
		"DespatchAdvice": "UBL-DespatchAdvice-2.1.xsd",
	}
	maindoc := filepath.Join("..", "..", "resources", "XSD Schemas 2.1", "maindoc")

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
			xsd, ok := schemas[root]
			if !ok {
				t.Skipf("no UBL 2.1 schema for <%s>", root)
			}
			cmd := exec.Command(xmllint, "--noout", "--nonet", "--schema", filepath.Join(maindoc, xsd), path)
			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out
			if err := cmd.Run(); err != nil {
				t.Errorf("%s does not validate against %s:\n%s", path, xsd, out.String())
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
