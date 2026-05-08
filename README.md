# Atmos Core 

Personal environmental telemetry and carbon intelligence backend.

## Why behind this project?
I’ve been fascinated by recent Climate Tech projects that focus on carbon capture and finding hundreds of different ways to utilize captured carbon. That got me thinking — why not start with tracking carbon emissions at an individual level?

This project is an attempt to visualize the impact each of us has on the planet by tracking the aspects of our carbon footprint that can realistically be measured. 

## Stack

| Layer | Technology |
|---|---|
| Language | Go 1.24 |
| Framework | GoFiber v2 |
| Database | PostgreSQL 16 |
| ORM | GORM |
| Auth | JWT (access + refresh) |
| Logging | Zap |
| Docs | Swagger (swaggo) |

## Quick start

### Prerequisites
- Go 1.22+
- Docker + Docker Compose (or a local PostgreSQL instance)

### 1. Clone and configure

```bash
git clone https://github.com/dipu989/atmos-core.git
cd atmos-core
cp .env.example .env
# Edit .env — set DB_* and JWT_* values
```

### 2. Start the database

```bash
make docker-db        # starts postgres on :5433
```

### 3. Run migrations and seeds

```bash
make migrate          # applies all pending SQL migrations
make seed             # seeds emission factors and reference data
```

### 4. Start the server

```bash
make run              # builds and starts on :8080
# or for live reload:
make dev              # requires air (installed automatically)
```

### 5. Explore the API

Swagger UI: [http://localhost:8080/swagger/index.html](http://localhost:8080/swagger/index.html)

Health check: `GET /health`

## Project structure

```
atmos-core/
├── cmd/
│   ├── api/          # HTTP server entrypoint
│   ├── migrate/      # Migration runner
│   └── seed/         # Database seeder
├── config/           # Env-based config loader
├── docs/             # Generated Swagger docs (do not edit)
├── internal/
│   ├── activity/     # Activity ingestion module
│   ├── auth/         # Authentication module
│   ├── device/       # Device registration module
│   ├── emission/     # Emission calculation engine
│   ├── identity/     # User profile module
│   ├── insight/      # Insight rules engine
│   ├── integration/  # External connectors (Uber, Gmail — future)
│   ├── seeds/        # Database seeders
│   └── timeline/     # Aggregation and summary module
├── migrations/       # Ordered SQL migration files
└── platform/
    ├── database/     # PostgreSQL connection
    ├── eventbus/     # In-process event bus
    ├── jwt/          # Token manager
    ├── logger/       # Zap logger
    ├── middleware/   # Auth, CORS, rate limit, request ID
    ├── response/     # Standardised HTTP envelope
    ├── uuid/         # UUIDv7 generator
    └── validator/    # Request body parser + validator
```

## Makefile targets

```
make build        Build all binaries (api, migrate, seed)
make run          Build and run the API server
make dev          Run with live reload (requires air)
make migrate      Apply pending database migrations
make migrate-dry  Show pending migrations without applying
make seed         Run all database seeders
make swagger      Regenerate Swagger docs
make lint         Run golangci-lint
make fmt          Format all Go files
make test         Run all tests
make docker-up    Build and start all services via Docker Compose
make docker-down  Stop all containers
make docker-logs  Tail API container logs
make clean        Remove build artifacts
```

## Environment variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `APP_ENV` | No | `development` | `development` or `production` |
| `APP_PORT` | No | `8080` | HTTP listen port |
| `DB_HOST` | Yes | — | PostgreSQL host |
| `DB_PORT` | No | `5432` | PostgreSQL port |
| `DB_USER` | Yes | — | PostgreSQL user |
| `DB_PASSWORD` | Yes | — | PostgreSQL password |
| `DB_NAME` | Yes | — | PostgreSQL database name |
| `DB_SSLMODE` | No | `disable` | `disable` / `require` / `verify-full` |
| `JWT_ACCESS_SECRET` | Yes | — | Signing secret for access tokens (min 32 chars) |
| `JWT_REFRESH_SECRET` | Yes | — | Signing secret for refresh tokens (min 32 chars) |
| `JWT_ACCESS_TTL` | No | `15m` | Access token lifetime |
| `JWT_REFRESH_TTL` | No | `720h` | Refresh token lifetime (30 days) |
| `GOOGLE_CLIENT_ID` | No | — | Google OAuth client ID |
| `GOOGLE_CLIENT_SECRET` | No | — | Google OAuth client secret |
| `GOOGLE_REDIRECT_URL` | No | — | Google OAuth callback URL |

## Event flow

```
POST /activities
  → ActivityIngested event
    → EmissionService   — resolves factor, calculates kg CO₂e
      → EmissionCalculated event
        → TimelineService — upserts daily/weekly/monthly summaries
        → InsightService  — evaluates streak and milestone rules
```

## API overview

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/auth/register` | Public | Register with email + password |
| POST | `/api/v1/auth/login` | Public | Login |
| POST | `/api/v1/auth/token/refresh` | Public | Rotate refresh token |
| POST | `/api/v1/auth/logout` | Public | Revoke refresh token |
| GET | `/api/v1/users/me` | Bearer | Get profile |
| PATCH | `/api/v1/users/me` | Bearer | Update profile |
| POST | `/api/v1/devices` | Bearer | Register device |
| GET | `/api/v1/devices` | Bearer | List devices |
| DELETE | `/api/v1/devices/:id` | Bearer | Deregister device |
| POST | `/api/v1/activities` | Bearer | Ingest activity |
| GET | `/api/v1/activities` | Bearer | List activities |
| GET | `/api/v1/activities/:id` | Bearer | Get activity |
| GET | `/api/v1/timeline/day/:date` | Bearer | Daily summary |
| GET | `/api/v1/timeline/week/:week_start` | Bearer | Weekly summary |
| GET | `/api/v1/timeline/month/:year/:month` | Bearer | Monthly summary |
| GET | `/api/v1/timeline/range` | Bearer | Date range summaries |
| GET | `/api/v1/insights` | Bearer | List insights |
| PATCH | `/api/v1/insights/:id/read` | Bearer | Mark insight read |
| GET | `/health` | Public | Health check |
| GET | `/swagger/*` | Public | Swagger UI |

## License

MIT
