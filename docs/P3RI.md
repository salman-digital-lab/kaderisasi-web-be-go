# P3RI committee registration (Open Recruitment 1448 H)

Members sign in on **p3ri.salmanitb.com** (`../p3ri-salmanitb`) with their
Kaderisasi account (`POST /v2/auth/login`, Bearer token), fill the P3RI 1448 H
committee form, and track their status. Admins review registrations in
Kaderisasi admin (`../kaderisasi-admin-be-go/docs/P3RI.md`).

This is a fast, temporary feature: one table and a hardcoded form.

## Data

`p3ri_registrations` (Ace migration
`1793195171058_create_p3ri_registrations_table`): one row per member
(`user_id` unique), `status` (`terdaftar`, `seleksi_berkas`, `wawancara`,
`diterima`, `tidak_diterima`), ordered `choices` (1–3 division codes),
`accepted_division` (set exactly when `diterima`, one of the choices),
`answers` (`{"common": {<question id>: …}, "divisions": {<code>: {<question id>: …}}}`),
`files` (`[{field, key, name, mime, size}]`), and admin-only `admin_note`,
`status_updated_by`, `status_updated_at`.

## Form

`internal/p3ri/form_1448h.json` is the whole form, extracted once from the
Google Form "Open Recruitment P3RI Salman ITB 1448 H": common `sections`,
the two `choice_questions` asked for every chosen division, the 42 `divisions`
with their own questions, and `opens_at`/`closes_at`. It is not stored in the
database. `../kaderisasi-admin-be-go/internal/p3ri/form_1448h.json` is a
byte-identical copy used for labels and the export: **change both together**.
To move the deadline, edit `closes_at` in both copies and redeploy.

Answer values by question type: `short`, `long`, `date` (`YYYY-MM-DD`), `radio`
and `dropdown` are strings (any text when `other` is true); `checkbox` is a string
array; `scale` is an integer `min`–`max`; `grid` maps each row to a column; `file`
is sent as the upload's `{key, name, mime, size}` and stored as its key.

## Routes (`internal/webapi/p3ri_routes.go`)

| Route | Auth | Notes |
| --- | --- | --- |
| `GET /v2/p3ri/form` | public | Form plus `is_open` |
| `GET /v2/p3ri/registration` | member | Own registration or `data: null`; `editable` while open and `terdaftar` |
| `PUT /v2/p3ri/registration` | member | `{choices, answers:{common, divisions}}`; create or replace; 422 on invalid answers, 409 when closed or already processed |
| `POST /v2/p3ri/uploads` | member | `{name, mime, size, dataUrl}`; JPG/PNG/WebP up to 5 MB; private at `p3ri/1448h/<userId>/<hex>-<name>` |
| `GET /v2/p3ri/files?key=` | member | Only the member's own keys |

Responses are `{"message":"OK","data":…}`; failures are `{"message":<Indonesian>}`.

## Verification

```sh
make check
make test-unit
set -a; . ../env/test/web-be-test-env; set +a
go test -tags=integration -count=1 ./internal/p3ri/
```

The integration test creates and drops its own `p3ri_go_<run>` schema.
