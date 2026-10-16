package xmlbuilder_test

// The XML output spec suite. Every case lives in spec/<family>/<case>/ as plain
// data — input.json, spec.yaml, expected.xml — and this file is the only Go that
// knows about them. See spec/README.md for the format; the point of keeping the
// cases out of Go is that a person, another AI, or xmllint can check them
// without reading this file.
//
//	go test ./internal/xmlbuilder -run TestSpec            # run every case
//	go test ./internal/xmlbuilder -run TestSpec/invoice    # one family
//	go test ./internal/xmlbuilder -run TestSpec -update    # rewrite expected.xml

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/antchfx/xmlquery"
	"github.com/antchfx/xpath"
	"github.com/beevik/etree"
	"golang.org/x/text/encoding/charmap"
	"gopkg.in/yaml.v3"

	sunat "github.com/nmitic/perunio-sunat-catalogs/sunat"
	"github.com/perunio/perunio-facturador/internal/model"
	"github.com/perunio/perunio-facturador/internal/xmlbuilder"
)

var update = flag.Bool("update", false, "rewrite spec/**/expected.xml from the current builder output")

const specRoot = "spec"

// specNS is the fixed prefix table every spec XPath is written against. The
// root element is always addressed as /* so no prefix is needed for the
// document's default namespace.
var specNS = map[string]string{
	"cac": sunat.UblNamespaceUri(sunat.UblNamespaceCAC),
	"cbc": sunat.UblNamespaceUri(sunat.UblNamespaceCBC),
	"ext": sunat.UblNamespaceUri(sunat.UblNamespaceEXT),
	"sac": sunat.UblNamespaceUri(sunat.UblNamespaceSAC),
	"ds":  sunat.UblNamespaceUri(sunat.UblNamespaceDS),
}

// specCase is one spec.yaml.
type specCase struct {
	Case     string   `yaml:"case"`
	Document string   `yaml:"document"` // cpe | summary | voided | despatch | filename
	Why      string   `yaml:"why"`
	Sources  []string `yaml:"sources"`
	Pending  string   `yaml:"pending"`
	// Effective is the YYYY-MM-DD from which SUNAT enforces the rule a case
	// pins, when that date is not yet in the past (or only recently passed).
	// Informational: it is shown with the pending reason.
	Effective string `yaml:"effective"`

	// Exactly one of these outcome kinds applies.
	Assert        []assertion `yaml:"assert"`
	ErrorContains string      `yaml:"error_contains"` // the builder must refuse the input
	Output        string      `yaml:"output"`         // filename cases: the exact string
}

type assertion struct {
	XPath   string   `yaml:"xpath"`
	Equals  *string  `yaml:"equals"`
	Count   *int     `yaml:"count"`
	Absent  bool     `yaml:"absent"`
	Matches string   `yaml:"matches"`
	True    bool     `yaml:"true"`
	Before  []string `yaml:"before"`
	Why     string   `yaml:"why"`
	Sunat   string   `yaml:"sunat"`
}

// invariant is one entry of spec/invariants.yaml: an XPath 1.0 expression that
// must be true for every document whose root is listed in AppliesTo.
type invariant struct {
	ID        string   `yaml:"id"`
	AppliesTo []string `yaml:"applies_to"`
	Assert    string   `yaml:"assert"`
	Sunat     string   `yaml:"sunat"`
	Why       string   `yaml:"why"`
}

