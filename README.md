# Supabase Direct Migrate

Go tool to apply Supabase migrations directly to PostgreSQL database without needing the Supabase CLI.

## Features

- Applies migrations from `supabase/migrations` directory
- Compatible with Supabase migration format
- Detects already applied migrations (idempotent)
- Uses transactions to ensure consistency
- Automatically creates schema and control table
- Calculates SHA-256 hash same as Supabase

## Installation

### Option 1: Build from source

```bash
git clone https://github.com/DaviSMoura/supabase-direct-migrate.git
cd supabase-direct-migrate
go build -o apply_migrations apply_migrations.go
```

### Option 2: Install via go install

```bash
go install github.com/DaviSMoura/supabase-direct-migrate@latest
```

## Usage

### 1. Set the environment variable

```bash
export DATABASE_URL="postgres://user:password@host:port/database"
```

### 2. Place your migrations in the correct directory

Migrations should be in `./supabase/migrations/` in the format:
```
{timestamp}_{name}.sql
```

Example: `20240101120000_create_users_table.sql`

### 3. Run the script

```bash
./apply_migrations
```

Or if installed via `go install`:

```bash
apply_migrations
```

## Migration Format

The script supports the standard Supabase format, splitting statements by `-- statement-breakpoint`:

```sql
-- statement-breakpoint
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    email TEXT UNIQUE NOT NULL
);

-- statement-breakpoint
CREATE INDEX idx_users_email ON users(email);
```

## How It Works

1. Connects to PostgreSQL database using `DATABASE_URL`
2. Creates `supabase_migrations` schema if it doesn't exist
3. Creates `schema_migrations` table if it doesn't exist
4. Loads all migrations from `./supabase/migrations` directory
5. Checks which migrations have already been applied
6. Applies only pending migrations in transactions
7. Records each migration in the control table

## Control Table Structure

The script creates and maintains the `supabase_migrations.schema_migrations` table:

```sql
CREATE TABLE supabase_migrations.schema_migrations (
    version TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    hash TEXT NOT NULL,
    statements TEXT[] NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    created_by TEXT,
    idempotency_key TEXT
);
```

## Complete Example

```bash
# 1. Set the connection
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/mydb"

# 2. Create a migration
echo 'CREATE TABLE test (id SERIAL PRIMARY KEY);' > supabase/migrations/20240101120000_test.sql

# 3. Run
./apply_migrations

# Output:
# Loading database state...
# Found 1 local migrations.
# Applying pending migration: 20240101120000 (test.sql)
# Migration 20240101120000 applied successfully.
# All pending migrations have been applied.
```

## Requirements

- Go 1.21 or higher
- PostgreSQL 12 or higher
- Access to PostgreSQL database

## License

MIT

## Contributing

Contributions are welcome! Feel free to open issues and pull requests.

