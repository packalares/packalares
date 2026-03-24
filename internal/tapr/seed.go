package tapr

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/packalares/packalares/internal/tapr/crypto"
	"golang.org/x/crypto/nacl/box"
	_ "github.com/lib/pq"
)

// SeedConfig holds the parameters needed to seed Infisical.
type SeedConfig struct {
	PGDSN       string // PostgreSQL connection string for infisical DB
	Email       string // admin email
	Username    string // admin username
	Password    string // admin password (used to encrypt private key)
	OrgName     string // organization name
	ProjectName string // project name for storing system secrets
}

// SeedResult holds the output of seeding.
type SeedResult struct {
	UserID      string
	OrgID       string
	ProjectID   string
	PublicKey   string // base64 NaCl public key
	PrivateKey  string // base64 NaCl private key (plaintext, for runtime use)
	JWTSecret   string // JWT signing secret from infisical-backend
}

// Seed creates the user, org, project, and encryption keys in Infisical's PostgreSQL.
// This bypasses the SRP login flow by writing directly to the database.
func Seed(ctx context.Context, cfg SeedConfig) (*SeedResult, error) {
	db, err := sql.Open("postgres", cfg.PGDSN)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	// Wait for DB
	for i := 0; i < 30; i++ {
		if err := db.PingContext(ctx); err == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}

	// Generate NaCl key pair
	publicKeyBytes, privateKeyBytes, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate nacl keys: %w", err)
	}

	publicKey := base64.StdEncoding.EncodeToString(publicKeyBytes[:])
	privateKey := base64.StdEncoding.EncodeToString(privateKeyBytes[:])

	// Encrypt private key with password (AES-256-GCM)
	paddedPassword := crypto.PadPasswordTo32(cfg.Password)
	encPrivKey, encIV, encTag, err := crypto.Encrypt(privateKey, paddedPassword)
	if err != nil {
		return nil, fmt.Errorf("encrypt private key: %w", err)
	}

	// Generate SRP salt and verifier
	// For simplicity, we generate a random salt and a dummy verifier.
	// The SRP verifier is only needed for Infisical's web login which we don't use.
	// We access secrets through our own JWT + API, not through SRP login.
	salt := hex.EncodeToString(randomBytes(32))
	verifier := hex.EncodeToString(randomBytes(128))

	userID := uuid.New().String()
	orgID := ""
	now := time.Now()

	// Check if org exists
	err = db.QueryRowContext(ctx, `SELECT id FROM organizations LIMIT 1`).Scan(&orgID)
	if err != nil {
		orgID = uuid.New().String()
		_, err = db.ExecContext(ctx,
			`INSERT INTO organizations (id, name, slug, "createdAt", "updatedAt")
			 VALUES ($1, $2, $3, $4, $5)`,
			orgID, cfg.OrgName, slugify(cfg.OrgName), now, now)
		if err != nil {
			return nil, fmt.Errorf("create org: %w", err)
		}
	}

	// Check if user exists
	existingUserID := ""
	err = db.QueryRowContext(ctx, `SELECT id FROM users WHERE email = $1`, cfg.Email).Scan(&existingUserID)
	if err == nil {
		userID = existingUserID
	} else {
		_, err = db.ExecContext(ctx,
			`INSERT INTO users (id, email, "firstName", "lastName", "isAccepted", username, "superAdmin", "createdAt", "updatedAt")
			 VALUES ($1, $2, $3, $4, true, $5, true, $6, $7)`,
			userID, cfg.Email, cfg.Username, "Admin", cfg.Username, now, now)
		if err != nil {
			return nil, fmt.Errorf("create user: %w", err)
		}
	}

	// Check if encryption keys exist for this user
	var existingKeyID string
	err = db.QueryRowContext(ctx, `SELECT id FROM user_encryption_keys WHERE "userId" = $1`, userID).Scan(&existingKeyID)
	if err != nil {
		_, err = db.ExecContext(ctx,
			`INSERT INTO user_encryption_keys (id, "userId", "publicKey", "encryptedPrivateKey", iv, tag, salt, verifier, "encryptionVersion")
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 1)`,
			uuid.New().String(), userID, publicKey, encPrivKey, encIV, encTag, salt, verifier)
	} else {
		_, err = db.ExecContext(ctx,
			`UPDATE user_encryption_keys SET "publicKey" = $1, "encryptedPrivateKey" = $2, iv = $3, tag = $4, salt = $5, verifier = $6 WHERE "userId" = $7`,
			publicKey, encPrivKey, encIV, encTag, salt, verifier, userID)
	}
	if err != nil {
		return nil, fmt.Errorf("create encryption keys: %w", err)
	}

	// Check if org membership exists
	var existingMemberID string
	err = db.QueryRowContext(ctx, `SELECT id FROM memberships WHERE "actorUserId" = $1 AND "scopeOrgId" = $2`, userID, orgID).Scan(&existingMemberID)
	if err != nil {
		_, err = db.ExecContext(ctx,
			`INSERT INTO memberships (id, scope, "actorUserId", "scopeOrgId", "isActive", status, "createdAt", "updatedAt")
			 VALUES ($1, 'organization', $2, $3, true, 'accepted', $4, $5)`,
			uuid.New().String(), userID, orgID, now, now)
		if err != nil {
			// Ignore — schema might differ
			fmt.Printf("tapr: warning: create membership: %v\n", err)
		}
	}

	// Create membership role if membership exists
	var membershipID string
	err = db.QueryRowContext(ctx,
		`SELECT id FROM memberships WHERE "actorUserId" = $1 AND "scopeOrgId" = $2`, userID, orgID).Scan(&membershipID)
	if err == nil {
		var existingRoleID string
		err = db.QueryRowContext(ctx, `SELECT id FROM membership_roles WHERE "membershipId" = $1`, membershipID).Scan(&existingRoleID)
		if err != nil {
			db.ExecContext(ctx,
				`INSERT INTO membership_roles (id, role, "isTemporary", "membershipId", "createdAt", "updatedAt")
				 VALUES ($1, 'admin', false, $2, $3, $4)`,
				uuid.New().String(), membershipID, now, now)
		}
	}

	// Create auth token session if not exists
	var existingSessionID string
	err = db.QueryRowContext(ctx, `SELECT id FROM auth_token_sessions WHERE "userId" = $1 LIMIT 1`, userID).Scan(&existingSessionID)
	if err != nil {
		_, err = db.ExecContext(ctx,
			`INSERT INTO auth_token_sessions (id, "userId", ip, "userAgent", "accessVersion", "refreshVersion", "lastUsed", "createdAt", "updatedAt")
			 VALUES ($1, $2, '127.0.0.1', 'packalares-tapr', 1, 1, $3, $4, $5)`,
			uuid.New().String(), userID, now, now, now)
		if err != nil {
			return nil, fmt.Errorf("create session: %w", err)
		}
	}

	return &SeedResult{
		UserID:     userID,
		OrgID:      orgID,
		PublicKey:  publicKey,
		PrivateKey: privateKey,
	}, nil
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	io.ReadFull(rand.Reader, b)
	return b
}

func slugify(name string) string {
	s := ""
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			s += string(c)
		} else if c >= 'A' && c <= 'Z' {
			s += string(c + 32)
		} else if c == ' ' {
			s += "-"
		}
	}
	return s
}
