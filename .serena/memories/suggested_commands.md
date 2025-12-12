# Suggested Commands for Development

## Build & Run

### Full Stack
```bash
make all                    # Build frontend and start backend
```

### Frontend Only
```bash
cd web
bun install                 # Install dependencies
bun run dev                 # Start dev server (Vite)
bun run build              # Build for production
bun run lint               # Check code formatting (Prettier)
bun run lint:fix           # Fix code formatting
bun run eslint             # Run ESLint
bun run eslint:fix         # Fix ESLint issues
```

### Backend Only
```bash
go run main.go             # Run backend (with hot reload via air)
go build -o ./tmp/main .   # Build binary
```

## Development Tools

### Git
```bash
git status                 # Check status
git add .                  # Stage changes
git commit -m "message"    # Create commit
git push                   # Push to remote
```

### Database
```bash
# SQLite (default)
# Database file: one-api.db

# MySQL/PostgreSQL
# Configure via SQL_DSN environment variable
```

### Environment
```bash
# Copy example env file
cp .env.example .env

# Key variables:
# SESSION_SECRET - Session encryption key (required for multi-instance)
# CRYPTO_SECRET - Encryption key for Redis
# SQL_DSN - Database connection string
# REDIS_CONN_STRING - Redis connection
# GIN_MODE - Set to "debug" for development
```

## Testing & Quality

### Frontend
```bash
bun run lint               # Prettier check
bun run lint:fix           # Prettier fix
bun run eslint             # ESLint check
bun run eslint:fix         # ESLint fix
```

### i18n (Internationalization)
```bash
bun run i18n:extract       # Extract i18n strings
bun run i18n:status        # Check i18n status
bun run i18n:sync          # Sync i18n files
bun run i18n:lint          # Lint i18n files
```

## Docker

```bash
# Build image
docker build -t new-api:latest .

# Run with SQLite
docker run --name new-api -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  new-api:latest

# Docker Compose
docker-compose up -d
docker-compose down
```

## Useful Utilities

```bash
# Check Go version
go version

# Check Node/Bun version
bun --version

# View logs
tail -f logs/*.log

# Hot reload (backend)
# Configured in .air.toml - automatically watches Go files
```
