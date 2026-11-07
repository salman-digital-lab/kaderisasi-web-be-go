# Member email change

Members change their account email from the profile page of `kaderisasi-web-fe`.
Admins change it directly from the admin dashboard (Anggota → detail → "Ubah email",
`PUT /v2/profiles/auth/:id` in `kaderisasi-admin-be-go`); an admin change applies at
once, sends no email and cancels any pending member request.

## Flow

1. `POST /v2/profiles/email-change` `{email, password?}` (logged in).
   - Member **with a password**: the password is required. A confirmation link is
     emailed to the **new** address → `EMAIL_CHANGE_LINK_SENT`, `step: "link"`.
   - Member **without a password** (Google-only): a 6-digit code is emailed to the
     **current** address → `EMAIL_CHANGE_CODE_SENT`, `step: "code"`.
2. Code step only: `POST /v2/profiles/email-change/verify-code` `{code}`. A correct
   code emails the confirmation link to the new address (`EMAIL_CHANGE_LINK_SENT`).
   A wrong code answers `INVALID_CODE` with `data.remaining_attempts`; the 5th wrong
   code cancels the request (`TOO_MANY_ATTEMPTS`).
3. The member opens `<host of RESET_PASSWORD_URL>/confirm-email?token=…` and clicks
   "Konfirmasi email": `PUT /v2/auth/confirm-email-change?token=…` (no session needed).
   The email changes, a notice goes to the old address, and the answer has the same
   shape as login (`EMAIL_CHANGE_SUCCESS`, `data: {user, data, token}`), so the site
   signs the member in with a fresh session. The notice is sent in the background.

Also: `GET /v2/profiles/email-change` → `{password_required, pending: null | {step,
email, expires_at}}`, and `DELETE /v2/profiles/email-change` cancels the open request.

Login keeps matching accounts by email; no Google account id is stored. After a
change, Google sign-in works with the new email only. Signing in with Google on the
old Gmail creates a new, separate account. Both the member dialog and the admin
dialog say so.

## Rules

| Rule | Value |
| --- | --- |
| Code lifetime | 15 minutes |
| Link lifetime | 60 minutes |
| Wrong codes before cancelling | 5 |
| Wait before starting a new request | 1 minute (`EMAIL_CHANGE_TOO_SOON`, 429) |
| Requests per member per hour | 5 (`EMAIL_CHANGE_LIMIT`, 429) |
| Open requests per member | 1 (a new request cancels the previous one) |

Cancelled requests count toward the wait and the hourly limit: every request emails
an address the member typed, so cancelling must not allow more emails.

Active members can change their email, and so can `no_account` members who have an
email, because Google sign-in lets them in too.

Emails are trimmed and lowercased. They must be free ignoring case (unique index
`public_users_lower_email_unique`). The new email is checked again at confirmation
under the same advisory lock Google sign-in uses (`EMAIL_ALREADY_REGISTERED`). The
link only works while the account still has the email it had when the request
started, so an admin change in the meantime voids it (`INVALID_TOKEN`).

Only hashes are stored: SHA-256 of the link token, HMAC-SHA256 (APP_KEY) of the code.

Refusal codes: `INVALID_EMAIL` (400), `EMAIL_UNCHANGED` (400), `WRONG_PASSWORD` (401),
`ACCOUNT_INACTIVE` (403), `EMAIL_ALREADY_REGISTERED` (409), `EMAIL_CHANGE_TOO_SOON`
(429), `EMAIL_CHANGE_NOT_FOUND` (404), `INVALID_CODE` (400), `CODE_EXPIRED` (400),
`EMAIL_CHANGE_LIMIT` (429), `TOO_MANY_ATTEMPTS` (429), `INVALID_TOKEN` (400),
`SEND_EMAIL_FAILED` (500, the request is withdrawn; the member can retry after the
1-minute wait).

The notice to the old email is sent in the background after the confirmation
answers. If the fresh session cannot be built after the email changed, the answer is
still `EMAIL_CHANGE_SUCCESS`, with `data.user` only; the member signs in again.

## Data

Table `public_user_email_changes` (Ace migration
`1794035765462_create_public_user_email_changes_table`) and the unique index on
`lower(public_users.email)` (`1794035764157_add_public_users_lower_email_unique_index`,
built concurrently; it first lowercases stored emails and stops if two accounts differ
only by case).

## Verification

```sh
set -a; . ../env/test/web-be-test-env; set +a
go test -tags=integration -count=1 ./internal/emailchange/   # owned email_go_<run> schema
go test -count=1 -run EmailChange ./internal/webapi/ ./internal/mail/
```

## Production deployment (7 November 2026)

- Ace migrations `1794035764157_add_public_users_lower_email_unique_index` and
  `1794035765462_create_public_user_email_changes_table` applied to production
  (no email needed lowercasing; no duplicates). Run through the database's public
  proxy port because the internal host is not reachable from a workstation.
  Index valid, table empty afterwards.
- Deployed `kaderisasi-admin-be-prod` at `787d560`, `kaderisasi-web-be-prod` at
  `b084cde`, then `kaderisasi-admin-fe-prod` at `69d2bde` and
  `kaderisasi-web-fe-prod` at `d675e7e` (manual deploys). `kaderisasi-mcp-prod` at
  `9f9d6d0`.
- Read-only smoke checks: both `/health` 200; `GET /v2/profiles/email-change` without
  a session 401; `PUT /v2/auth/confirm-email-change` with a bad token 400
  `INVALID_TOKEN`; `/confirm-email` page 200; dashboard serves the new email dialog.
  No member email was changed during the checks.
