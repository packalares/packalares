package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"io"
	"strings"
)

const blockSizeBytes = 16

// Encrypt encrypts text with AES-256-GCM using the given secret key.
// Returns base64-encoded ciphertext, IV, and auth tag.
func Encrypt(text, secret string) (ciphertext, iv, tag string, err error) {
	plainText := []byte(text)
	nonce := make([]byte, blockSizeBytes)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", "", "", err
	}

	block, err := aes.NewCipher([]byte(secret))
	if err != nil {
		return "", "", "", err
	}

	aesgcm, err := cipher.NewGCMWithNonceSize(block, blockSizeBytes)
	if err != nil {
		return "", "", "", err
	}

	ret := aesgcm.Seal(nil, nonce, plainText, nil)
	ciphertext = base64.StdEncoding.EncodeToString(ret[:len(plainText)])
	iv = base64.StdEncoding.EncodeToString(nonce)
	tag = base64.StdEncoding.EncodeToString(ret[len(plainText):])

	return
}

// Decrypt decrypts AES-256-GCM encrypted data.
func Decrypt(ciphertext, iv, tag, secret string) (string, error) {
	nonce, err := base64.StdEncoding.DecodeString(iv)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher([]byte(secret))
	if err != nil {
		return "", err
	}

	aesgcm, err := cipher.NewGCMWithNonceSize(block, blockSizeBytes)
	if err != nil {
		return "", err
	}

	cipherIn, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	tagBytes, err := base64.StdEncoding.DecodeString(tag)
	if err != nil {
		return "", err
	}

	cipherIn = append(cipherIn, tagBytes...)

	plaintext, err := aesgcm.Open(nil, nonce, cipherIn, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// PadPasswordTo32 pads or truncates password to 32 bytes for AES-256 key.
func PadPasswordTo32(password string) string {
	if len(password) >= 32 {
		return password[:32]
	}
	return strings.Repeat("0", 32-len(password)) + password
}
