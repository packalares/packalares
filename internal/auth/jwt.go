package auth

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// JWT with HS512 as specified.

type JWTClaims struct {
	Subject   string   `json:"sub"`
	Username  string   `json:"username"`
	Groups    []string `json:"groups,omitempty"`
	AuthLevel int      `json:"auth_level"`
	IssuedAt  int64    `json:"iat"`
	ExpiresAt int64    `json:"exp"`
	Issuer    string   `json:"iss"`
}

const (
	jwtHeaderHS512 = `{"alg":"HS512","typ":"JWT"}`
	jwtDefaultTTL  = 15 * time.Minute
)

func SignJWT(claims *JWTClaims, secret string) (string, error) {
	header := base64URLEncode([]byte(jwtHeaderHS512))

	if claims.IssuedAt == 0 {
		claims.IssuedAt = time.Now().Unix()
	}
	if claims.ExpiresAt == 0 {
		claims.ExpiresAt = time.Now().Add(jwtDefaultTTL).Unix()
	}
	if claims.Issuer == "" {
		claims.Issuer = "packalares-auth"
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}
	encodedPayload := base64URLEncode(payload)

	signingInput := header + "." + encodedPayload
	signature := computeHS512([]byte(signingInput), []byte(secret))

	return signingInput + "." + base64URLEncode(signature), nil
}

func ValidateJWT(tokenStr, secret string) (*JWTClaims, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	signingInput := parts[0] + "." + parts[1]
	expectedSig := computeHS512([]byte(signingInput), []byte(secret))
	actualSig, err := base64URLDecode(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decode signature: %w", err)
	}

	if !hmac.Equal(expectedSig, actualSig) {
		return nil, fmt.Errorf("invalid signature")
	}

	payload, err := base64URLDecode(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}

	var claims JWTClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("unmarshal claims: %w", err)
	}

	if time.Now().Unix() > claims.ExpiresAt {
		return nil, fmt.Errorf("token expired")
	}

	return &claims, nil
}

func computeHS512(message, key []byte) []byte {
	mac := hmac.New(sha512.New, key)
	mac.Write(message)
	return mac.Sum(nil)
}

func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func base64URLDecode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}
