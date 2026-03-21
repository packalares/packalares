package auth

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/packalares/packalares/core/db"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	IsAdmin      bool      `json:"is_admin"`
	TOTPSecret   string    `json:"-"`
	TOTPEnabled  bool      `json:"totp_enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

var (
	ErrUserNotFound    = errors.New("user not found")
	ErrUserExists      = errors.New("username already taken")
	ErrInvalidPassword = errors.New("invalid password")
	ErrSetupComplete   = errors.New("initial setup already completed")
)

func CreateUser(username, password string, isAdmin bool) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	d := db.Get()
	adminInt := 0
	if isAdmin {
		adminInt = 1
	}
	result, err := d.Exec(
		"INSERT INTO users (username, password_hash, is_admin) VALUES (?, ?, ?)",
		username, string(hash), adminInt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, ErrUserExists
		}
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &User{
		ID:       id,
		Username: username,
		IsAdmin:  isAdmin,
	}, nil
}

func GetUserByUsername(username string) (*User, error) {
	d := db.Get()
	row := d.QueryRow(
		"SELECT id, username, password_hash, is_admin, totp_secret, totp_enabled, created_at, updated_at FROM users WHERE username = ?",
		username,
	)

	var u User
	var isAdmin, totpEnabled int
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &isAdmin, &u.TOTPSecret, &totpEnabled, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	u.IsAdmin = isAdmin == 1
	u.TOTPEnabled = totpEnabled == 1
	return &u, nil
}

func GetUserByID(id int64) (*User, error) {
	d := db.Get()
	row := d.QueryRow(
		"SELECT id, username, password_hash, is_admin, totp_secret, totp_enabled, created_at, updated_at FROM users WHERE id = ?",
		id,
	)

	var u User
	var isAdmin, totpEnabled int
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &isAdmin, &u.TOTPSecret, &totpEnabled, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	u.IsAdmin = isAdmin == 1
	u.TOTPEnabled = totpEnabled == 1
	return &u, nil
}

func VerifyPassword(user *User, password string) error {
	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return ErrInvalidPassword
	}
	return nil
}

func UserCount() (int, error) {
	d := db.Get()
	var count int
	err := d.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}

func UpdateTOTPSecret(userID int64, secret string) error {
	d := db.Get()
	_, err := d.Exec("UPDATE users SET totp_secret = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", secret, userID)
	return err
}

func EnableTOTP(userID int64) error {
	d := db.Get()
	_, err := d.Exec("UPDATE users SET totp_enabled = 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?", userID)
	return err
}

func DisableTOTP(userID int64) error {
	d := db.Get()
	_, err := d.Exec("UPDATE users SET totp_enabled = 0, totp_secret = '', updated_at = CURRENT_TIMESTAMP WHERE id = ?", userID)
	return err
}
