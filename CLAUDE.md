# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Go backend for the `kite-admin` admin panel. It provides user/role/permission management, system configuration, login and operation logs, scheduled task execution, a database-backed job queue, media/file management with local or S3-compatible storage, in-app messages with SSE updates, and email configuration/templates.

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

# Regenerate Swagger docs (requires swag: go install github.com/swaggo/swag/cmd/swag@latest)
swag init

# Reset database (drops all tables before migration — dev only)
RESET_DB=1 go run main.go
```

On Windows PowerShell, use `$env:RESET_DB = '1'; go run main.go` for the reset command.

There are no tests, linter configs, or CI pipelines in this project.

## Architecture

### Startup Sequence (main.go)

1. `config.LoadConfig()` — loads `.env` via godotenv and falls back to defaults
2. `config.InitDB()` — connects to MySQL via GORM (`config.DB` global)
3. `config.AutoMigrate()` — GORM auto-migration for every model in `config.RegisteredModels()`
4. `config.Seed()` — syncs permission tree and seeds initial admin/roles/config/templates/storage data
5. `scheduler.Init()` — starts cron scheduler and loads enabled DB tasks
6. `queue.Init()` — starts the database queue manager polling loop
7. `mountLocalStorage()` — mounts enabled local storage configs as Gin static directories
8. `routes.SetupRoutes()` — registers Swagger, public auth routes, authenticated API routes, and SSE route
9. Gin server starts on `SERVER_PORT`

### Layer Structure

- **`config/`** — environment config, DB connection, registered model list, auto-migration, seed data. `config.DB` is the global GORM instance used everywhere.
- **`models/`** — GORM models and response helpers (`Response`, `PageData`, `Success`, `Fail`, `PageSuccess`).
- **`controllers/`** — Gin HTTP handlers by domain: auth, users, roles, permissions, logs, system config, tasks, queues, media/storage, messages, email config/templates.
- **`middleware/`** — CORS, JWT auth (`AuthMiddleware`), RBAC permission check (`RequirePermission("CODE")`), async operation audit logging (`OperationLog`).
- **`routes/routes.go`** — All route definitions. Public routes live under `/auth`; authenticated routes use `AuthMiddleware` + `OperationLog`; SSE uses only `AuthMiddleware` so streaming responses are not buffered.
- **`scheduler/`** — Cron task system built on `robfig/cron/v3`. Supports HTTP, Shell, and FUNC task types. Register new built-in functions in `scheduler/builtin.go`.
- **`queue/`** — Database-backed job queue. Register handlers with `queue.Handle("name", fn)`. Push jobs with `queue.Push(ctx, "name", payload)`. Polls every 2 seconds.
- **`storage/`** — `Storage` interface (`Put`, `Delete`, `DeletePrefix`) with Local and S3 implementations.
- **`sse/`** — SSE hub for in-app message notifications.
- **`docs/`** — Generated Swagger files imported by `main.go`.
- **`utils/`** — JWT, bcrypt password, captcha helpers.

### Key Patterns

- **Permission tree is code-defined**: `config/seed.go` contains `defaultPermissions()` with hardcoded IDs/codes. This is the single source of truth and syncs to DB on startup. New protected endpoints need corresponding permission entries.
- **Registered models are centralized**: add new GORM models to `config.RegisteredModels()` so `AutoMigrate()` and `RESET_DB=1` include them.
- **Unified response format**: Controllers return `models.Response` with `Code: 0` for success. Use `models.Success()`, `models.Fail()`, and `models.PageSuccess()` helpers.
- **RBAC**: `RequirePermission("PERMISSION_CODE")` middleware checks role permissions. `SUPER_ADMIN` role bypasses permission checks.
- **Audit logging**: `OperationLog` captures request/response bodies and latency into `SysLog` asynchronously. Do not wrap SSE routes with it because it buffers responses.
- **System config exposure**: `/auth/system/config` is intentionally public for frontend bootstrapping; authenticated `/system/config` exposes the same data behind auth.
- **Local media serving**: enabled LOCAL storage configs are mounted at startup using their `PublicPrefix` and `LocalDir`; defaults are `/uploads` and `./uploads`.

### Database

MySQL via GORM. Connection is configured through `.env` variables (`DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`). No migration files are used — schema is managed by GORM `AutoMigrate`.

The current registered model set includes users/profiles, roles, permissions, logs, tasks/task logs, queues/jobs, media/folders/storage configs, messages/recipients, email config/templates, login logs, and system config.

### Configuration

All config comes from `.env` loaded by godotenv, with defaults in `config/config.go`:

- `SERVER_PORT` default `:8080`
- `DB_HOST` default `localhost`
- `DB_PORT` default `3306`
- `DB_USER` default `root`
- `DB_PASSWORD` default `123456`
- `DB_NAME` default `admin_system`
- `JWT_SECRET` default `your-secret-key`
- `JWT_EXPIRE_HOURS` default `24`

Default admin credentials are `admin` / `123456`.

### API Documentation

Swagger UI is mounted at `/swagger/*any`, usually `/swagger/index.html` after the server starts. Swagger files are generated under `docs/`; run `swag init` after changing Swagger annotations.
