package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

// totpKeyInfo domain-separates the TOTP encryption key from the other uses of
// SESSION_SECRET (notably the session-ID HMAC in SignSessionID), so the same
// master secret never produces the same key for two different purposes.
const totpKeyInfo = "packalares:auth:totp-encryption:v1"

// deriveKey turns the master secret into a 32-byte AES key with HKDF-SHA256.
//
// The secret is a 256-bit value from crypto/rand (SESSION_SECRET, generated as
// generateSecret(32)), not a human-chosen password, so a deliberately slow
// password hash such as Argon2id would buy nothing here — there is no
// low-entropy input to brute-force. HKDF is the right primitive for this shape
// of input: it is an extract-and-expand KDF for high-entropy key material, and
// it gives us domain separation, which a bare hash does not.
func deriveKey(secret string) ([]byte, error) {
	key, err := hkdf.Key(sha256.New, []byte(secret), nil, totpKeyInfo, 32)
	if err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}
	return key, nil
}

// legacyKey reproduces the original key derivation: a bare, unsalted SHA-256 of
// the secret. It exists only so ciphertexts written before the move to HKDF
// remain readable; nothing encrypts with it any more.
func legacyKey(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

// encryptAES encrypts plaintext using AES-256-GCM under a key derived from the
// master secret.
func encryptAES(plaintext, secret string) (string, error) {
	key, err := deriveKey(secret)
	if err != nil {
		return "", err
	}

	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptAES decrypts base64-encoded ciphertext using AES-256-GCM. It tries the
// current HKDF-derived key first and falls back to the legacy SHA-256 key, so
// secrets stored by an earlier build still decrypt. A value that falls back is
// re-encrypted under the current key the next time it is written.
func decryptAES(encrypted, secret string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}

	key, err := deriveKey(secret)
	if err != nil {
		return "", err
	}

	plaintext, err := openGCM(key, data)
	if err == nil {
		return plaintext, nil
	}

	// GCM authentication failed under the current key — the value may predate
	// the HKDF change.
	if plaintext, legacyErr := openGCM(legacyKey(secret), data); legacyErr == nil {
		return plaintext, nil
	}

	return "", err
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("aes gcm: %w", err)
	}
	return gcm, nil
}

func openGCM(key, data []byte) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("aes decrypt: %w", err)
	}
	return string(plaintext), nil
}
