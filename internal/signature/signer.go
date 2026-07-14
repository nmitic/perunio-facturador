package signature

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/beevik/etree"
	"golang.org/x/text/encoding/charmap"
)

// SignXML takes unsigned ISO-8859-1 XML bytes and signs it using xmlsec1.
// It returns signed XML bytes with ds:Signature filled in.
func SignXML(xmlBytes, privateKeyPEM, certPEM []byte) ([]byte, error) {
	dir := shmDir()

	keyFile, err := writeTmp(dir, "key-*.pem", privateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("write key tmp: %w", err)
	}
	defer os.Remove(keyFile)

	certFile, err := writeTmp(dir, "cert-*.pem", certPEM)
	if err != nil {
		return nil, fmt.Errorf("write cert tmp: %w", err)
	}
	defer os.Remove(certFile)

	xmlFile, err := writeTmp(dir, "doc-*.xml", xmlBytes)
	if err != nil {
		return nil, fmt.Errorf("write xml tmp: %w", err)
	}
	defer os.Remove(xmlFile)

	args := []string{"sign"}
	// xmlsec1 1.3.0 made key search strict: it tries to match the loaded key
	// against the (empty) ds:KeyInfo/X509Data in our signature template and
	// refuses to fall back to the single provided key, failing with
	// KEY-NOT-FOUND. --lax-key-search restores the 1.2.x behavior of using the
	// key we hand it. The flag does not exist before 1.3.0, so it is only added
	// when the installed xmlsec1 supports it.
	if laxKeySearchSupported() {
		args = append(args, "--lax-key-search")
	}
	args = append(args, "--privkey-pem", keyFile+","+certFile, xmlFile)

	var stderr bytes.Buffer
	cmd := exec.Command("xmlsec1", args...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("xmlsec1 sign: %w; stderr: %s", err, stderr.String())
	}
	return out, nil
}

var laxKeySearch struct {
	sync.Once
	supported bool
}

// laxKeySearchSupported reports whether the installed xmlsec1 accepts the
// --lax-key-search flag, i.e. its version is >= 1.3.0. The result is probed
// once (via `xmlsec1 --version`) and cached. On any parsing failure we assume
// unsupported, which keeps the pre-1.3 command line intact.
func laxKeySearchSupported() bool {
	laxKeySearch.Do(func() {
		out, err := exec.Command("xmlsec1", "--version").Output()
		if err != nil {
			return
		}
		// Output looks like: "xmlsec1 1.3.7 (openssl)".
		fields := strings.Fields(string(out))
		if len(fields) < 2 {
			return
		}
		parts := strings.SplitN(fields[1], ".", 3)
		if len(parts) < 2 {
			return
		}
		major, err1 := strconv.Atoi(parts[0])
		minor, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			return
		}
		laxKeySearch.supported = major > 1 || (major == 1 && minor >= 3)
	})
	return laxKeySearch.supported
}

// shmDir returns /dev/shm when available, otherwise os.TempDir().
func shmDir() string {
	if _, err := os.Stat("/dev/shm"); err == nil {
		return "/dev/shm"
	}
	return os.TempDir()
}

// writeTmp writes data to a temp file with 0600 perms and returns the path.
func writeTmp(dir, pattern string, data []byte) (string, error) {
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := f.Chmod(0600); err != nil {
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		return "", err
	}
	return f.Name(), nil
}

// DigestValue extracts the ds:DigestValue from a signed XML document (for QR code).
// SUNAT XML is ISO-8859-1; etree assumes UTF-8, so we register a charset reader.
func DigestValue(signedXML []byte) (string, error) {
	doc := etree.NewDocument()
	doc.ReadSettings.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		if strings.EqualFold(charset, "ISO-8859-1") {
			return charmap.ISO8859_1.NewDecoder().Reader(input), nil
		}
		return nil, fmt.Errorf("unsupported charset: %s", charset)
	}
	if err := doc.ReadFromBytes(signedXML); err != nil {
		return "", fmt.Errorf("parse signed XML: %w", err)
	}

	el := doc.FindElement("//ds:DigestValue")
	if el == nil {
		return "", fmt.Errorf("ds:DigestValue not found")
	}
	return el.Text(), nil
}
