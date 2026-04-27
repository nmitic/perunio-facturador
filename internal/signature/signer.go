package signature

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"

	"github.com/beevik/etree"
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

	var stderr bytes.Buffer
	cmd := exec.Command("xmlsec1", "sign",
		"--privkey-pem", keyFile+","+certFile,
		xmlFile,
	)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("xmlsec1 sign: %w; stderr: %s", err, stderr.String())
	}
	return out, nil
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
func DigestValue(signedXML []byte) (string, error) {
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(signedXML); err != nil {
		return "", fmt.Errorf("parse signed XML: %w", err)
	}

	el := doc.FindElement("//ds:DigestValue")
	if el == nil {
		return "", fmt.Errorf("ds:DigestValue not found")
	}
	return el.Text(), nil
}
