# Kaderisasi public web API in Go

Agent contribution rules: [AGENTS.md](AGENTS.md). `CLAUDE.md` imports the same rules.

The Go replacement for the AdonisJS public member API (`../kaderisasi-web-be`), on
port **3333**. Routes move here group by group. Every route not ported yet is
reverse-proxied unchanged to Adonis at `WEB_BE_LEGACY_URL`, so frontends keep one API
origin throughout. Phases, compatibility rules, local running and deployment are in
[docs/MIGRATION.md](docs/MIGRATION.md).

## Local setup

Required: Go **1.26.8**, sqlc **1.31.1**, Node **24** (runner, interop fixtures and contract checks).

```sh
npm ci --prefix tests/interop
make check
make test-unit       # includes byte-level interop tests against the real Node libraries
make test-contract   # differential check against Adonis web-be (owned test schema)
```

## Running

```sh
# kaderisasi-web-be on 3335, then:
make run
```

`make run` builds `bin/web-api` and starts it with `../env/test/web-be-test-env`.
That file targets the shared dev database, so it is for reads and manual checks only.

## Database

The schema is owned by the Ace migrations in `../kaderisasi-admin-be`.
`database/schema.sql` is a sqlc-only snapshot copied from
`../kaderisasi-admin-be-go/database/schema.sql`. After a migration, refresh both
snapshots, then run `make sqlc`. Never hand-edit `internal/dbgen`.
