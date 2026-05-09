## Overview

<!-- What does this PR do and why? One or two sentences. -->

## Changes

<!-- Bullet list of what changed. Be specific — file paths, function names, behaviour. -->

-
-

## API Updates

<!-- List any new, modified, or removed endpoints. Use "None" if not applicable. -->

| Method | Path | Change |
|--------|------|--------|
| | | |

## Database Changes

<!-- Describe schema changes. Include migration file name(s). Use "None" if not applicable. -->

- **Migration:** `migrations/NNN_description.sql`
- **Tables affected:**
- **Indexes added/removed:**
- **Backward compatible:** yes / no

## Notes

<!-- Anything reviewers should know: tradeoffs made, follow-up tickets, known limitations. -->

## Manual Verification

- [ ] Server starts without errors (`make run`)
- [ ] New/changed endpoints return correct status codes
- [ ] Validation rejects invalid payloads
- [ ] Idempotency holds for duplicate requests (if applicable)
- [ ] No N+1 queries introduced (check query logs)
- [ ] Migration runs cleanly on a fresh DB (`make migrate`)
- [ ] Existing endpoints are unaffected (smoke tested)
- [ ] Swagger docs updated (`make swag`)
