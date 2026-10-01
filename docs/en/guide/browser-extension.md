# Browser extension

This page covers installing the PT Tools Helper browser extension and its two jobs: syncing a site's cookie to pt-tools, and collecting pages to request support for a new site.

## Installing it

| Option                     | Notes                                                                                                                                                                                                                                                                                     |
| -------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Edge Add-ons (recommended) | Search for "PT Tools Helper" on [Edge Add-ons](https://microsoftedge.microsoft.com/addons/detail/pt-tools-helper/pgicnjkmgenmjfhlclodbpbedjmojbea). It updates itself; each release waits about a week for store review. Chrome and other Chromium browsers can install it from there too |
| Load it yourself           | Download `pt-tools-helper.zip` from the [releases page](https://github.com/sunerpy/pt-tools/releases) and unpack it to a folder you keep. In `chrome://extensions` or `edge://extensions`, turn on Developer mode and choose Load unpacked. It does not update itself                     |
| Signed Chrome package      | `pt-tools-helper.crx` on the same releases page                                                                                                                                                                                                                                           |
| Firefox                    | In `about:debugging#/runtime/this-firefox`, choose Load Temporary Add-on and pick `manifest.json` in the unpacked folder. It has to be loaded again after Firefox restarts                                                                                                                |

The extension follows the browser's language; the labels below are its English ones, with the Chinese labels in brackets. The first time you click the extension's icon, choose 🔓 Grant & Enable (授权并启用) to grant the permissions it needs.

## Connecting it to pt-tools

Under Global Settings (全局设置) at the bottom of the extension, enter the pt-tools URL (for example `http://192.168.1.100:8080`), add your pt-tools user name and password if needed, and choose 🔗 Test Connection (连接测试). The address must be reachable from the browser you are using.

## Syncing cookies

For sites pt-tools already supports that sign in with a cookie.

- **One site**: sign in to the site, click the extension's icon, check that it names the site and shows the cookie as Valid (有效), then choose 🔄 Sync Cookie to pt-tools (同步 Cookie 到 pt-tools).
- **Several sites**: tick them under Batch Cookie Sync (批量同步 Cookie) in Global Settings and choose 🔄 Sync Selected (同步已选).
- **Automatically**: turn on Auto-sync Cookie (自动同步 Cookie), and the cookie is synced whenever you sign in again or it changes.

Sites that use an API key or a passkey (M-Team, Rousi Pro) cannot be synced by the extension; enter their credentials in pt-tools' site settings instead, as described in [Cookies and API keys](get-cookie-apikey.md).

## Collecting pages

For sites pt-tools does not support yet, and for reporting a parsing problem on one it does. The extension collects a torrent list page, a torrent details page and your profile page, and removes passkeys, cookies, session IDs, email addresses, IP addresses, API keys, tokens and invitation links as it does. Afterwards you can export a ZIP or open a pre-filled GitHub issue. The full steps are in [Request a new site](request-new-site.md).

## Privacy

- The extension sends cookies only to the pt-tools address you entered in Global Settings.
- The pt-tools user name and password you enter are kept in the browser's extension storage. On a shared computer, do not save the password, and remove the extension when you are done.
- The exported ZIP is already redacted, but it is still worth opening it and checking before you submit it.
