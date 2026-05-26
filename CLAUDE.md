# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Go backend for an admin panel system (`kite-admin`). Provides user/role/permission management, scheduled task execution, a database-backed job queue, media/file management with pluggable storage backends, and operation audit logging.

## Common Commands

```bash
# Run the server
go run main.go

# Build
go build -o ./tmp/main.exe ./main.go

# Hot-reload development (requires Air: go install github.com/air-verse/air@latest)
air

# Manage dependencies
go mod tidy

# Reset database (drops all tables before migration — dev only)
RESET_DB=1 go run main.go
```

There are no tests, linter configs, or CI pipelines in this project.

## Architecture

### Startup Sequence (main.go)

1. `config.LoadConfig()` — loads `.env` via godotenv
2. `config.InitDB()` — connects to MySQL via GORM (`config.DB` global)
3. `config.AutoMigrate()` — GORM auto-migration for all 12 models
4. `config.Seed()` — syncs permission tree from code to DB (every startup), creates admin user/roles on first run
5. `scheduler.Init()` — starts cron scheduler, loads enabled tasks from DB
6. `queue.Init()` — starts queue manager polling loop
7. Route registration and Gin server start

### Layer Structure

- **`config/`** — DB connection, auto-migration, seed data. `config.DB` is the global GORM instance used everywhere.
- **`models/`** — GORM model definitions. Response helpers (`Response`, `PageData`) in `response.go`.
- **`controllers/`** — Gin HTTP handlers. Each file corresponds to a domain (auth, user, role, permission, task, queue, media, syslog).
- **`middleware/`** — CORS, JWT auth (`AuthMiddleware`), RBAC permission check (`RequirePermission("CODE")`), async operation audit logging (`OperationLog`).
- **`routes/routes.go`** — All route definitions. Public routes under `/auth`, authenticated routes use `AuthMiddleware` + `OperationLog`. Write endpoints are guarded by `RequirePermission`.
- **`scheduler/`** — Cron task system built on `robfig/cron/v3`. Supports HTTP, Shell, and FUNC task types. Register new built-in functions in `scheduler/builtin.go`.
- **`queue/`** — Database-backed job queue. Register handlers with `queue.Handle("name", fn)`. Push jobs with `queue.Push(ctx, "name", payload)`. Polls every 2s.
- **`storage/`** — `Storage` interface (`Put`, `Delete`, `DeletePrefix`) with Local and S3 implementations.
- **`utils/`** — JWT, bcrypt password, captcha helpers.

### Key Patterns

- **Permission tree is code-defined**: `config/seed.go` contains `defaultPermissions()` with hardcoded IDs. This is the single source of truth — changes sync to DB automatically on startup. New endpoints need a corresponding permission entry here.
- **Unified response format**: All controllers return `models.Response` with `Code: 0` for success. Use `models.Success()`, `models.Fail()`, `models.PageSuccess()` helpers.
- **RBAC**: `RequirePermission("PERMISSION_CODE")` middleware checks role permissions. `SUPER_ADMIN` role bypasses all permission checks.
- **Audit logging**: `OperationLog` middleware captures request/response bodies and latency into `SysLog` asynchronously (via goroutine).

### Database

MySQL via GORM. Connection configured through `.env` variables (`DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`). No migration files — schema is managed by GORM's `AutoMigrate`.

### Configuration

All config via `.env` file (loaded by godotenv). Key variables: `SERVER_PORT`, `DB_*`, `JWT_SECRET`, `JWT_EXPIRE_HOURS`. Default admin credentials: `admin` / `123456`.
