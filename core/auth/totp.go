package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"math"
	"time"
)

const (
	totpPeriod  = 30
	totpDigits  = 6
	totpSkew    = 1
	secretBytes = 20
)

func GenerateTOTPSecret(username, issuer string) (secret string, uri string, err error) {
	b := make([]byte, secretBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate random bytes: %w", err)
	}

	secret = base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)

	uri = fmt.Sprintf(
		"otpauth://totp/%s:%s?secret=%s&issuer=%s&algorithm=SHA1&digits=%d&period=%d",
		issuer, username, secret, issuer, totpDigits, totpPeriod,
	)

	return secret, uri, nil
}

func ValidateTOTPCode(secret, code string) bool {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return false
	}

	now := time.Now().Unix()
	counter := now / totpPeriod

	for i := -totpSkew; i <= totpSkew; i++ {
		expected := generateHOTP(key, uint64(counter+int64(i)))
		if expected == code {
			return true
		}
	}

	return false
}

func generateHOTP(key []byte, counter uint64) string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, counter)

	mac := hmac.New(sha1.New, key)
	mac.Write(buf)
	hash := mac.Sum(nil)

	offset := hash[len(hash)-1] & 0x0f
	code := binary.BigEndian.Uint32(hash[offset:offset+4]) & 0x7fffffff

	otp := code % uint32(math.Pow10(totpDigits))

	return fmt.Sprintf("%0*d", totpDigits, otp)
}
