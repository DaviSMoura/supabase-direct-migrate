-- statement-breakpoint
CREATE TABLE IF NOT EXISTS test_table (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- statement-breakpoint
CREATE INDEX IF NOT EXISTS idx_test_table_name ON test_table(name);