func TestSpec(t *testing.T) {
	invariants := loadInvariants(t)

	var dirs []string
	err := filepath.WalkDir(specRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "spec.yaml" {
			dirs = append(dirs, filepath.Dir(path))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", specRoot, err)
	}
	if len(dirs) == 0 {
		t.Fatalf("no spec cases found under %s", specRoot)
	}

	for _, dir := range dirs {
		name, _ := filepath.Rel(specRoot, dir)
		t.Run(filepath.ToSlash(name), func(t *testing.T) {
			runCase(t, dir, invariants)
		})
	}
}

func runCase(t *testing.T, dir string, invariants []invariant) {
	var sc specCase
	readYAML(t, filepath.Join(dir, "spec.yaml"), &sc)
	if sc.Case == "" || sc.Why == "" {
		t.Fatalf("spec.yaml must set both case and why")
	}
	if want := filepath.Base(dir); !strings.HasSuffix(want, sc.Case) {
		t.Fatalf("spec.yaml case %q does not match its directory %q", sc.Case, want)
	}
	if sc.Effective != "" {
		if _, err := time.Parse(time.DateOnly, sc.Effective); err != nil {
			t.Fatalf("spec.yaml effective %q is not a YYYY-MM-DD date", sc.Effective)
		}
	}
	input, err := os.ReadFile(filepath.Join(dir, "input.json"))
	if err != nil {
		t.Fatalf("read input.json: %v", err)
	}

	failures := evaluateCase(t, dir, sc, input, invariants)

	if sc.Pending != "" {
		if len(failures) == 0 {
			t.Fatalf("case is marked pending (%q) but now passes: remove `pending` from spec.yaml and run -update", sc.Pending)
		}
		label := "PENDING"
		if sc.Effective != "" {
			label += " (effective " + sc.Effective + ")"
		}
		t.Skipf("%s %s\n  first failure: %s", label, sc.Pending, failures[0])
	}
	for _, f := range failures {
		t.Error(f)
	}
}

// evaluateCase builds the document and returns every failed expectation as a
// human-readable line. It never calls t.Error itself so pending cases can be
// reported as a skip.
func evaluateCase(t *testing.T, dir string, sc specCase, input []byte, invariants []invariant) []string {
	t.Helper()

	if sc.Document == "filename" {
		got, err := buildFilename(input)
		if err != nil {
			return []string{err.Error()}
		}
		if got != sc.Output {
			return []string{fmt.Sprintf("filename = %q, want %q", got, sc.Output)}
		}
		return nil
	}

	raw, buildErr := buildDocument(sc.Document, input)

	if sc.ErrorContains != "" {
		switch {
		case buildErr == nil:
			return []string{fmt.Sprintf("builder accepted the input, want an error containing %q", sc.ErrorContains)}
		case !strings.Contains(buildErr.Error(), sc.ErrorContains):
			return []string{fmt.Sprintf("builder error %q does not contain %q", buildErr, sc.ErrorContains)}
		}
		return nil
	}
	if buildErr != nil {
		return []string{fmt.Sprintf("builder returned error: %v", buildErr)}
	}

	doc, err := xmlquery.Parse(bytes.NewReader(raw))
	if err != nil {
		return []string{fmt.Sprintf("output is not well-formed XML: %v", err)}
	}

	var failures []string
	for _, a := range sc.Assert {
		if msg := checkAssertion(doc, a); msg != "" {
			failures = append(failures, msg)
		}
	}
	failures = append(failures, checkInvariants(doc, raw, invariants)...)

	// The golden file locks what we emit for cases that are believed correct.
	// Pending cases have no golden: their current output is known to be wrong.
	if sc.Pending == "" {
		if msg := checkGolden(t, dir, raw); msg != "" {
			failures = append(failures, msg)
		}
	}
	return failures
}

// ---------------------------------------------------------------------------
// Builders
// ---------------------------------------------------------------------------

// despatchInput is the JSON shape of a despatch case's input.json.
type despatchInput struct {
	RUC              string               `json:"ruc"`
	CompanyName      string               `json:"companyName"`
	CompanyAddress   string               `json:"companyAddress"`
	EventBaseDocType string               `json:"eventBaseDocType"`
	Despatch         model.Despatch       `json:"despatch"`
	Lines            []model.DespatchLine `json:"lines"`
}

func buildDocument(kind string, input []byte) ([]byte, error) {
	switch kind {
	case "cpe":
		var req model.IssueRequest
		if err := strictJSON(input, &req); err != nil {
			return nil, err
		}
		return xmlbuilder.BuildDocumentXML(req)
	case "summary":
		var req model.SummaryRequest
		if err := strictJSON(input, &req); err != nil {
			return nil, err
		}
		return xmlbuilder.BuildSummaryXML(req)
	case "voided":
		var req model.VoidRequest
		if err := strictJSON(input, &req); err != nil {
			return nil, err
		}
		return xmlbuilder.BuildVoidedXML(req)
	case "despatch":
		var in despatchInput
		if err := strictJSON(input, &in); err != nil {
			return nil, err
		}
		return xmlbuilder.BuildDespatchXML(xmlbuilder.DespatchXMLInput{
			Despatch:         &in.Despatch,
			Lines:            in.Lines,
			RUC:              in.RUC,
			CompanyName:      in.CompanyName,
			CompanyAddress:   in.CompanyAddress,
			EventBaseDocType: in.EventBaseDocType,
		})
	default:
		return nil, fmt.Errorf("spec.yaml: unknown document %q (want cpe|summary|voided|despatch|filename)", kind)
	}
}

// filenameInput is the JSON shape of a filename case's input.json.
type filenameInput struct {
	Kind             string `json:"kind"` // cpe | summary | voided | despatch
	RUC              string `json:"ruc"`
	DocType          string `json:"docType"`
	Series           string `json:"series"`
	Correlative      int    `json:"correlative"`
	IssueDate        string `json:"issueDate"`
	EventBaseDocType string `json:"eventBaseDocType"`
}

func buildFilename(input []byte) (string, error) {
	var in filenameInput
	if err := strictJSON(input, &in); err != nil {
		return "", err
	}
	switch in.Kind {
	case "cpe":
		return xmlbuilder.Filename(in.RUC, in.DocType, in.Series, in.Correlative), nil
	case "summary":
		return xmlbuilder.SummaryFilename(in.RUC, in.IssueDate, in.Correlative), nil
	case "voided":
		return xmlbuilder.VoidFilename(in.RUC, in.IssueDate, in.Correlative), nil
	case "despatch":
		return xmlbuilder.DespatchFilename(in.RUC, in.DocType, in.Series, in.Correlative, in.EventBaseDocType), nil
	default:
		return "", fmt.Errorf("input.json: unknown filename kind %q", in.Kind)
	}
}

// strictJSON rejects unknown fields so a typo in input.json fails loudly
// instead of silently testing a default.
func strictJSON(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("input.json: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Assertions
// ---------------------------------------------------------------------------

func checkAssertion(doc *xmlquery.Node, a assertion) string {
	label := a.XPath
	if len(a.Before) > 0 {
		label = strings.Join(a.Before, " before ")
	}
	fail := func(format string, args ...any) string {
		msg := fmt.Sprintf("%s: %s", label, fmt.Sprintf(format, args...))
		if a.Sunat != "" {
			msg += " [SUNAT " + a.Sunat + "]"
		}
		if a.Why != "" {
			msg += " — " + a.Why
		}
		return msg
	}

	if len(a.Before) > 0 {
		if len(a.Before) != 2 {
			return fail("before needs exactly two paths")
		}
		first, err := firstNode(doc, a.Before[0])
		if err != nil {
			return fail("%v", err)
		}
		second, err := firstNode(doc, a.Before[1])
		if err != nil {
			return fail("%v", err)
		}
		if docOrder(doc, first) >= docOrder(doc, second) {
			return fail("first path does not precede the second in document order")
		}
		return ""
	}

	res, err := evaluate(doc, a.XPath)
	if err != nil {
		return fail("%v", err)
	}

	switch {
	case a.Absent:
		if n := len(nodeValues(res)); n != 0 {
			return fail("want absent, found %d node(s)", n)
		}
	case a.Count != nil:
		if n := countOf(res); n != *a.Count {
			return fail("count = %d, want %d", n, *a.Count)
		}
	case a.True:
		if !truthy(res) {
			return fail("expression is false")
		}
	case a.Equals != nil:
		got, numeric, err := singleValue(res)
		if err != nil {
			return fail("%v", err)
		}
		if !valuesEqual(got, numeric, *a.Equals) {
			return fail("got %q, want %q", got, *a.Equals)
		}
	case a.Matches != "":
		got, _, err := singleValue(res)
		if err != nil {
			return fail("%v", err)
		}
		re, err := regexp.Compile(a.Matches)
		if err != nil {
			return fail("bad regex: %v", err)
		}
		if !re.MatchString(got) {
			return fail("got %q, want match for /%s/", got, a.Matches)
		}
	default:
		// A bare xpath means "exists".
		if len(nodeValues(res)) == 0 && !truthy(res) {
			return fail("no node matches")
		}
	}
	return ""
}

func checkInvariants(doc *xmlquery.Node, raw []byte, invariants []invariant) []string {
	root := rootName(doc)
	var failures []string

	// The prolog is not reachable from XPath, so it is the one invariant that
	// lives in Go. SUNAT reads the declared encoding to decode the file.
	if !bytes.HasPrefix(raw, []byte(`<?xml version="1.0" encoding="ISO-8859-1"`)) {
		failures = append(failures, "[INV-PROLOG] document must open with an ISO-8859-1 XML declaration")
	}

	for _, inv := range invariants {
		if !appliesTo(inv, root) {
			continue
		}
		res, err := evaluate(doc, inv.Assert)
		if err != nil {
			failures = append(failures, fmt.Sprintf("[%s] %v", inv.ID, err))
			continue
		}
		if !truthy(res) {
			failures = append(failures, fmt.Sprintf("[%s] invariant violated [SUNAT %s] — %s", inv.ID, inv.Sunat, inv.Why))
		}
	}
	return failures
}

func appliesTo(inv invariant, root string) bool {
	for _, r := range inv.AppliesTo {
		if r == root || r == "*" {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Golden files
// ---------------------------------------------------------------------------

// checkGolden compares the output against expected.xml: a UTF-8, indented copy
// of the exact bytes the builder produced (indentation is added between
// elements only; text and attribute values are untouched). The byte encoding
// itself is covered by the INV-PROLOG invariant and the encoding cases.
func checkGolden(t *testing.T, dir string, raw []byte) string {
	t.Helper()
	pretty, err := prettyUTF8(raw)
	if err != nil {
		return fmt.Sprintf("golden: %v", err)
	}
	path := filepath.Join(dir, "expected.xml")
	if *update {
		if err := os.WriteFile(path, pretty, 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		return ""
	}
	want, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "golden: expected.xml is missing — run with -update and review it"
	}
	if err != nil {
		return fmt.Sprintf("golden: %v", err)
	}
	if !bytes.Equal(pretty, want) {
		return "golden: output differs from expected.xml — " + firstDiff(want, pretty) +
			" (if the change is intended, run with -update and review the diff)"
	}
	return ""
}

func prettyUTF8(raw []byte) ([]byte, error) {
	utf8, err := charmap.ISO8859_1.NewDecoder().Bytes(raw)
	if err != nil {
		return nil, err
	}
	utf8 = bytes.Replace(utf8, []byte(`encoding="ISO-8859-1"`), []byte(`encoding="UTF-8"`), 1)
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(utf8); err != nil {
		return nil, err
	}
	doc.Indent(2)
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func firstDiff(want, got []byte) string {
	wl := strings.Split(string(want), "\n")
	gl := strings.Split(string(got), "\n")
	for i := 0; i < len(wl) || i < len(gl); i++ {
		var w, g string
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		if w != g {
			return fmt.Sprintf("line %d: want %q, got %q", i+1, strings.TrimSpace(w), strings.TrimSpace(g))
		}
	}
	return "no line differs"
}

// ---------------------------------------------------------------------------
// XPath helpers
// ---------------------------------------------------------------------------

func evaluate(doc *xmlquery.Node, expr string) (any, error) {
	e, err := xpath.CompileWithNS(expr, specNS)
	if err != nil {
		return nil, fmt.Errorf("bad xpath %q: %w", expr, err)
	}
	return e.Evaluate(xmlquery.CreateXPathNavigator(doc)), nil
}

// nodeValues returns the string value of every node in a node-set result, or
// nil for a scalar result.
func nodeValues(res any) []string {
	it, ok := res.(*xpath.NodeIterator)
	if !ok {
		return nil
	}
	var out []string
	for it.MoveNext() {
		out = append(out, it.Current().Value())
	}
	return out
}

func countOf(res any) int {
	if f, ok := res.(float64); ok {
		return int(f)
	}
	return len(nodeValues(res))
}

func truthy(res any) bool {
	switch v := res.(type) {
	case bool:
		return v
	case float64:
		return v != 0 && !math.IsNaN(v)
	case string:
		return v != ""
	default:
		return len(nodeValues(res)) > 0
	}
}

// singleValue reduces a result to one string, reporting whether it came from
// an XPath number (count(), sum(), arithmetic) rather than from document text.
// A node-set must select exactly one node so an `equals` can never pass by
// accident on the wrong match.
func singleValue(res any) (string, bool, error) {
	switch v := res.(type) {
	case string:
		return v, false, nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true, nil
	case bool:
		return strconv.FormatBool(v), false, nil
	}
	vals := nodeValues(res)
	if len(vals) != 1 {
		return "", false, fmt.Errorf("equals needs exactly one node, found %d", len(vals))
	}
	return vals[0], false, nil
}

// valuesEqual compares document text exactly — "1180.0" in the XML never
// matches "1180.00", because SUNAT validates the format too. XPath numbers are
// compared numerically to the cent, so `sum(...)` can be written as "1180.00".
func valuesEqual(got string, numeric bool, want string) bool {
	if !numeric {
		return got == want
	}
	g, err1 := strconv.ParseFloat(got, 64)
	w, err2 := strconv.ParseFloat(want, 64)
	return err1 == nil && err2 == nil && math.Abs(g-w) < 0.005
}

func firstNode(doc *xmlquery.Node, expr string) (*xmlquery.Node, error) {
	e, err := xpath.CompileWithNS(expr, specNS)
	if err != nil {
		return nil, fmt.Errorf("bad xpath %q: %w", expr, err)
	}
	it, ok := e.Evaluate(xmlquery.CreateXPathNavigator(doc)).(*xpath.NodeIterator)
	if !ok || !it.MoveNext() {
		return nil, fmt.Errorf("%s: no node matches", expr)
	}
	return it.Current().(*xmlquery.NodeNavigator).Current(), nil
}

func docOrder(doc, target *xmlquery.Node) int {
	i := 0
	found := -1
	var walk func(n *xmlquery.Node)
	walk = func(n *xmlquery.Node) {
		if found >= 0 {
			return
		}
		if n == target {
			found = i
			return
		}
		i++
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return found
}

func rootName(doc *xmlquery.Node) string {
	for c := doc.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == xmlquery.ElementNode {
			return c.Data
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Loading
// ---------------------------------------------------------------------------

func loadInvariants(t *testing.T) []invariant {
	t.Helper()
	var file struct {
		Invariants []invariant `yaml:"invariants"`
	}
	readYAML(t, filepath.Join(specRoot, "invariants.yaml"), &file)
	for _, inv := range file.Invariants {
		if inv.ID == "" || inv.Assert == "" || len(inv.AppliesTo) == 0 {
			t.Fatalf("invariants.yaml: every entry needs id, applies_to and assert (got %+v)", inv)
		}
	}
	return file.Invariants
}

func readYAML(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(v); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
}
