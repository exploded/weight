# weight — personal weight tracker

Single-page weight tracker: JSON API to record readings, Chart.js graph with
date-range filters and summary stats, Discord notification on each new reading.
Live at https://weight.mchugh.au.

After any UI change, verify it in the browser (use the chrome-devtools MCP tools if
available) before reporting done. Do not claim a fix works without exercising the flow.

## Stack
Go 1.26, stdlib `net/http`, html/template, modernc.org/sqlite (pure Go) with
**hand-written SQL in `db.go` — no sqlc**. Frontend is vanilla JS + self-hosted
Chart.js (**no HTMX**, so the go-htmx-skill patterns mostly don't apply here).
`exploded/monitor` logship: use `slog`, not `log` — WARN+ ships to the monitor
dashboard.

## Build / test / run (Windows)
```
build.bat        # go build -o weight.exe, creates .env from .env.example, runs it
run.bat          # runs existing weight.exe with .env loaded
go test ./...    # weight_test.go
```
Env (`.env`): PORT (prod 8990), PROD, DB_PATH, WEIGHT_API_KEY (required for
posting readings), DISCORD_WEBHOOK_URL (optional), MONITOR_URL/MONITOR_API_KEY
(optional).

## Layout
Flat `package main`: `weight.go` (entry, routes, middleware), `db.go` (SQLite),
`weight_test.go`, `templates/index.html`, `static/` (script.js, styles.css,
chart.min.js + date adapter).

Routes: `GET /` page, `POST /api/weight` (JSON `{"weight_kg": …}`),
`GET /api/weights`.

## Deployment
- Repo: `exploded/weight`, default branch `master`. Push to `master` → Actions
  "Test and Deploy" (test, then `CGO_ENABLED=0 GOOS=linux GOARCH=amd64` build).
- Bundle = binary + `favicon.ico` + `templates/` + `static/` + deploy script —
  **assets are NOT embedded**; template/static changes still need a deploy.
- Prod: systemd unit `weight`, port **8990**, `/var/www/weight`, Caddy at
  https://weight.mchugh.au. Provisioning: `scripts/server-setup.sh`.

## Gotchas (verified in code / git history)
- **Startup prunes the DB**: for each date only the lowest reading is kept —
  `db.go` runs a `DELETE` on every boot (c611bce). Don't store multiple readings
  per day and expect them to survive a restart. There is deliberately no delete
  button in the UI (removed in c611bce).
- `POST /api/weight` needs `Authorization: Bearer <WEIGHT_API_KEY>`: 401 if the
  key is missing or wrong, 503 if `WEIGHT_API_KEY` is unset. The scale posts over
  **plain HTTP** (Caddy's `http://weight.mchugh.au` block), so the key travels in
  clear. Keep it in `Authorization`: Caddy's JSON access log records every other
  header (and the URI) and ships to monitor, but blanks `Authorization`.
  `GET /api/weights` and the dashboard stay public.
- Static asset URLs get a `?v=<startup-unix-time>` cache-buster (`assetVersion`
  in `weight.go`, commit 63c19aa) — new JS/CSS shows up after redeploy without
  manual cache clearing; keep using it for any new asset links.
- CSP allows only `self` + Cloudflare Insights; adding external scripts means
  editing `securityHeaders` in `weight.go`.
- Never commit `.env` or `*.db*` (gitignored, 629c94c).
