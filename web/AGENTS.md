# `web/` — HTTP Server, APIs, and Vue SPA

## Role

`web` exposes authenticated JSON APIs and an embedded Vue SPA using stdlib `http.ServeMux`. It receives already-wired services from `cmd/web.go`; it should not become the composition root.

## Backend Layout

```text
web/
├── server.go                    # Server, auth, middleware, route table, SPA embed
├── routes_chatops.go            # Conditional ChatOps route registration
├── api_chatops.go               # Notification/binding/audit/RSS-log APIs
├── api_site.go                  # Site CRUD/definitions/templates/validation
├── api_site_login.go            # Login state/probe/config/visit routes
├── api_credential.go            # Credential update boundary
├── api_cloak.go                 # CloakBrowser settings/connectivity
├── api_extension_actions.go     # Browser-extension pending-action routes
├── api_search.go / api_userinfo.go
├── api_downloader*.go           # Settings, directories, hub/actions/stats
├── api_torrent_*.go             # Download, push, management
├── api_filter_rule.go / api_rss_filter.go
├── api_maintenance.go           # Authenticated cleanup preview/confirm
├── api_version.go / api_log_level.go / api_favicon.go / api_levels.go
├── middleware/bearer.go         # Scoped bearer-token middleware building block
├── frontend/                    # Vue application source
└── static/                      # Built assets embedded by Go
```

## Server and Auth

`Server` holds `ConfigStore`, `scheduler.Manager`, in-memory browser sessions, optional ChatOps dependencies, a QA hook, and the live `http.Server`.

- `/login`, `/logout`, and `/api/ping` are public; normal APIs use `s.auth`.
- Browser sessions are in memory and intentionally expire on restart.
- Default admin credentials come from `PT_ADMIN_USER`/`PT_ADMIN_PASS`; `PT_ADMIN_RESET=1` performs a startup reset.
- Extension origins receive narrowly scoped CORS handling in `logMiddleware`.
- ChatOps routes are registered only when dependencies were injected.
- `Shutdown(ctx)` must remain safe before/concurrent with `Serve`.

## Adding a Route

1. Put handlers in `api_<feature>.go`.
2. Register exact paths in `Server.Serve()` or a cohesive `register*Routes` helper.
3. Register specific paths before catch-all prefixes such as `/api/sites/` and `/api/torrents/`.
4. Wrap protected routes with `s.auth`; use bearer middleware only for an intentionally token-scoped surface.
5. Validate method, path values, JSON shape/unknown fields, and body limits.
6. Return explicit status codes and stable JSON errors.
7. Add `httptest` coverage through the real mux when routing precedence matters.

Go 1.22+ path patterns and `r.PathValue` are used for some ChatOps routes; do not manually parse those paths inconsistently.

## Sensitive Data

- Site list/detail DTOs redact cookie plaintext/ciphertext and expose `has_cookie`. The authenticated settings DTO currently includes API key/passkey fields; do not reuse it for a broader/public surface.
- Credential changes go through `ConfigStore` encryption and then refresh registered site instances. Never return downloader passwords or AES keys.
- Notification list DTOs redact `ConfigJSON`; the authenticated notification-detail endpoint intentionally decrypts it for editing. Do not cache, log, or expose that detail through other route classes.
- Sanitized errors may be returned, but upstream response bodies and signed tracker URLs must not leak.

## Important Route Families

| Prefix                                                 | Purpose                                      |
| ------------------------------------------------------ | -------------------------------------------- |
| `/api/sites`, `/api/sites/{name}`                      | Site config and credentials                  |
| `/api/sites/{name}/login-state/*`                      | Probe/config/visit login monitoring          |
| `/api/v2/search/*`, `/api/v2/userinfo/*`               | Multi-site search and statistics             |
| `/api/downloaders*`, `/api/downloader-torrents*`       | Client settings and torrent hub              |
| `/api/v2/torrents/*`, `/api/torrents/*`, `/api/site/*` | Push/download/manage torrents                |
| `/api/filter-rules`, `/api/rss/*`                      | Filtering and RSS associations               |
| `/api/chatops/*`                                       | Channels, bindings, audit, RSS delivery logs |
| `/api/cloak/*`, `/api/extension-actions/*`             | Cloak and extension integration              |
| `/api/maintenance/clean`                               | Preview/confirmed maintenance cleanup        |
| `/api/version/*`                                       | Check/runtime metadata/self-upgrade          |

## Frontend

The SPA uses Vue 3, Vue Router hash history, Pinia, Element Plus, TypeScript, and external CSS files under `web/frontend/src/styles/`. API contracts live in `src/api/index.ts`; update them with backend DTO changes. ChatOps screens live under `src/views/chatops/`.

Build output must exist for production compilation:

```bash
pnpm --dir web/frontend test
pnpm --dir web/frontend build
```

`make embed-placeholder` is a static-analysis escape hatch only and must never supply release assets.

## Tests

Use `setupTestServer`, `httptest.NewRecorder`, real `ServeMux` registration for route tests, and explicit fake downloaders/sites. Verify auth, methods, routing precedence, secret redaction, error status, and mutation side effects. Avoid network access in ordinary unit tests.
