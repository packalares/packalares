package middleware

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"

	_ "github.com/lib/pq"
)

// PGProvisioner handles PostgreSQL database and user creation.
type PGProvisioner struct {
	host     string
	port     int
	adminUser string
	adminPass string
}

func NewPGProvisioner(host string, port int, adminUser, adminPass string) *PGProvisioner {
	return &PGProvisioner{
		host:      host,
		port:      port,
		adminUser: adminUser,
		adminPass: adminPass,
	}
}

func (p *PGProvisioner) connect(database string) (*sql.DB, error) {
	if database == "" {
		database = "postgres"
	}
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		p.host, p.port, p.adminUser, p.adminPass, database)
	return sql.Open("postgres", dsn)
}

// CreateOrUpdateUser creates a PostgreSQL user or updates its password.
func (p *PGProvisioner) CreateOrUpdateUser(ctx context.Context, username, password string) error {
	db, err := p.connect("")
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer db.Close()

	// Check if user exists
	var exists bool
	err = db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)", username).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check user exists: %w", err)
	}

	// Escape password for SQL (use quoting)
	escapedPwd := strings.ReplaceAll(password, "'", "''")

	if exists {
		_, err = db.ExecContext(ctx, fmt.Sprintf("ALTER ROLE %s WITH PASSWORD '%s'", quoteIdent(username), escapedPwd))
		if err != nil {
			return fmt.Errorf("alter user password: %w", err)
		}
		log.Printf("updated password for PostgreSQL user %q", username)
	} else {
		_, err = db.ExecContext(ctx, fmt.Sprintf("CREATE ROLE %s WITH LOGIN PASSWORD '%s'", quoteIdent(username), escapedPwd))
		if err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		log.Printf("created PostgreSQL user %q", username)
	}

	return nil
}

// CreateDatabase creates a database owned by the given user.
func (p *PGProvisioner) CreateDatabase(ctx context.Context, appNamespace, dbName, owner string) error {
	db, err := p.connect("")
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer db.Close()

	realName := GetDatabaseName(appNamespace, dbName)

	// Check if database exists
	var exists bool
	err = db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", realName).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check database exists: %w", err)
	}

	if exists {
		log.Printf("PostgreSQL database %q already exists", realName)
		return nil
	}

	_, err = db.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s OWNER %s", quoteIdent(realName), quoteIdent(owner)))
	if err != nil {
		return fmt.Errorf("create database %q: %w", realName, err)
	}

	log.Printf("created PostgreSQL database %q owned by %q", realName, owner)
	return nil
}

// CreateExtensions creates PostgreSQL extensions in a specific database.
func (p *PGProvisioner) CreateExtensions(ctx context.Context, appNamespace, dbName string, extensions []string) error {
	realName := GetDatabaseName(appNamespace, dbName)
	db, err := p.connect(realName)
	if err != nil {
		return fmt.Errorf("connect to database %q: %w", realName, err)
	}
	defer db.Close()

	for _, ext := range extensions {
		_, err = db.ExecContext(ctx, fmt.Sprintf("CREATE EXTENSION IF NOT EXISTS %s", quoteIdent(ext)))
		if err != nil {
			return fmt.Errorf("create extension %q in %q: %w", ext, realName, err)
		}
		log.Printf("created extension %q in database %q", ext, realName)
	}

	return nil
}

// DropDatabase drops a database.
func (p *PGProvisioner) DropDatabase(ctx context.Context, appNamespace, dbName string) error {
	db, err := p.connect("")
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer db.Close()

	realName := GetDatabaseName(appNamespace, dbName)

	// Terminate existing connections
	_, _ = db.ExecContext(ctx, fmt.Sprintf(
		"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid()",
		strings.ReplaceAll(realName, "'", "''"),
	))

	_, err = db.ExecContext(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", quoteIdent(realName)))
	if err != nil {
		return fmt.Errorf("drop database %q: %w", realName, err)
	}

	log.Printf("dropped PostgreSQL database %q", realName)
	return nil
}

// DropUser drops a PostgreSQL user.
func (p *PGProvisioner) DropUser(ctx context.Context, username string) error {
	db, err := p.connect("")
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer db.Close()

	_, err = db.ExecContext(ctx, fmt.Sprintf("DROP ROLE IF EXISTS %s", quoteIdent(username)))
	if err != nil {
		return fmt.Errorf("drop user %q: %w", username, err)
	}

	log.Printf("dropped PostgreSQL user %q", username)
	return nil
}

// ListDatabasesByOwner returns all databases owned by the given user.
func (p *PGProvisioner) ListDatabasesByOwner(ctx context.Context, owner string) ([]string, error) {
	db, err := p.connect("")
	if err != nil {
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx,
		"SELECT datname FROM pg_database d JOIN pg_roles r ON d.datdba = r.oid WHERE r.rolname = $1",
		owner,
	)
	if err != nil {
		return nil, fmt.Errorf("list databases by owner: %w", err)
	}
	defer rows.Close()

	var dbs []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		dbs = append(dbs, name)
	}

	return dbs, rows.Err()
}

// GetDatabaseName returns the real database name used in PostgreSQL.
// Compatible with Olares naming: {namespace}_{dbname}
func GetDatabaseName(appNamespace, dbName string) string {
	// Replace dashes with underscores for valid PG identifiers
	ns := strings.ReplaceAll(appNamespace, "-", "_")
	name := strings.ReplaceAll(dbName, "-", "_")
	return ns + "_" + name
}

func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
