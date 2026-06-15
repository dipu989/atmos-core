# CLAUDE.md

This file provides guidance to Claude Code when working with code in this repository.

---

## What this repo is

`atmos-core` is the Go/GoFiber backend for the Atmos personal carbon-footprint tracker. It exposes a REST API consumed by the mobile app (`atmos-mobile`) and the web dashboard (`atmos-web`).

- **Framework**: Go 1.22 + GoFiber v2
- **Database**: PostgreSQL 16 via GORM
- **Auth**: JWT (access + refresh)
- **Email ingestion**: Gmail OAuth2 — Uber/Rapido receipt parsing → activity dedup pipeline
- **CO₂ calculation**: backend-only (DEFRA IN 2023 factors); never compute emissions locally

---

## Commands

```bash
# Run the API server (requires .env)
make dev              # or: go run ./cmd/api

# Build
go build ./...

# Tests
go test ./...

# Formatting (must pass before push)
gofmt -w .
gofmt -l .            # lists non-compliant files; must be empty

# Vet
go vet ./...

# Database migrations (goose)
make migrate-up
make migrate-down
```

---

## CI pipeline

Every PR runs three checks — all must pass before merge:

| Check | Command | Fail condition |
|---|---|---|
| **Formatting** | `gofmt -l .` | Any file listed → fix with `gofmt -w <file>` |
| **Vet** | `go vet ./...` | Any issue reported |
| **Build** | `go build ./...` | Compile error |

**Run these locally before pushing:**
```bash
gofmt -w . && go vet ./... && go build ./...
```

---

## Git workflow

Always work on a feature branch — never push directly to `main`:

```bash
git checkout -b feat/<short-description>   # see branch format below
# ... make changes ...
gofmt -w . && go vet ./... && go build ./...   # must pass
git add <files>
git commit -m "feat(gmail): add receipt coordinate extraction"
git push -u origin feat/<short-description>
gh pr create
```

### Commit message format

Commits are linted by commitlint on every PR. Non-compliant messages will fail CI.

```
<type>(<scope>): <subject>

[optional body]

[optional footer]
```

**Allowed types** (others will fail the pipeline):

| Type | When to use |
|---|---|
| `feat` | New feature or capability |
| `fix` | Bug fix |
| `perf` | Performance improvement |
| `refactor` | Code change that neither fixes a bug nor adds a feature |
| `chore` | Maintenance, dependency bumps, config changes |
| `docs` | Documentation only |
| `test` | Adding or updating tests |
| `ci` | CI/CD workflow changes |
| `revert` | Reverting a previous commit |

**Not allowed** (common conventional-commits types that are excluded here): `build`, `style`

**Rules:**
- Type is **required**
- Subject is **required**, written in lowercase imperative mood (`add`, `fix`, `remove` — not `Added`)
- No period at the end of the subject line
- Scope is optional but recommended for multi-package repos

**Examples:**
```
feat(gmail): extract Google Maps coordinates from receipt HTML
fix(dedup): prevent empty-string receipt_id being written on enrich
chore: apply gofmt to all files
refactor(tripmatcher): consolidate haversineKm into tripmatcher package
test(tripmatcher): add GPS late-start edge case
ci: add go vet step to build workflow
```

### Branch naming

```
feat/<short-description>
fix/<short-description>
chore/<short-description>
```

---

## Architecture notes

- `cmd/api/main.go` — wires all services and starts the Fiber app
- `internal/<domain>/` — each domain owns its handler, service, repository, and domain types
- `internal/tripmatcher/` — pure-function scorer; no DB/IO dependencies (fully unit-testable)
- `internal/geocoder/` — Google Maps API client; degrades to no-op when `GOOGLE_MAPS_API_KEY` is absent
- `migrations/` — SQL migration files (goose); schema changes go here, not in GORM AutoMigrate
- `platform/` — shared infrastructure (logger, JWT, eventbus, middleware)

### Key invariants

- CO₂ values are always computed by the emission service, never in handlers or clients
- `receipt_id` is a system-only field set by the Gmail ingestion pipeline; it must never be accepted from external API clients
- GPS wins coordinates; receipt wins fare/distance/duration when merging a GPS trip with a receipt
