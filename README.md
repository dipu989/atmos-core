<h1 align="center">Atmos Core</h1>

<img width="1983" height="793" alt="atmos-backend-image" src="https://github.com/user-attachments/assets/d102a95b-6936-40d3-bf1c-5a9011a35ef9" />

## Why behind this project?
I’ve been fascinated by recent Climate Tech projects that focus on carbon capture and finding hundreds of different ways to utilize captured carbon. That got me thinking — why not start with tracking carbon emissions at an individual level?

This project is an attempt to visualize the impact each of us has on the planet by tracking the aspects of our carbon footprint that can realistically be measured, eventually evolving into a personal environmental telemetry and carbon intelligence platform.

## Stack

| Layer | Technology |
|---|---|
| Language | Go 1.26 |
| Framework | GoFiber v2 |
| Database | PostgreSQL 16 |
| ORM | GORM |
| Auth | JWT (access + refresh) |
| Logging | Zap |
| Docs | Swagger (swaggo) |

## Quick start

### Prerequisites
- Go 1.26+
- Docker + Docker Compose

### First-time setup

Run the bootstrap script — it validates tooling, copies `.env`, starts postgres, runs migrations, and seeds the database:

```bash
git clone https://github.com/dipu989/atmos-core.git
cd atmos-core
make bootstrap
```

Then edit `.env` and set `JWT_ACCESS_SECRET` and `JWT_REFRESH_SECRET` to random 32-char strings:

```bash
openssl rand -hex 32   # run twice — one value per secret
```

### Start the server

```bash
make dev              # live reload via Air (installed automatically)
# or
make run              # compile and run once
```

### Explore the API

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

| Target | Description |
|---|---|
| `make bootstrap` | First-time setup — tools, infra, migrate, seed |
| `make dev` | Run with live reload (Air installed automatically) |
| `make run` | Build and start the API |
| `make build` | Compile all binaries (`api`, `migrate`, `seed`) |
| **Database** | |
| `make migrate-up` | Apply all pending migrations |
| `make migrate-down` | Roll back the last migration |
| `make migrate-dry` | Preview pending migrations without applying |
| `make seed` | Run all seeders (idempotent) |
| `make reset-db` | Drop, recreate, migrate, and seed — destructive |
| **Infrastructure** | |
| `make infra-up` | Start PostgreSQL via Docker Compose |
| `make infra-down` | Stop all Docker Compose services |
| `make docker-logs` | Tail API container logs |
| **Quality** | |
| `make fmt` | Format all Go files with `gofmt` |
| `make lint` | Run `golangci-lint` |
| `make vet` | Run `go vet` |
| `make test` | Run all tests with race detector |
| `make swag` | Regenerate Swagger docs |
| `make smoke-test` | Curl-based end-to-end API smoke tests |
| `make clean` | Remove build artifacts (`bin/`, `tmp/`) |

Run `make help` to see the full list in the terminal.

## Environment variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `APP_ENV` | No | `development` | `development` or `production` |
| `APP_PORT` | No | `8080` | HTTP listen port |
| `DB_HOST` | Yes | — | PostgreSQL host |
| `DB_PORT` | No | `5433` | PostgreSQL port (Docker Compose maps to 5433) |
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
