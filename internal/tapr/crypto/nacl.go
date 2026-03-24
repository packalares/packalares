package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"

	"golang.org/x/crypto/nacl/box"
)

const naclNonceSize = 24

// EncryptAsymmetric encrypts plaintext using NaCl box (Curve25519 + XSalsa20-Poly1305).
// publicKey and privateKey are base64-encoded 32-byte keys.
// Returns base64-encoded ciphertext and nonce.
func EncryptAsymmetric(plaintext, publicKey, privateKey string) (ciphertext, nonce string, err error) {
	var nonceBytes [naclNonceSize]byte
	if _, err := io.ReadFull(rand.Reader, nonceBytes[:]); err != nil {
		return "", "", err
	}

	pk, err := base64Decode32(publicKey)
	if err != nil {
		return "", "", err
	}

	sk, err := base64Decode32(privateKey)
	if err != nil {
		return "", "", err
	}

	encrypted := box.Seal(nil, []byte(plaintext), &nonceBytes, &pk, &sk)

	return base64.StdEncoding.EncodeToString(encrypted),
		base64.StdEncoding.EncodeToString(nonceBytes[:]),
		nil
}

// DecryptAsymmetric decrypts NaCl box encrypted data.
func DecryptAsymmetric(ciphertext, nonce, publicKey, privateKey string) (string, error) {
	cipherData, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	nonceData, err := base64.StdEncoding.DecodeString(nonce)
	if err != nil {
		return "", err
	}

	pk, err := base64Decode32(publicKey)
	if err != nil {
		return "", err
	}

	sk, err := base64Decode32(privateKey)
	if err != nil {
		return "", err
	}

	var n [naclNonceSize]byte
	copy(n[:], nonceData)

	plainData, ok := box.Open(nil, cipherData, &n, &pk, &sk)
	if !ok {
		return "", errors.New("nacl box decryption failed")
	}

	return string(plainData), nil
}

func base64Decode32(s string) ([32]byte, error) {
	var out [32]byte
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return out, err
	}
	copy(out[:], data)
	return out, nil
}
