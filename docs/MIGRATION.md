# Migration from Adonis web-be

This service is replacing the AdonisJS public member API (`../kaderisasi-web-be`)
route group by route group. It listens on port **3333** and answers the routes ported
so far. Every other request is reverse-proxied unchanged to the Adonis service at
`WEB_BE_LEGACY_URL`. Frontends keep a single API origin throughout the migration, and a
route can be rolled back by unregistering it.

HTTP, middleware and the proxy live in `internal/webapi`. The learner JWT lives in
`internal/auth`, and sqlc queries in `database/queries` → `internal/dbgen`.

## Phases

| Phase | Scope | Status |
| --- | --- | --- |
| 0 | Service, config, request id/logging, CORS, learner JWT, auth middleware, proxy, `/health` | **Done** |
| 1 | `auth/*`: register, login, Google, check-email, forgot/reset password (SMTP mailer), logout | **Done**: `scripts/contract-auth.mjs` passes 47/47 |
| 2 | Public reads: activities, categories, clubs, monthly achievements, certificate verify/show (with throttle) | **Done**: `scripts/contract-public.mjs` passes 210/210, byte for byte |
| 3 | Profiles, picture, history, notifications, achievements, ruang-curhat, members/submit | **Done**: `scripts/contract-members.mjs` passes 180/180 (one key-order note, below) |
| 4 | Activity and club registrations | **Done**: covered by `scripts/contract-forms.mjs` |
| 5 | Custom forms, sessions, responses, uploads | **Done**: `scripts/contract-forms.mjs` passes 260/260 together with phase 4 |
| 6 | Courses and the remaining certificate routes | **Done**: `scripts/contract-courses.mjs` passes 181/181 |
| 7 | Remove the proxy, retire the Adonis deployment | Pending |

Every phase must pass the contract diff against Adonis for the routes it moves, plus
`make check` and `make test-unit`. It also ports the matching web-be Japa specs.

## Contract checks

`make test-contract` runs `scripts/contract-auth.mjs` (phase 1),
`scripts/contract-public.mjs` (phase 2), `scripts/contract-members.mjs` (phase 3) and
`scripts/contract-forms.mjs` (phases 4 and 5) and `scripts/contract-courses.mjs` (phase 6) on the
shared harness in `scripts/contract-lib.mjs`.
Each one does the following:

1. Creates an owned schema `web_go_<run>` in the test database and migrates it with Ace
   from `../kaderisasi-admin-be`.
2. Starts Adonis web-be and this service against that schema, with a synthetic `APP_KEY`
   and a local SMTP sink.
3. Sends the same requests to both, then compares status, content type, JSON body
   (including key order) and the resulting `public_users`/`profiles` rows.
4. Runs the Go integration tests (`-tags=integration`) inside the same schema.
5. Drops the schema, also on failure. A results summary is written to
   `.artifacts/contract-auth.json`.

It needs `../kaderisasi-web-be` with `node_modules`, `../kaderisasi-admin-be` with
`node_modules`, and `../env/test/*`. Ports 3346/3347 must be free.

Phase 1 coverage:
- every validation shape;
- JSON, form and query-string input;
- duplicate and mixed-case emails;
- a null password, a non-scrypt hash, and inactive accounts;
- tokens used across the two implementations;
- recovery email headers and HTML;
- reset links mailed by one side and accepted by the other.

Phase 2 coverage: synthetic fixtures for activities, clubs (rich text, media, registration
info and open/closed states), leadership roles, the monthly leaderboard and issued
certificates, including malformed snapshots. Requests cover:
- every pagination edge (0, negative, fractional, NaN, Infinity, repeated parameters);
- search wildcards and filters that fail in PostgreSQL;
- encoded route parameters;
- ISO month forms;
- the shared 30-per-minute certificate throttle.

Responses are compared byte for byte, including key order and error text.

Google sign-in is checked against Adonis for its validation and configuration answers.
The code exchange and ID-token checks are unit-tested against a fake Google
(`internal/google`), and `google_account.spec.ts` is ported as an integration test
(`internal/member`).

