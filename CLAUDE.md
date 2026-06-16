# atmos-core

Go/Fiber REST backend. Postgres via GORM. Side-effects are event-driven through `platform/eventbus` — handlers never call downstream services directly.

## Commands

```bash
go build ./...    # must pass before every commit
go test ./...     # run all tests
gofmt -l .        # must output nothing — CI rejects unformatted files
gofmt -w .        # fix formatting
```

## Commits — enforced by commitlint in CI

```
<type>(<scope>): <subject>
```

Allowed: `feat` `fix` `perf` `refactor` `chore` `docs` `test` `ci` `revert`  
**Not allowed (fail CI):** `build` `style`

## Rules

- Never push to `main`. Branch → PR always.
- Migrations live in `migrations/NNN_name.sql`. Never alter tables outside migrations.
- CO₂ is computed in the emission service. Never inline emission factors anywhere else.
- `receipt_id` is system-only — strip it from any external API ingest path.
- Targeted DB updates use `map[string]any{}` with GORM `.Updates()`. Never `Save()` a partial struct — it overwrites all fields.
- Nil-guard each coord column independently (`OriginLat`, `OriginLng`, `DestLat`, `DestLng`). Pairing them in one guard causes a nil-panic if a single coord is missing.
- `SourceGPSReceipt` ("gps+receipt") is a receipt source for dedup. `isReceiptSource()` must include it so already-merged rows stay visible on retry and split-session ingest.

## Non-obvious gotchas

**effectiveEnd fallback** — when `EndedAt` and `DurationMinutes` are both nil, `effectiveEnd` assumes 60 min. This inflates match windows. Check `MatchResult.HasEndTime` and apply `ThresholdAutoMergeNoCoord` (0.75) instead of the normal 0.65 when it's false.

**Places handler** — uses Google Places Text Search (`/place/textsearch/json`), not Geocoding API. Geocoding does not do prefix/partial matching. Don't swap them.

**Event bus** — `IngestWithDedup` (receipt→GPS) and `Ingest` GPS path both publish `EventActivityPossibleDuplicate` on review-range match (0.45–0.65). `NotificationService` fans it out to FCM. Don't publish from inside `createActivity` — the event carries the created activity's ID, which only exists after it returns.
