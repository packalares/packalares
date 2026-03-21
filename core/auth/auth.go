package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/packalares/packalares/core/db"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrTokenExpired = errors.New("token expired")
)

type Claims struct {
	UserID   int64  `json:"sub"`
	Username string `json:"username"`
	IsAdmin  bool   `json:"admin"`
	Exp      int64  `json:"exp"`
	Iat      int64  `json:"iat"`
}

type TokenPair struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

const (
	accessTokenDuration  = 15 * time.Minute
	refreshTokenDuration = 7 * 24 * time.Hour
)

func GenerateTokenPair(user *User, secret string) (*TokenPair, error) {
	now := time.Now()

	claims := Claims{
		UserID:   user.ID,
		Username: user.Username,
		IsAdmin:  user.IsAdmin,
		Exp:      now.Add(accessTokenDuration).Unix(),
		Iat:      now.Unix(),
	}

	token, err := signJWT(claims, secret)
	if err != nil {
		return nil, fmt.Errorf("sign access token: %w", err)
	}

	refreshClaims := Claims{
		UserID:   user.ID,
		Username: user.Username,
		IsAdmin:  user.IsAdmin,
		Exp:      now.Add(refreshTokenDuration).Unix(),
		Iat:      now.Unix(),
	}

	refreshToken, err := signJWT(refreshClaims, secret+"_refresh")
	if err != nil {
		return nil, fmt.Errorf("sign refresh token: %w", err)
	}

	d := db.Get()
	_, err = d.Exec(
		"INSERT INTO refresh_tokens (user_id, token, expires_at) VALUES (?, ?, ?)",
		user.ID, refreshToken, now.Add(refreshTokenDuration),
	)
	if err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}

	return &TokenPair{
		Token:        token,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(accessTokenDuration.Seconds()),
	}, nil
}

func ValidateAccessToken(tokenStr, secret string) (*Claims, error) {
	return validateJWT(tokenStr, secret)
}

func RefreshAccessToken(refreshTokenStr, secret string) (*TokenPair, error) {
	claims, err := validateJWT(refreshTokenStr, secret+"_refresh")
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}

	d := db.Get()
	var id int64
	err = d.QueryRow("SELECT id FROM refresh_tokens WHERE token = ? AND expires_at > CURRENT_TIMESTAMP", refreshTokenStr).Scan(&id)
	if err != nil {
		return nil, ErrInvalidToken
	}

	_, _ = d.Exec("DELETE FROM refresh_tokens WHERE id = ?", id)
	_, _ = d.Exec("DELETE FROM refresh_tokens WHERE user_id = ? AND expires_at <= CURRENT_TIMESTAMP", claims.UserID)

	user, err := GetUserByID(claims.UserID)
	if err != nil {
		return nil, err
	}

	return GenerateTokenPair(user, secret)
}

func signJWT(claims Claims, secret string) (string, error) {
	header := base64URLEncode([]byte(`{"alg":"HS256","typ":"JWT"}`))

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encodedPayload := base64URLEncode(payload)

	signingInput := header + "." + encodedPayload
	signature := computeHMAC([]byte(signingInput), []byte(secret))

	return signingInput + "." + base64URLEncode(signature), nil
}

func validateJWT(tokenStr, secret string) (*Claims, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	signingInput := parts[0] + "." + parts[1]
	expectedSig := computeHMAC([]byte(signingInput), []byte(secret))
	actualSig, err := base64URLDecode(parts[2])
	if err != nil {
		return nil, ErrInvalidToken
	}

	if !hmac.Equal(expectedSig, actualSig) {
		return nil, ErrInvalidToken
	}

	payload, err := base64URLDecode(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}

	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, ErrInvalidToken
	}

	if time.Now().Unix() > claims.Exp {
		return nil, ErrTokenExpired
	}

	return &claims, nil
}

func computeHMAC(message, key []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(message)
	return mac.Sum(nil)
}

func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func base64URLDecode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}
