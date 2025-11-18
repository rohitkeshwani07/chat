# Database Migrations

This directory contains database migration files for the chat backend service.

## Migration Files

Migrations follow the naming convention: `{version}_{description}.{up|down}.sql`

- `up.sql`: Applied when migrating forward
- `down.sql`: Applied when rolling back

## Running Migrations

### Using Docker Compose

Migrations run automatically when starting services:

```bash
docker compose up
```

The `migrate` service runs before backend services start, ensuring the database schema is up-to-date.

### Running Migrations Manually with Docker

```bash
# Run migrations up
docker compose run --rm migrate ./migrate up

# Roll back all migrations
docker compose run --rm migrate ./migrate down

# Check current migration version
docker compose run --rm migrate ./migrate version

# Force migration to specific version
docker compose run --rm migrate ./migrate force 1
```

### Running Migrations Locally

```bash
# Set environment variables
export DB_HOST=localhost
export DB_PORT=5432
export DB_USER=postgres
export DB_PASSWORD=postgres
export DB_NAME=chat
export DB_SSLMODE=disable
export MIGRATIONS_PATH=file://./migrations

# Run migrations
cd backend
go run cmd/migrate/main.go up
```

## Creating New Migrations

1. Create two files in this directory:
   - `{version}_{description}.up.sql` - Forward migration
   - `{version}_{description}.down.sql` - Rollback migration

2. Use sequential version numbers (e.g., 000001, 000002, etc.)

3. Example:
   ```bash
   # Create migration files
   touch migrations/000002_add_messages_table.up.sql
   touch migrations/000002_add_messages_table.down.sql
   ```

4. Write your SQL:
   ```sql
   -- 000002_add_messages_table.up.sql
   CREATE TABLE messages (
     id UUID PRIMARY KEY,
     session_id UUID NOT NULL,
     content TEXT,
     created_at TIMESTAMP DEFAULT NOW()
   );
   ```

   ```sql
   -- 000002_add_messages_table.down.sql
   DROP TABLE IF EXISTS messages;
   ```

## Migration Commands

- `up`: Apply all pending migrations
- `down`: Roll back all migrations
- `version`: Display current migration version
- `force N`: Force database to specific version (use with caution)

## Best Practices

1. Always test migrations locally before deploying
2. Create both up and down migrations for every change
3. Make migrations idempotent when possible (use `IF EXISTS`, `IF NOT EXISTS`)
4. Keep migrations small and focused on single changes
5. Never modify existing migration files after they've been applied to production
