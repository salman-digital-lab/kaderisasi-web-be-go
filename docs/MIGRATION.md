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
| 0 | Service, config, request id/logging, CORS, learner JWT, auth middleware, proxy, `/health` | **Done**: proxies every `/v2` route |
| 1 | `auth/*` (register, login, Google, check-email, forgot/reset password with an SMTP mailer, logout) | Pending |
| 2 | Public reads: activities, categories, clubs, monthly achievements, certificate verify/show (with throttle) | Pending |
| 3 | Profiles, picture, history, notifications, achievements, ruang-curhat, members/submit | Pending |
| 4 | Activity and club registrations | Pending |
| 5 | Custom forms, sessions, responses, uploads | Pending |
| 6 | Courses and the remaining certificate routes | Pending |
| 7 | Remove the proxy, retire the Adonis deployment | Pending |

Every phase must pass the contract diff against Adonis for the routes it moves, plus
`make check` and `make test-unit`. It also ports the matching web-be Japa specs.

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

### Client IP note

Adonis trusts `X-Forwarded-For` only from loopback (its default `trustProxy`). Behind
Coolify's proxy, web-be already sees the proxy's address as `request.ip()`, so the
public certificate throttle is effectively shared across clients today. Routing through
web-api keeps that behavior. Phase 2 moves the throttle into Go and should key it on the
real client address.

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

Create a new application from this repository's `Dockerfile`. It needs no libvips and
runs a static binary.

- `PORT=3333`, `HOST=0.0.0.0`; health check `GET /health`;
- the web-be environment (`env/prod/web-be-prod-env`), plus `WEB_BE_LEGACY_URL` set to
  the Adonis service's internal URL;
- the public web-be domain moved from the Adonis application to this one, while Adonis
  stays reachable internally.

Rollback: point the domain back at the Adonis application. No data migration is involved.
