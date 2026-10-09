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
├── api_tokens.go                # API token admin (/api/tokens), session only
├── api_qbit_compat.go           # qB-compatible entrance settings (/api/qbit-compat), session only
├── qbitcompat/                  # qB WebUI API v2 subset on its own port (M13): SID login with qbit:compat tokens
├── api_app_v1*.go               # App API v1: route table with scopes, principals, audit, DTOs
├── mcp.go                       # /mcp (M14): Bearer-token MCP endpoint; mcpBackend calls App API v1 routes in-process
├── remote.go / api_remote.go    # Remote access (M15): /remote/v1/stream, device dispatcher (App API only), /api/remote* (session only)
├── middleware/principal.go      # Request principal (session / api_token / mcp / remote_device), Bearer parsing
├── frontend/                    # Vue application source
└── static/                      # Built assets embedded by Go
```

## Server and Auth

`Server` holds `ConfigStore`, `scheduler.Manager`, in-memory browser sessions, optional ChatOps dependencies, a QA hook, and the live `http.Server`.

- `/login`, `/logout`, and `/api/ping` are public; normal APIs use `s.auth`.
- Browser sessions are in memory and intentionally expire on restart.
- Default admin credentials come from `PT_ADMIN_USER`/`PT_ADMIN_PASS`; `PT_ADMIN_RESET=1` performs a startup reset.
- Extension origins receive narrowly scoped CORS handling in `logMiddleware`.
- ChatOps routes are registered only when dependencies were injected, and are session-only like every other `/api/*` route.
- API tokens (`internal/apitoken`) are accepted only by the App API (`/api/app/v1/*`), `/mcp` and the qB-compatible entrance. Token management (`/api/tokens`) and every other route stay session-only through `s.auth`; `TestAppAPI_TokensCannotReachSessionAPIs` pins this through the real mux.
- Remote devices never pass through `buildHandler`: `RemoteDispatcher` mounts only the App API routes and puts the device principal under the unexported `remoteDeviceKey`, which `appPrincipal` checks first. `statusRecorder` must keep `Unwrap()` so the direct WebSocket entrance can hijack the connection through `logMiddleware`.
- `Shutdown(ctx)` must remain safe before/concurrent with `Serve`; a `Shutdown` that runs before `Serve` makes the later `Serve` return without listening.

## Adding a Route

1. Put handlers in `api_<feature>.go`.
2. Register exact paths in `Server.Serve()` or a cohesive `register*Routes` helper.
3. Register specific paths before catch-all prefixes such as `/api/sites/` and `/api/torrents/`.
4. Wrap protected routes with `s.auth`. App API routes go in `appRoutes()` with a scope instead (`app:read` for reads, `app:write` for writes; `TestAppAPI_RouteTable` checks); never accept tokens on an `s.auth` route.
5. Validate method, path values, JSON shape/unknown fields, and body limits.
6. Return explicit status codes and stable JSON errors.
7. Add `httptest` coverage through the real mux when routing precedence matters.

Go 1.22+ path patterns and `r.PathValue` are used for some ChatOps routes; do not manually parse those paths inconsistently.

## Sensitive Data

- Site list/detail DTOs redact cookie plaintext/ciphertext and expose `has_cookie`. The authenticated settings DTO currently includes API key/passkey fields; do not reuse it for a broader/public surface.
- Credential changes go through `ConfigStore` encryption and then refresh registered site instances. Never return downloader passwords or AES keys.
- Notification list DTOs redact `ConfigJSON`; the authenticated notification-detail endpoint intentionally decrypts it for editing. Do not cache, log, or expose that detail through other route classes.
- Sanitized errors may be returned, but upstream response bodies and signed tracker URLs must not leak.
- App API DTOs (`App*` types) carry no cookies, API keys, passkeys, passwords, RSS addresses or download/detail links; error text goes through `appRedact`. Every App handler test runs `assertNoSecrets` on its response.
- Non-session writes on the App API are audited into `ActionAudit` (`channel_type` = principal kind, result `success` / `error:http_<status>` / `denied:scope`), using the same result vocabulary as ChatOps so the audit page can classify them.

## Important Route Families

| Prefix                                                 | Purpose                                         |
| ------------------------------------------------------ | ----------------------------------------------- |
| `/api/sites`, `/api/sites/{name}`                      | Site config and credentials                     |
| `/api/sites/{name}/login-state/*`                      | Probe/config/visit login monitoring             |
| `/api/v2/search/*`, `/api/v2/userinfo/*`               | Multi-site search and statistics                |
| `/api/downloaders*`, `/api/downloader-torrents*`       | Client settings and torrent hub                 |
| `/api/v2/torrents/*`, `/api/torrents/*`, `/api/site/*` | Push/download/manage torrents                   |
| `/api/filter-rules`, `/api/rss/*`                      | Filtering and RSS associations                  |
| `/api/chatops/*`                                       | Channels, bindings, audit, RSS delivery logs    |
| `/api/tokens`, `/api/app/v1/*`, `/mcp`                 | API token admin; App API and MCP for tokens     |
| `/api/qbit-compat`                                     | qB-compatible entrance settings and status      |
| `/api/remote*`, `/remote/v1/stream`                    | Remote access settings/devices; direct entrance |
| `/api/cloak/*`, `/api/extension-actions/*`             | Cloak and extension integration                 |
| `/api/maintenance/clean`                               | Preview/confirmed maintenance cleanup           |
| `/api/version/*`                                       | Check/runtime metadata/self-upgrade             |

## Frontend

The SPA uses Vue 3, Vue Router hash history, Pinia, Element Plus, TypeScript, and external CSS files under `web/frontend/src/styles/`. API contracts live in `src/api/index.ts`; update them with backend DTO changes. ChatOps screens live under `src/views/chatops/`.

### Every list or table page owes two contracts

Both are design requirements, not polish. `src/views/TaskList.vue` is the reference implementation.

1. **Six data states** — `loading / empty / zero / error / partial / perm`. Drive them with
   `useDataState` (`src/composables/useDataState.ts`) and render with `PtDataState`. A failed load
   must leave the error on the page: a toast alone plus an `empty` table tells the user the data is
   gone rather than unfetched. 401/403 resolve to `perm` and get no retry button. `partial` (some
   of several sources failed) shows a note above the still-usable rows, and only becomes the main
   state when nothing was fetched — so aggregating endpoints must report per-source failures
   instead of skipping silently (see `DownloaderFailure` in `api_downloader_torrents.go`).
2. **Mobile row cards** — below 768px no desktop table may render and nothing may scroll
   horizontally. Keep the table behind `v-if="!isMobile"` (`useIsMobile`) and add a `v-else` list of
   `PtRowCard` (`lead / title / meta / status / progress / actions`). Do not delete desktop columns;
   the cards are a second view. Touch targets are ≥44px.

### Page layout follows the Penpot boards, and the boards use two shapes

The main area (x=328, width 1112 at a 1440 canvas) is either **bands** or **cards**, never a padded
canvas of floating panels. Pick the shape the page's board uses; `.pt-shell__inner` has no padding of
its own, so a page that wraps nothing sits flush against the shell.

- **Band pages** (`userinfo`, `sites`, `tasks`, `search`, `filter-rules`, `chatops/audit`,
  `chatops/rss-notifications`, `supported-sites`) stack full-bleed siblings:
  head 64 → `PtToolbar band` 40 → `.pt-band--grid` → `.pt-band--sel` 44 → `.pt-band--foot` 34 →
  cards. The toolbar is a **sibling** of the grid, not a child of it.
- **Card pages** (everything else) go straight from the head into a `.pt-cards` grid, which supplies
  the 16px inset and the 16px gutter. Column widths are not equal — use the variant the board uses:
  `--wide` 1080 · `--2` 548/516 · `--main` 700/364 · `--side` 300/764 · `--rail` 276/788 ·
  `--3` 364/340/344 · `--narrow` centred 520 · `--lead` left-aligned 612. `.pt-cards__full` spans all
  columns. All collapse to one column below 1181px.

The head is the shell's, not the page's. Pages fill it through two teleport targets and must disable
the teleport on mobile, where the head is hidden:

```html
<Teleport to="#pt-head-sub">{{ headSub }}</Teleport>
<Teleport to="#pt-head-acts" :disabled="isMobile">…</Teleport>
```

`#pt-head-sub` is a live summary of real numbers (`37 个任务 · 12 下载中 · ↓93.8 MB/s`), not a
breadcrumb — breadcrumbs belong to detail routes only, where they read `<list page> › <this record>`.
Two routes own their top band instead of taking the shell head (`App.vue`'s `OWN_TOP_ROUTES`):
`userinfo` puts a `PtKpiBar` there, `search` an 88-high `.pt-band--head`.

Styling slot content from a shared component needs `:slotted(...)` — a plain descendant selector in
the component's scoped block silently matches nothing.

`vue-tsc` does **not** report a component used in a template but never imported. Vue renders it as a
literal unknown element (`<ptpanel>`), so the card's header, footer and border silently vanish while
the build stays green. `src/views/views.components.test.ts` scans every SFC for this; keep it green.

Build output must exist for production compilation. `pnpm build` runs `vue-tsc -b` and is the real
type gate; `vue-tsc --noEmit` has been observed to pass errors the build rejects:

```bash
pnpm --dir web/frontend test
pnpm --dir web/frontend build
```

`make embed-placeholder` is a static-analysis escape hatch only and must never supply release assets.

## Tests

Use `setupTestServer`, `httptest.NewRecorder`, real `ServeMux` registration for route tests, and explicit fake downloaders/sites. Verify auth, methods, routing precedence, secret redaction, error status, and mutation side effects. Avoid network access in ordinary unit tests.