## Compatibility rules implemented in phase 0

- **Learner JWT** (`internal/auth/public.go`): HS256 with `APP_KEY`, payload
  `{userId, email, aud: "kaderisasi-public", iat, exp}`, 1-day expiry. Tokens are
  byte-identical to `@maximemrf/adonisjs-jwt`/`jsonwebtoken` output for the same inputs.
  `TestPublicAccessAdonisInteroperability` checks both directions through
  `scripts/crypto-fixture.cjs`.
- **Accepted tokens**: same as the Adonis guard (valid signature, `exp` checked when
  present, numeric `userId` of an existing `public_users` row; account status is not
  checked), with two deliberate tightenings:
  - only HS256 is accepted;
  - a token with a non-learner audience (for example an admin token) is rejected.
    Adonis ignores the audience and would read an admin id as a learner id.
  Legacy tokens without `aud` stay valid.
- **Token source**: a cookie named exactly `token` wins, otherwise the text after
  `Bearer ` in `Authorization`. Adonis's fallback regex also matched cookie names
  ending in `token` (for example `admin_refresh_token`), which could only cause false
  401s; that quirk is not reproduced.
- **401 from `auth`**: plain text `Unauthorized access`, `text/plain; charset=utf-8`.
  The JWT guard has no JSON renderer, so Adonis falls back to the plain message.
- **`courseAudience`**: `Cache-Control: private, no-store`. The bearer header token must
  have the learner audience, an `exp`, and the authenticated user's id; otherwise
  `401 {"message":"UNAUTHORIZED"}`.
- **CORS** (`config/cors.ts`: origin `*` with credentials, methods
  `GET,HEAD,POST,PUT,DELETE`, any requested header, max age 90). Go answers every
  preflight (`OPTIONS` with `Origin`, always 204). On simple requests it adds headers
  only to responses it produces itself, because proxied responses already carry
  Adonis's headers.
- **Request id**: a valid `X-Request-Id` (≤128 chars, `[A-Za-z0-9._:-]`) is kept,
  otherwise a UUID is issued. The id is forwarded to Adonis, which reuses it, and is sent
  back once.
- **Logging**: `>=400` warns and `>=500` errors, with `request_id`, `actor_user_id` and
  `proxied` fields. Successful requests log at debug level.
- **Proxy**: method, path, query, body, cookies and headers are forwarded. The incoming
  `X-Forwarded-For` chain is kept and the caller's address is appended. If Adonis is
  unreachable the response is `502 {"message":"LEGACY_API_UNAVAILABLE"}`.

## Deliberate deviations (phase 1)

