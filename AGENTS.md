# Go public web API rules

When working in the full workspace, also read `../AGENTS.md`.

- This repo replaces `../kaderisasi-web-be` (AdonisJS) route group by route group.
  Read `docs/MIGRATION.md` before moving a route. A moved route must keep the web-be
  contract exactly: methods, paths, statuses, error bodies, JSON names, null/missing
  behavior, pagination and dates. Document any deliberate deviation there.
- Routes not ported yet stay in web-be and are proxied. Fix bugs in unported routes in web-be.
- Use standard `net/http`. HTTP, middleware and handlers go in `internal/webapi`;
  workflows go in feature packages under `internal/`.
- SQL goes in `database/queries`; run `make sqlc`. `database/schema.sql` is a snapshot
  copied from `../kaderisasi-admin-be-go/database/schema.sql`. Migrations happen only
  through Ace in `../kaderisasi-admin-be`.
- Learner tokens must stay interoperable with Adonis (`internal/auth/public.go`,
  checked by `TestPublicAccessAdonisInteroperability`). `APP_KEY` must match web-be and
  the admin API.
- Checks: `make check`, `make test-unit` (needs `npm ci --prefix tests/interop`).
- Never run against `../env/prod/*`. The test env targets the shared dev database:
  reads only, no resets or seeding.
