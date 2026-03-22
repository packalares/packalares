package appservice

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/lib/pq"
	"gopkg.in/yaml.v3"
	"k8s.io/klog/v2"
)

// MiddlewareProvisioner handles pre-provisioning middleware resources (databases,
// redis namespaces, etc.) that Olares charts expect. In the full Olares platform
// this is done by the middleware operator via MiddlewareRequest CRDs, but in
// Packalares we handle it directly since we run a single Citus/PostgreSQL and
// a single Redis instance.
type MiddlewareProvisioner struct {
	pgHost     string
	pgPort     string
	pgUser     string
	pgPassword string
	pgSSLMode  string

	redisHost     string
	redisPort     string
	redisPassword string

	owner  string
	domain string
	zone   string
}

// NewMiddlewareProvisioner creates a provisioner from environment variables.
func NewMiddlewareProvisioner(owner string) *MiddlewareProvisioner {
	p := &MiddlewareProvisioner{
		pgHost:     envOrDefault("PG_HOST", "citus-master-svc.packalares-platform"),
		pgPort:     envOrDefault("PG_PORT", "5432"),
		pgUser:     envOrDefault("PG_USER", "postgres"),
		pgPassword: os.Getenv("PG_PASSWORD"),
		pgSSLMode:  envOrDefault("PG_SSLMODE", "disable"),

		redisHost:     envOrDefault("REDIS_HOST", envOrDefault("NODE_IP", "127.0.0.1")),
		redisPort:     envOrDefault("REDIS_PORT", "6379"),
		redisPassword: os.Getenv("REDIS_PASSWORD"),

		owner:  owner,
		domain: envOrDefault("OLARES_DOMAIN", "olares.local"),
	}

	p.zone = envOrDefault("USER_ZONE", owner+"."+p.domain)

	return p
}

// MiddlewareRequirements describes what middleware an app needs,
// parsed from the OlaresManifest.yaml middleware section.
type MiddlewareRequirements struct {
	Postgres *PostgresRequirement `yaml:"postgres,omitempty"`
	Redis    *RedisRequirement    `yaml:"redis,omitempty"`
	MongoDB  *MongoRequirement    `yaml:"mongodb,omitempty"`
	Nats     *NatsRequirement     `yaml:"nats,omitempty"`
}

// PostgresRequirement describes what PostgreSQL resources an app needs.
type PostgresRequirement struct {
	Username  string              `yaml:"username"`
	Password  string              `yaml:"password,omitempty"`
	Databases []PostgresDatabase  `yaml:"databases"`
}

// PostgresDatabase describes a single database needed by an app.
type PostgresDatabase struct {
	Name        string   `yaml:"name"`
	Distributed bool     `yaml:"distributed,omitempty"`
	Extensions  []string `yaml:"extensions,omitempty"`
	Scripts     []string `yaml:"scripts,omitempty"`
}

// RedisRequirement describes what Redis resources an app needs.
type RedisRequirement struct {
	Namespace string `yaml:"namespace,omitempty"`
}

// MongoRequirement describes what MongoDB resources an app needs.
type MongoRequirement struct {
	Username  string   `yaml:"username,omitempty"`
	Databases []string `yaml:"databases,omitempty"`
}

// NatsRequirement describes what NATS resources an app needs.
type NatsRequirement struct {
	Subjects []string `yaml:"subjects,omitempty"`
}

