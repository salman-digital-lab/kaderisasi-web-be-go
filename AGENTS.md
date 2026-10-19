# Go public web API rules

When working in the full workspace, also read `../AGENTS.md`.

- This repo replaced `../kaderisasi-web-be` (AdonisJS) and serves every public `/v2` route.
  Read `docs/MIGRATION.md` before changing a route. Keep the web-be contract the
  frontends depend on: methods, paths, statuses, error bodies, JSON names, null/missing
  behavior, pagination and dates. Change a contract only together with its frontend
  consumer, and document deliberate deviations in `docs/MIGRATION.md`.
- New public API features go here, not in web-be. Unknown paths answer Adonis's
  `404 Cannot <METHOD>:<path>`; there is no proxy.
- Use standard `net/http`. HTTP, middleware and handlers go in `internal/webapi`;
  workflows go in feature packages under `internal/`.
- SQL goes in `database/queries`; run `make sqlc`. `database/schema.sql` is a snapshot
  copied from `../kaderisasi-admin-be-go/database/schema.sql`. Migrations happen only
  through Ace in `../kaderisasi-admin-be`.
- Learner tokens must stay interoperable with Adonis (`internal/auth/public.go`,
  checked by `TestPublicAccessAdonisInteroperability`). `APP_KEY` must match web-be and
  the admin API.
- Checks: `make check`, `make test-unit` (needs `npm ci --prefix tests/interop` and libvips).
  `make test-contract` compares against Adonis web-be in owned twin schemas while that
  checkout is still around; keep it passing when touching a route.
- Never run against `../env/prod/*`. The test env targets the shared dev database:
  reads only, no resets or seeding.
