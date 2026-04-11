package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

// DecodeKeyHex decodes a 64-char hex string into a 32-byte AES-256 key. Use
// this once at startup to convert the awssecrets-provided key into raw bytes.
func DecodeKeyHex(keyHex string) ([]byte, error) {
	keyBytes, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("encryption key must be valid hex: %w", err)
	}
	if len(keyBytes) != 32 {
		return nil, fmt.Errorf("encryption key must be 64 hex chars (32 bytes), got %d bytes", len(keyBytes))
	}
	return keyBytes, nil
}

// DecryptAES256GCM decrypts a value encrypted by the Node.js backend.
// The encrypted format is "{iv}:{authTag}:{ciphertext}" where each part is hex-encoded.
// The key must be 32 bytes (AES-256).
func DecryptAES256GCM(encrypted string, key []byte) (string, error) {
	parts := strings.SplitN(encrypted, ":", 3)
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid encrypted format: expected iv:authTag:ciphertext")
	}

	iv, err := hex.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("decode IV: %w", err)
	}

	authTag, err := hex.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode auth tag: %w", err)
	}

	ciphertext, err := hex.DecodeString(parts[2])
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	// GCM expects ciphertext + authTag concatenated
	combined := append(ciphertext, authTag...)

	plaintext, err := gcm.Open(nil, iv, combined, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}

// EncryptAES256GCM encrypts plaintext into the same "iv:authTag:ciphertext"
// hex format that the Node.js backend produces (see
// perunio-backend/src/services/sunat.service.ts). The key must be 32 bytes.
func EncryptAES256GCM(plaintext string, key []byte) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("encryption key must be 32 bytes, got %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	iv := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(iv); err != nil {
		return "", fmt.Errorf("generate IV: %w", err)
	}

	// Seal returns ciphertext || authTag concatenated. Split them apart so the
	// on-wire format matches Node.js (which keeps them as separate hex chunks).
	sealed := gcm.Seal(nil, iv, []byte(plaintext), nil)
	tagSize := gcm.Overhead()
	ciphertext := sealed[:len(sealed)-tagSize]
	authTag := sealed[len(sealed)-tagSize:]

	return strings.Join([]string{
		hex.EncodeToString(iv),
		hex.EncodeToString(authTag),
		hex.EncodeToString(ciphertext),
	}, ":"), nil
}