// ParseMiddlewareFromManifest extracts middleware requirements from a parsed
// OlaresManifest. The manifest YAML has a top-level "middleware" key.
func ParseMiddlewareFromManifest(chartDir string) (*MiddlewareRequirements, error) {
	candidates := []string{
		filepath.Join(chartDir, "OlaresManifest.yaml"),
		filepath.Join(chartDir, "TerminusManifest.yaml"),
	}

	var data []byte
	var readErr error
	for _, path := range candidates {
		data, readErr = os.ReadFile(path)
		if readErr == nil {
			break
		}
	}
	if readErr != nil {
		return nil, fmt.Errorf("no manifest file found in %s", chartDir)
	}

	// Parse the raw YAML to get the middleware section
	var raw struct {
		Middleware *MiddlewareRequirements `yaml:"middleware"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse manifest middleware: %w", err)
	}

	return raw.Middleware, nil
}

// chartValues is a minimal representation of a chart's values.yaml to detect
// what middleware-related values it expects.
type chartValues struct {
	Postgres *chartPostgres `yaml:"postgres,omitempty"`
	Redis    *chartRedis    `yaml:"redis,omitempty"`
	Mongo    *chartMongo    `yaml:"mongo,omitempty"`
	Nats     *chartNats     `yaml:"nats,omitempty"`
}

type chartPostgres struct {
	Host      string                 `yaml:"host"`
	Port      interface{}            `yaml:"port"` // can be int or string
	Username  string                 `yaml:"username"`
	Password  string                 `yaml:"password"`
	Databases map[string]interface{} `yaml:"databases,omitempty"`
}

type chartRedis struct {
	Host      string `yaml:"host"`
	Port      interface{} `yaml:"port"`
	Password  string `yaml:"password"`
	Namespace string `yaml:"namespace,omitempty"`
}

type chartMongo struct {
	Host     string      `yaml:"host"`
	Port     interface{} `yaml:"port"`
	Username string      `yaml:"username"`
	Password string      `yaml:"password"`
}

type chartNats struct {
	URL string `yaml:"url"`
}

// parseChartValues reads the chart's values.yaml to understand what values are
// expected (postgres, redis, etc.)
func parseChartValues(chartDir string) (*chartValues, error) {
	data, err := os.ReadFile(filepath.Join(chartDir, "values.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return &chartValues{}, nil
		}
		return nil, fmt.Errorf("read values.yaml: %w", err)
	}

	var vals chartValues
	if err := yaml.Unmarshal(data, &vals); err != nil {
		return nil, fmt.Errorf("parse values.yaml: %w", err)
	}

	return &vals, nil
}

// ProvisionAndBuildValues is the main entry point. It:
//  1. Parses the OlaresManifest to find middleware requirements
//  2. Parses the chart's values.yaml to see what values are referenced
//  3. Pre-provisions PostgreSQL databases as needed
//  4. Returns a map of helm --set values for all middleware fields
func (p *MiddlewareProvisioner) ProvisionAndBuildValues(ctx context.Context, chartDir, appName string) (map[string]string, error) {
	vals := make(map[string]string)

	// Always inject standard Olares platform values
	vals["bfl.username"] = p.owner
	vals["user.zone"] = p.zone
	vals["domain"] = p.domain
	vals["namespace"] = "user-space-" + p.owner
	vals["userspace.appData"] = envOrDefault("USERSPACE_APPDATA", "/terminus/userdata/appdata")
	vals["userspace.appCache"] = envOrDefault("USERSPACE_APPCACHE", "/terminus/userdata/appcache")

	// Parse the OlaresManifest middleware section
	mwReqs, err := ParseMiddlewareFromManifest(chartDir)
	if err != nil {
		klog.V(2).Infof("no middleware requirements for %s: %v", appName, err)
	}

	// Also check the chart's values.yaml to see if it has postgres/redis sections
	chartVals, err := parseChartValues(chartDir)
	if err != nil {
		klog.V(2).Infof("could not parse values.yaml for %s: %v", appName, err)
	}

	// Determine if app needs PostgreSQL
	needsPostgres := (mwReqs != nil && mwReqs.Postgres != nil) || (chartVals != nil && chartVals.Postgres != nil)
	needsRedis := (mwReqs != nil && mwReqs.Redis != nil) || (chartVals != nil && chartVals.Redis != nil)

	if needsPostgres {
		pgVals, provErr := p.provisionPostgres(ctx, appName, mwReqs, chartVals)
		if provErr != nil {
			return nil, fmt.Errorf("provision postgres for %s: %w", appName, provErr)
		}
		for k, v := range pgVals {
			vals[k] = v
		}
	}

	if needsRedis {
		redisVals := p.buildRedisValues(mwReqs)
		for k, v := range redisVals {
			vals[k] = v
		}
	}

	return vals, nil
}

// provisionPostgres creates databases and returns helm values for PostgreSQL.
func (p *MiddlewareProvisioner) provisionPostgres(ctx context.Context, appName string, mwReqs *MiddlewareRequirements, chartVals *chartValues) (map[string]string, error) {
	vals := make(map[string]string)

	// Set the connection values
	vals["postgres.host"] = p.pgHost
	vals["postgres.port"] = p.pgPort
	vals["postgres.password"] = p.pgPassword

	// Determine the username
	pgUsername := p.pgUser
	if mwReqs != nil && mwReqs.Postgres != nil && mwReqs.Postgres.Username != "" {
		pgUsername = mwReqs.Postgres.Username
	}
	vals["postgres.username"] = pgUsername

	// Determine which databases to create
	var databasesToCreate []PostgresDatabase

	if mwReqs != nil && mwReqs.Postgres != nil && len(mwReqs.Postgres.Databases) > 0 {
		// Use the manifest's database list
		databasesToCreate = mwReqs.Postgres.Databases
	} else if chartVals != nil && chartVals.Postgres != nil && len(chartVals.Postgres.Databases) > 0 {
		// Fall back to values.yaml database entries
		for dbName := range chartVals.Postgres.Databases {
			databasesToCreate = append(databasesToCreate, PostgresDatabase{Name: dbName})
		}
	} else {
		// Default: create a database named after the app
		databasesToCreate = []PostgresDatabase{{Name: appName}}
	}

	// Create databases and set the helm values
	for _, db := range databasesToCreate {
		dbName := db.Name
		// Prefix with owner to avoid collisions in multi-user setups
		qualifiedDBName := fmt.Sprintf("user_%s_%s", sanitizeDBName(p.owner), sanitizeDBName(dbName))

		if err := p.ensureDatabase(ctx, qualifiedDBName, pgUsername, db.Extensions); err != nil {
			return nil, fmt.Errorf("create database %s: %w", qualifiedDBName, err)
		}

		// Run any post-creation scripts
		if len(db.Scripts) > 0 {
			if err := p.runDatabaseScripts(ctx, qualifiedDBName, pgUsername, db.Scripts); err != nil {
				klog.Warningf("run scripts for database %s: %v (continuing)", qualifiedDBName, err)
			}
		}

		// Set the helm value: postgres.databases.<name> = <qualifiedDBName>
		vals[fmt.Sprintf("postgres.databases.%s", dbName)] = qualifiedDBName
	}

	return vals, nil
}

// ensureDatabase creates a PostgreSQL database if it does not already exist,
// and creates the user role if needed.
func (p *MiddlewareProvisioner) ensureDatabase(ctx context.Context, dbName, username string, extensions []string) error {
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=postgres sslmode=%s",
		p.pgHost, p.pgPort, p.pgUser, p.pgPassword, p.pgSSLMode)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}

	// Ensure the role exists (for non-superuser app accounts)
	if username != p.pgUser {
		if err := p.ensureRole(ctx, db, username); err != nil {
			return fmt.Errorf("ensure role %s: %w", username, err)
		}
	}

	// Check if database exists
	var exists bool
	err = db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", dbName).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check database existence: %w", err)
	}

	if !exists {
		// CREATE DATABASE cannot be parameterized, sanitize the name
		_, err = db.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s OWNER %s",
			quoteIdentifier(dbName), quoteIdentifier(username)))
		if err != nil {
			return fmt.Errorf("create database %s: %w", dbName, err)
		}
		klog.Infof("created database %s owned by %s", dbName, username)
	} else {
		klog.V(2).Infof("database %s already exists", dbName)
	}

	// Install extensions if requested
	if len(extensions) > 0 {
		if err := p.installExtensions(ctx, dbName, extensions); err != nil {
			klog.Warningf("install extensions in %s: %v (continuing)", dbName, err)
		}
	}

	return nil
}

// ensureRole creates a PostgreSQL role if it does not exist.
func (p *MiddlewareProvisioner) ensureRole(ctx context.Context, db *sql.DB, username string) error {
	var exists bool
	err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)", username).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check role existence: %w", err)
	}

	if !exists {
		// Create role with login privilege, using the global PG password
		_, err = db.ExecContext(ctx, fmt.Sprintf("CREATE ROLE %s LOGIN PASSWORD %s",
			quoteIdentifier(username), quoteLiteral(p.pgPassword)))
		if err != nil {
			return fmt.Errorf("create role %s: %w", username, err)
		}
		klog.Infof("created PostgreSQL role %s", username)
	}

	return nil
}

// installExtensions installs PostgreSQL extensions in a specific database.
func (p *MiddlewareProvisioner) installExtensions(ctx context.Context, dbName string, extensions []string) error {
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		p.pgHost, p.pgPort, p.pgUser, p.pgPassword, dbName, p.pgSSLMode)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", dbName, err)
	}
	defer db.Close()

	for _, ext := range extensions {
		_, err := db.ExecContext(ctx, fmt.Sprintf("CREATE EXTENSION IF NOT EXISTS %s", quoteIdentifier(ext)))
		if err != nil {
			klog.Warningf("install extension %s in %s: %v", ext, dbName, err)
			// Don't fail -- the extension might not be available but the app may still work
		} else {
			klog.V(2).Infof("installed extension %s in %s", ext, dbName)
		}
	}

	return nil
}

// runDatabaseScripts runs post-creation SQL scripts against a specific database.
func (p *MiddlewareProvisioner) runDatabaseScripts(ctx context.Context, dbName, username string, scripts []string) error {
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		p.pgHost, p.pgPort, p.pgUser, p.pgPassword, dbName, p.pgSSLMode)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("connect to %s for scripts: %w", dbName, err)
	}
	defer db.Close()

	// Join all scripts and replace $dbusername placeholder
	fullScript := strings.Join(scripts, "\n")
	fullScript = strings.ReplaceAll(fullScript, "$dbusername", username)

	_, err = db.ExecContext(ctx, fullScript)
	if err != nil {
		return fmt.Errorf("run scripts in %s: %w", dbName, err)
	}

	klog.V(2).Infof("ran %d scripts in database %s", len(scripts), dbName)
	return nil
}

// buildRedisValues returns helm --set values for Redis configuration.
func (p *MiddlewareProvisioner) buildRedisValues(mwReqs *MiddlewareRequirements) map[string]string {
	vals := make(map[string]string)

	vals["redis.host"] = p.redisHost
	vals["redis.port"] = p.redisPort
	vals["redis.password"] = p.redisPassword

	if mwReqs != nil && mwReqs.Redis != nil && mwReqs.Redis.Namespace != "" {
		vals["redis.namespace"] = mwReqs.Redis.Namespace
	}

	return vals
}

// sanitizeDBName replaces characters that are not safe for database names.
func sanitizeDBName(name string) string {
	replacer := strings.NewReplacer(
		"-", "_",
		".", "_",
		" ", "_",
	)
	return strings.ToLower(replacer.Replace(name))
}

// quoteIdentifier quotes a PostgreSQL identifier (table name, database name, etc.)
// to prevent SQL injection. This is the standard "" quoting.
func quoteIdentifier(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// quoteLiteral quotes a PostgreSQL string literal to prevent SQL injection.
func quoteLiteral(s string) string {
	return `'` + strings.ReplaceAll(s, `'`, `''`) + `'`
}

// envOrDefault returns the value of an environment variable, or a default if not set.
func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