- **Framework error bodies:** Adonis runs with `NODE_ENV=development` (the prod env file
  says so too), so bodyparser errors include `frames` with server stack traces. Go returns
  the same status, `message`, `name` and `status` without frames. For malformed JSON, V8's
  `SyntaxError` wording is reproduced only for a truncated document ("Unexpected end of
  JSON input"); other syntax errors say "Invalid JSON body".
- **500 `GENERAL_ERROR` bodies from database failures** carry Go/pgx error text, not
  knex's `insert into ... - <message>` text.
- **Nested form/query keys** (`a[b]=`, `a.b=`) are not expanded the way the `qs` library
  does. No auth route reads nested input.
- **`extra_data` (jsonb)** is passed through compacted. JavaScript's reordering of
  integer-like object keys is not reproduced.
- **Unicode edge cases:** a lone UTF-16 surrogate, and the few characters whose lower-case
  form differs between JavaScript and Go (e.g. `İ`).

## Phase 2 notes

- **Rich text:** `internal/richtext` ports htmlparser2 12 (tokenizer, parser, entity
  decoding), sanitize-html 2.17.6 with web-be's allowlist, launder's `naughtyHref`, and
  the postcss rules the `style` filter relies on. `TestSanitizeMatchesSanitizeHTML`
  compares it byte for byte with the Node library on handwritten cases plus 20,000
  generated inputs. Set `RICHTEXT_FUZZ=<n>` for a wider run with a random seed
  (200,000 inputs matched). The CSS emulation is not a full postcss: exotic style
  values (nested blocks mixed with declarations, comments inside words) could still
  differ. Only the `text-align` keyword values are ever kept.
- **Database error text** now matches knex (`<sql with $n> - <PostgreSQL message>`) for
  the phase 2 queries, including invalid ids and filters and a negative `per_page`. The
  phase 1 note about pgx error text still applies to the auth routes.
- **Route parameters** are read undecoded from the request target, as Adonis does
  (`%20SAL-0002` is not a valid code; `slug-with-%C3%BC` does not match `slug-with-ü`).
- **`month`** (monthly leaderboard) supports Luxon's common ISO forms: calendar,
  ordinal and week dates, an optional time, and `Z`/offset. Rarer Luxon inputs are
  treated as invalid, which (like Adonis) queries `month IS NULL`.
- **Repeated `category` / `club_id`** query parameters use the first value; knex would
  build an array comparison. The frontend never repeats them.
- **Throttle:** the same per-process buckets keyed by `request.ip()` (proxy-addr trusting
  loopback only), shared by both certificate routes. Behind Coolify's proxy every
  client still shares one bucket, as with Adonis today.

## Phase 3 notes

- **Twin schemas.** Mutating routes run against two owned schemas (`web_go_<run>_a` for
  Adonis, `_g` for Go) seeded identically, so database state is compared row by row after
  the same request sequence. Each server gets its own fake S3; object keys, content types,
  ACLs and deletions are compared.
- **Validation** (`internal/vine`, `internal/validators`): every web-be validator is ported
  and fuzzed against web-be's own modules (`scripts/webbe-validators-fixture.mjs`, 4,812
  cases), including the `vine.file` macro (size, then extension; ordered `meta`).
- **Multipart** (`internal/webapi/body.go`, `internal/filetype`): bodyparser's
  `{...query, ...body}` merge, empty strings to `null`, and file-type 20.5 detection
  (magic bytes for supported extensions, otherwise the name's extension; OOXML via
  `[Content_Types].xml`), checked against the Node library.
- **Profile updates** keep Lucid's dirty tracking (`birth_date` is always dirty;
  `updated_at` only moves when something changed) and knex's insert/update error text.
- **Known differences:**
  - Profile pictures are encoded with govips, not sharp: same limits (40 MP input),
    resize (512×512 inside), format (WebP q82) and key layout, but the bytes differ.
  - Adonis preloads `university`/`province`/`city` concurrently, so their key order in
    `GET /v2/profiles` varies between Adonis runs. Go always uses that declaration order.
  - Responses where Adonis serializes `error.stack` or framework 500 frames carry the
    same status and message without stack text.
  - V8's legacy (non-ISO) `Date` parsing is supported for the common forms only.

## Phase 4 and 5 notes

- **Registrations** keep web-be's transactions and row locks (activity `FOR UPDATE` on guest
  and form registration, registration `FOR UPDATE` on questionnaire edits and club
  cancellation, form `FOR UPDATE` before a club registration). A response returned inside a
  transaction still commits it, as with `db.transaction`. Club unique violations answer
  `409 ALREADY_REGISTERED`.
- **Forms** (`internal/forms`): submission validation, routing, `schemaHash` (ICU collation
  via `golang.org/x/text/collate`) and the attachment claim rules are ported and fuzzed
  against web-be's own modules (`scripts/forms-fixture.mjs`, `scripts/collate-fixture.mjs`).
  Standalone submissions validate `profile_data` sections like any other section.
- **Uploads**: `processFormFile` is ported. PDFs are checked by `internal/pdfcheck`, a
  lenient structural reader matched against pdf-lib (`scripts/pdf-fixture.mjs`); it is not a
  full PDF parser, so unusual files could still be judged differently. Images are converted
  with govips (2400×2400 inside, WebP q85): dimensions match sharp, bytes and sizes do not.
  Objects are written with `ACL: private` and `Cache-Control: private, no-store` under
  `custom-forms/<form id>/<uuid>.<ext>`, and orphaned objects are removed as in web-be.
