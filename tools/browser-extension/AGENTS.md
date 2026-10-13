# `tools/browser-extension/` — PT Tools Helper Extension

## Role

The MV3 companion extension recognizes tracker pages, reads permitted cookies, captures/sanitizes site metadata, syncs credentials/activity to pt-tools, and surfaces login-state reminders and pending actions.

## Layout

```text
tools/browser-extension/
├── src/manifest.ts              # Permissions, host permissions, build metadata
├── src/background/index.ts      # Runtime messaging and sync orchestration
├── src/content/index.ts         # Page-side collection entrypoint
├── src/core/
│   ├── constants.ts             # KNOWN_SITES and domain/schema metadata
│   ├── storage.ts / types.ts    # Persisted settings and contracts
│   ├── permissions.ts           # Optional-host permission handling
│   └── messages.ts              # Cross-context message protocol
├── src/modules/
│   ├── collector/               # Detect/capture/sanitize tracker metadata
│   ├── sync/                    # pt-tools HTTP client, cookies, errors
│   ├── cookiecloud/             # CookieCloud crypto (MD5 + WebCrypto AES-CBC), upload/merge, settings store
│   └── export/                  # ZIP/GitHub export helpers
└── src/popup/                    # Vue popup and login-status/settings panels
```

## Site Synchronization

Go definitions and extension `KNOWN_SITES` are a checked invariant. When adding or changing a site:

1. Update `src/core/constants.ts` with the ID, domains, schema, and display metadata.
2. Keep optional host permissions consistent with supported domains.
3. Run `make check-sites`; extension packaging fails on ID drift.
4. Add detection/sanitization fixture tests when markup behavior changes.

Do not add an extension-only built-in site without a Go definition.

## Security

- Request only the narrow host/cookie permissions needed for enabled sites.
- Never send tracker cookies anywhere except the user-configured pt-tools endpoint and the user-configured CookieCloud server; CookieCloud only ever gets ciphertext (the password stays in the browser), the upload merges into the existing data and refuses to overwrite data it cannot decrypt unless the user confirms. Keep the crypto vectors in `modules/cookiecloud/crypto.test.ts` and `internal/cookiecloud` `TestDecryptExtensionVectors` identical.
- Settings messages (`SAVE_*`, `UPLOAD_COOKIECLOUD`, `RUN_SYNC_NOW`, `GET_SYNC_SETTINGS`) are refused from content scripts (`requireExtensionPage`); the popup never receives the stored CookieCloud password.
- Sanitize captured HTML/metadata before persistence/export.
- Do not log cookies, bearer tokens, API keys, or full authenticated URLs.
- Treat the pt-tools base URL and credentials as untrusted input; use the shared timeout/error helpers.
- Keep background/content/popup messages typed and validate incoming actions.

## API and Login Monitoring

`modules/sync/api-client.ts` is the backend boundary. Keep paths and DTOs aligned with `web/api_credential.go`, `web/api_site_login.go`, and extension-action routes. Manual probe and visit reporting must preserve the backend's per-site single-flight behavior and handle conflict responses without retry storms.

## Validation

```bash
make check-sites
pnpm --dir tools/browser-extension typecheck
pnpm --dir tools/browser-extension test
pnpm --dir tools/browser-extension build
make build-extension
```

The extension version is sourced from `package.json`; do not maintain a second literal in the manifest code.
