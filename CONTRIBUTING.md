# Contributing to Atmos Core

## Branch Naming

| Type | Pattern | Example |
|------|---------|---------|
| Feature | `feat/<short-description>` | `feat/timeline-trend-api` |
| Bug fix | `fix/<short-description>` | `fix/emission-nil-factor` |
| Chore / refactor | `chore/<short-description>` | `chore/cleanup-identity-service` |
| Migration | `db/<short-description>` | `db/add-device-push-token-index` |
| Infrastructure | `infra/<short-description>` | `infra/k8s-resource-limits` |

Branch off `main`. Never commit directly to `main`.

## Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/). The subject line must be lowercase and under 72 characters.

```
feat(timeline): add trend calculation to daily summary endpoint

fix(emission): return zero factor instead of nil when mode is unresolvable

chore(deps): upgrade gofiber to v2.52.5

db: add index on activities(user_id, date_local)

refactor(identity): extract preference upsert into dedicated repository method

test(activity): add idempotency key collision integration test

docs(api): regenerate swagger after timeline route changes
```

**Types:** `feat`, `fix`, `chore`, `refactor`, `test`, `db`, `infra`, `docs`, `perf`

Scope is the module name: `auth`, `identity`, `device`, `activity`, `emission`, `timeline`, `insight`.

## Pull Request Workflow

1. Create a branch from `main`.
2. Make focused, reviewable commits.
3. Fill in the PR template completely — empty sections slow reviews.
4. Self-review your diff before requesting review. Check for debug logs, commented-out code, and hardcoded values.
5. All PRs are **squash merged** into `main`. The squash commit message becomes the changelog entry, so write it well.
6. Delete the branch after merge.

### Squash Merge Strategy

Every PR lands as a single commit on `main`. This keeps `git log` readable and makes bisecting tractable.

The squash commit subject must follow the conventional commit format. GitHub will prefill it from the PR title — set your PR title correctly from the start.

## Coding Expectations

### General

- No feature flags, backwards-compat shims, or dead code. Change the thing directly.
- No error handling for scenarios that cannot happen. Trust GORM, Fiber, and Go stdlib contracts at internal call sites; validate only at system boundaries (HTTP handlers, event payloads).
- Comments only when the *why* is non-obvious — a hidden constraint, a known upstream bug, a subtle invariant. Never explain *what* the code does.

### Architecture

- Each module follows the layered structure: `domain → repository → service → handler`.
- Services never import handlers. Repositories never import services. Domain imports nothing internal.
- Cross-module communication goes through `platform/eventbus` — direct imports between modules are not allowed.
- All primary keys are UUIDv7, generated at the application layer via `platform/uuid.New()`.

### HTTP / Fiber

- Use `platform/validator.ParseAndValidate` in every handler that reads a request body. It returns `fiber.NewError` — never call `c.Status(...).JSON(...)` and return `nil`.
- `oneof` validation does not work on pointer fields (`*string`). Use plain `string` and treat empty string as "not provided".
- Return `response.Created` for POST, `response.OK` for GET/PUT/PATCH, `response.NoContent` for DELETE.

### Database

- All schema changes go through numbered migration files in `migrations/`. No `AutoMigrate` in production code.
- Use `clause.OnConflict` for upserts. Never `DELETE + INSERT`.
- Add an index for every foreign key and every column that appears in a `WHERE` clause on large tables.

### Go Style

- `gofmt` and `goimports` before committing. The build pipeline will reject unformatted code.
- Prefer table-driven tests with `t.Run` subtests.
- Integration tests that touch the database must use a real PostgreSQL instance — no mocks.

## Running Locally

For first-time setup, the bootstrap script handles everything:

```bash
make bootstrap      # validates tools, starts postgres, migrates, seeds
```

Day-to-day commands:

```bash
make infra-up       # start PostgreSQL (if not already running)
make migrate-up     # apply pending migrations
make seed           # re-run seeders (idempotent)
make dev            # start API with live reload
```

Resetting your local database (destructive — drops and recreates):

```bash
make reset-db
```

Running smoke tests against a locally running API:

```bash
make smoke-test
# or against another target:
BASE_URL=http://localhost:8080 make smoke-test
```

## Generating Swagger Docs

```bash
make swag
```

Commit the updated `docs/` directory with any PR that changes API contracts.
