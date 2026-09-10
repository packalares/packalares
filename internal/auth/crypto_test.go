package auth

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"testing"
)

const testSecret = "5f5cd2a1b0e34c6d8a9f0b1c2d3e4f5061728394a5b6c7d8e9f0a1b2c3d4e5f6"

// encryptLegacy reproduces the pre-HKDF scheme: AES-256-GCM under a bare
// SHA-256 of the secret. Used to prove old ciphertexts still decrypt.
func encryptLegacy(t *testing.T, plaintext, secret string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		t.Fatalf("aes cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("gcm: %v", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		t.Fatalf("nonce: %v", err)
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plaintext), nil))
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	for _, plaintext := range []string{"", "JBSWY3DPEHPK3PXP", "unicode ✓ and 'quotes'"} {
		enc, err := encryptAES(plaintext, testSecret)
		if err != nil {
			t.Fatalf("encrypt %q: %v", plaintext, err)
		}
		got, err := decryptAES(enc, testSecret)
		if err != nil {
			t.Fatalf("decrypt %q: %v", plaintext, err)
		}
		if got != plaintext {
			t.Errorf("round trip: got %q, want %q", got, plaintext)
		}
	}
}

// The whole point of the fallback: TOTP secrets enrolled before the HKDF change
// must keep working after it.
func TestDecryptAcceptsLegacyCiphertext(t *testing.T) {
	const plaintext = "JBSWY3DPEHPK3PXP"
	legacy := encryptLegacy(t, plaintext, testSecret)

	got, err := decryptAES(legacy, testSecret)
	if err != nil {
		t.Fatalf("legacy ciphertext failed to decrypt: %v", err)
	}
	if got != plaintext {
		t.Errorf("legacy decrypt: got %q, want %q", got, plaintext)
	}
}

func TestEncryptUsesHKDFNotLegacyKey(t *testing.T) {
	enc, err := encryptAES("JBSWY3DPEHPK3PXP", testSecret)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	data, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	// New ciphertexts must NOT be readable under the old derivation.
	if _, err := openGCM(legacyKey(testSecret), data); err == nil {
		t.Error("new ciphertext decrypted under the legacy key; HKDF is not in effect")
	}
}

func TestDeriveKeyIsDomainSeparated(t *testing.T) {
	key, err := deriveKey(testSecret)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("key length = %d, want 32", len(key))
	}
	// Must differ from the bare hash the session HMAC path would produce from
	// the same master secret.
	if bytes.Equal(key, legacyKey(testSecret)) {
		t.Error("HKDF key equals bare SHA-256 of the secret; no domain separation")
	}
}

func TestDecryptRejectsWrongSecret(t *testing.T) {
	enc, err := encryptAES("JBSWY3DPEHPK3PXP", testSecret)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := decryptAES(enc, testSecret+"tampered"); err == nil {
		t.Error("decrypted under the wrong secret")
	}
}

func TestDecryptRejectsGarbage(t *testing.T) {
	if _, err := decryptAES("!!!not base64!!!", testSecret); err == nil {
		t.Error("accepted non-base64 input")
	}
	if _, err := decryptAES(base64.StdEncoding.EncodeToString([]byte("short")), testSecret); err == nil {
		t.Error("accepted truncated ciphertext")
	}
}
