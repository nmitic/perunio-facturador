package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"fmt"
	"strings"
)

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
