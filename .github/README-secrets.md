# Required secrets for GitHub Actions

Set these in repo Settings → Secrets and variables → Actions:

| Secret               | Value                                                                  |
| -------------------- | ---------------------------------------------------------------------- |
| `TELEGRAM_BOT_TOKEN` | Bot API token from @BotFather (release announcer)                      |
| `TELEGRAM_CHAT_ID`   | Target group chat_id (negative integer for supergroups)                |
| `CRX_PRIVATE_KEY`    | Base64-encoded RSA private key for signing browser extension `.crx`    |

`GITHUB_TOKEN` is auto-provided.

The bot must be added to the target group AND granted **"Pin messages"** admin permission. Without that permission the message is still sent, but pinning silently warns and the workflow stays green.

To test without a real release, trigger `Telegram Release Announcement` workflow via Actions → Run workflow → enter tag (e.g. `v0.31.0`).

## Setting up CRX signing for browser extension releases

The `release-please.yml` workflow optionally produces a signed `pt-tools-helper.crx`
artifact alongside the existing `pt-tools-helper.zip`. The CRX is signed with a
**stable** RSA private key — it MUST remain identical across releases, otherwise
users who installed via `.crx` will lose their auto-update path (Chrome treats a
key change as a different extension).

### One-time setup

1. Generate a stable RSA private key (do this **once**, never regenerate):

   ```bash
   openssl genrsa -out crx-private.pem 2048
   base64 -w 0 crx-private.pem > crx-private.pem.b64
   ```

2. Add the contents of `crx-private.pem.b64` as a repo secret named `CRX_PRIVATE_KEY`
   (Settings → Secrets and variables → Actions → New repository secret).

3. **Keep `crx-private.pem` somewhere safe** (e.g. 1Password, a hardware key, or
   another offline vault). If you lose it, all users who installed the extension
   from the signed `.crx` lose their auto-update path and must reinstall.

4. Once the secret exists, every release-please release run will decode it,
   sign the freshly built extension, and attach `pt-tools-helper.crx` to the
   GitHub Release assets.

### Behavior when the secret is missing

The CRX build steps are guarded with `if: ${{ env.CRX_PRIVATE_KEY != '' }}` —
if the secret is unset, those steps are skipped silently, the workflow stays
green, and only the unsigned `.zip` is uploaded as before. No release is broken
by a missing key.
