package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	migrationsDir = "./supabase/migrations"
	schemaName    = "supabase_migrations"
	tableName     = "schema_migrations"
)

type Migration struct {
	Version    string
	Name       string
	Raw        string
	Statements []string
	Hash       string
}

// SHA-256 same as Supabase
func computeHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// Loads local migrations in format {version}_{name}.sql
func loadLocalMigrations() ([]Migration, error) {
	files, err := os.ReadDir(migrationsDir)
	if err != nil {
		return nil, err
	}

	var migrations []Migration

	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".sql") {
			continue
		}

		path := filepath.Join(migrationsDir, f.Name())
		rawBytes, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}

		raw := string(rawBytes)

		parts := strings.SplitN(f.Name(), "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid migration name: %s", f.Name())
		}

		version := parts[0]
		name := parts[1]

		// Split by "-- statement-breakpoint" (Supabase behavior)
		statements := []string{}
		chunks := strings.Split(raw, "-- statement-breakpoint")
		for _, c := range chunks {
			stmt := strings.TrimSpace(c)
			if stmt != "" {
				statements = append(statements, stmt)
			}
		}

		migrations = append(migrations, Migration{
			Version:    version,
			Name:       name,
			Raw:        raw,
			Statements: statements,
			Hash:       computeHash(raw),
		})
	}

	// Sort by version (timestamp)
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

func printHelp() {
	fmt.Println("Supabase Direct Migrate")
	fmt.Println("Apply Supabase migrations directly to PostgreSQL database")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  supabase-direct-migrate [flags]")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -h, --help    Show this help message")
	fmt.Println()
	fmt.Println("Environment Variables:")
	fmt.Println("  DATABASE_URL    PostgreSQL connection string (required)")
	fmt.Println("                  Example: postgres://user:password@host:port/database")
	fmt.Println()
	fmt.Println("Migrations Directory:")
	fmt.Println("  Place your migration files in ./supabase/migrations/")
	fmt.Println("  Format: {timestamp}_{name}.sql")
	fmt.Println("  Example: 20240101120000_create_users_table.sql")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  export DATABASE_URL=\"postgres://postgres:postgres@localhost:5432/mydb\"")
	fmt.Println("  supabase-direct-migrate")
	fmt.Println()
	fmt.Println("For more information, visit:")
	fmt.Println("  https://github.com/DaviSMoura/supabase-direct-migrate")
}

func main() {
	help := flag.Bool("help", false, "Show help message")
	flag.BoolVar(help, "h", false, "Show help message")
	flag.Parse()

	if *help {
		printHelp()
		os.Exit(0)
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fmt.Println("Error: DATABASE_URL environment variable is required")
		fmt.Println("Run with --help for usage information")
		os.Exit(1)
	}

	ctx := context.Background()

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	fmt.Println("Loading database state...")

	// Create schema if it doesn't exist
	_, err = db.ExecContext(ctx, fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s`, schemaName))
	if err != nil {
		panic(fmt.Errorf("error creating schema: %v", err))
	}

	// Create table if it doesn't exist
	_, err = db.ExecContext(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s.%s (
			version TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			hash TEXT NOT NULL,
			statements TEXT[] NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			created_by TEXT,
			idempotency_key TEXT
		)
	`, schemaName, tableName))
	if err != nil {
		panic(fmt.Errorf("error creating table: %v", err))
	}

	// Fetch already applied migrations
	rows, err := db.QueryContext(ctx,
		fmt.Sprintf(`SELECT version, hash FROM %s.%s`, schemaName, tableName))
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	applied := map[string]string{}
	for rows.Next() {
		var version string
		var hash string
		if err := rows.Scan(&version, &hash); err != nil {
			panic(err)
		}
		applied[version] = hash
	}

	// Load local migrations
	localMigrations, err := loadLocalMigrations()
	if err != nil {
		panic(err)
	}

	fmt.Printf("Found %d local migrations.\n", len(localMigrations))

	// Apply pending migrations
	for _, m := range localMigrations {
		_, already := applied[m.Version]
		if already {
			fmt.Printf("Migration already applied: %s (%s)\n", m.Version, m.Name)
			continue
		}

		fmt.Printf("Applying pending migration: %s (%s)\n", m.Version, m.Name)

		tx, err := db.BeginTx(ctx, &sql.TxOptions{})
		if err != nil {
			panic(err)
		}

		success := false

		// Apply statements
		for _, stmt := range m.Statements {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				fmt.Printf("Error executing statement: %v\n", err)
				tx.Rollback()
				panic(err)
			}
		}

		// Insert into control table
		arrayStr := formatPostgresArray(m.Statements)
		_, err = tx.ExecContext(ctx,
			fmt.Sprintf(`
				INSERT INTO %s.%s
					(version, name, hash, statements, created_by, idempotency_key)
				VALUES
					($1, $2, $3, $4::text[], $5, NULL)
			`, schemaName, tableName),
			m.Version,
			m.Name,
			m.Hash,
			arrayStr,
			"supabase-direct-migrate",
		)
		if err != nil {
			tx.Rollback()
			panic(err)
		}

		if err := tx.Commit(); err != nil {
			panic(err)
		}

		success = true
		if success {
			fmt.Printf("Migration %s applied successfully.\n", m.Version)
		}
	}

	fmt.Println("All pending migrations have been applied.")
}

func formatPostgresArray(arr []string) string {
	if len(arr) == 0 {
		return "{}"
	}
	escaped := make([]string, len(arr))
	for i, v := range arr {
		v = strings.ReplaceAll(v, `\`, `\\`)
		v = strings.ReplaceAll(v, `"`, `\"`)
		escaped[i] = `"` + v + `"`
	}
	return "{" + strings.Join(escaped, ",") + "}"
}