- **Multipart** parsing follows `@poppinss/multiparty`: `name`/`filename` come from its
  `Content-Disposition` regexes (control characters allowed, `%22`/`\"` and `&#NNNN;`
  decoded, text after the last backslash). Go's `mime/multipart` is not used because it
  rejects such headers and strips directories from file names.
- **Session tokens** in the harness are random per server, so requests that carry one are
  sent with the token each server issued.
- **Known differences:** Adonis preloads a club registration's `club` and `roles`
  concurrently, so their key order varies; Go always writes `club` first. Uncaught errors
  answer with the same `message`, `name` and `status` but without debug `frames`.

## Phase 6 notes

- **Courses** keep `course_service.ts`'s queries (published, `minimum_level` against the
  member's profile level, soft-deleted lessons and documents excluded), its pagination
  (`page` ≤ 1,000,000, `per_page` ≤ 100, unescaped `ILIKE` search) and its
  `course_lesson_progress` upsert. Course ids must match `^[1-9]\d*$` and fit an int4;
  anything else is `404 COURSE_NOT_FOUND`. Documents stream from S3 as
  `attachment; filename="materi.pdf"; filename*=UTF-8''<encodeURIComponent(name)>`.
- **Owner certificate routes** (`registrations/:id`, `:id/download`, `code/:code/access`,
  `code/:code/download`) return the stored snapshots (`buildIssuedResponseData`) and keep
  the issued-only, revoked (`410`) and replacement-code (`current_code`) rules. Malformed
  template snapshots fail with V8's `TypeError` message, as the uncaught Adonis error does.
- With phase 6 every web-be route is served by Go; the proxy receives no known route.

## Behaviour kept on purpose (looks odd, but clients depend on it)

- `GET /v2/activities` answers with the key `messages` (not `message`).
- Club descriptions and registration info are sanitized; activity descriptions are not.
- The default monthly leaderboard queries today's date, not the first of the month.

- Auth validation failures answer **500**, with `{message: <first message>, error: [...]}`.
  `forgot-password` answers `{message: "GENERAL_ERROR", error: "Validation failure"}`.
- Login looks the email up after lower-casing it, so a stored mixed-case email cannot log
  in with a password; Google sign-in matches case-insensitively. Login does not check
  `account_status`.
- A password hash that is not scrypt simply fails with `401 WRONG_PASSWORD`.
- Logout only answers `LOGOUT_SUCCESS`; learner JWTs cannot be revoked.
- Reset links are reusable until they expire (30 minutes).

### Client IP note

Adonis trusts `X-Forwarded-For` only from loopback (its default `trustProxy`). Behind
Coolify's proxy, web-be already sees the proxy's address as `request.ip()`, so the
public certificate throttle is effectively shared across clients today. Routing through
web-api keeps that behavior. Phase 2 moved the throttle into Go with the same keying.
Making it per client (trusting the Coolify proxy's `X-Forwarded-For`) is a deliberate
behavior change for later; it needs the proxy's address range.

## Running locally

Adonis has to move off 3333 while web-api takes that port:

```sh
# kaderisasi-web-be
PORT=3335 npm run dev
# kaderisasi-web-be-go (reads ../env/test/web-be-test-env; WEB_BE_LEGACY_URL defaults to http://localhost:3335)
make run
```

`kaderisasi-web-fe` keeps using `http://localhost:3333/v2`.

## Deployment (Coolify)

Create a new application from this repository's `Dockerfile`. Since phase 3 the binary
links libvips (CGO); the Dockerfile builds the same pinned, minimal libvips 8.18.6 as the
admin API.

- `PORT=3333`, `HOST=0.0.0.0`; health check `GET /health`;
- the web-be environment (`env/prod/web-be-prod-env`, including the `DRIVE_*` S3 keys), plus `WEB_BE_LEGACY_URL` set to
  the Adonis service's internal URL;
- the public web-be domain moved from the Adonis application to this one, while Adonis
  stays reachable internally.

Rollback: point the domain back at the Adonis application. No data migration is involved.
