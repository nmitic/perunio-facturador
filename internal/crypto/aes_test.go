package crypto_test

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"testing"

	"maragu.dev/is"

	facturadorCrypto "github.com/perunio/perunio-facturador/internal/crypto"
)

func encryptForTest(t *testing.T, plaintext string, key []byte) string {
	t.Helper()

	block, err := aes.NewCipher(key)
	is.NotError(t, err)

	gcm, err := cipher.NewGCM(block)
	is.NotError(t, err)

	iv := make([]byte, gcm.NonceSize())
	_, err = rand.Read(iv)
	is.NotError(t, err)

	sealed := gcm.Seal(nil, iv, []byte(plaintext), nil)

	// Split: ciphertext is everything except the last 16 bytes (authTag)
	tagSize := gcm.Overhead()
	ciphertext := sealed[:len(sealed)-tagSize]
	authTag := sealed[len(sealed)-tagSize:]

	return hex.EncodeToString(iv) + ":" + hex.EncodeToString(authTag) + ":" + hex.EncodeToString(ciphertext)
}

func TestDecryptAES256GCM(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	is.NotError(t, err)

	t.Run("should decrypt a value encrypted in the backend format", func(t *testing.T) {
		original := "my-secret-password"
		encrypted := encryptForTest(t, original, key)

		decrypted, err := facturadorCrypto.DecryptAES256GCM(encrypted, key)
		is.NotError(t, err)
		is.Equal(t, original, decrypted)
	})

	t.Run("should fail with wrong key", func(t *testing.T) {
		encrypted := encryptForTest(t, "test", key)

		wrongKey := make([]byte, 32)
		_, err := rand.Read(wrongKey)
		is.NotError(t, err)

		_, err = facturadorCrypto.DecryptAES256GCM(encrypted, wrongKey)
		is.True(t, err != nil, "should fail with wrong key")
	})

	t.Run("should fail with invalid format", func(t *testing.T) {
		_, err := facturadorCrypto.DecryptAES256GCM("invalid", key)
		is.True(t, err != nil, "should fail with invalid format")
	})
}
