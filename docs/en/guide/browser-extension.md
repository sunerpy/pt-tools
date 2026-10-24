# Browser extension

This page covers installing the PT Tools Helper browser extension and what it does: syncing site cookies to pt-tools (on demand, on a schedule, or through CookieCloud), and collecting pages to request support for a new site.

## Installing it

| Option                              | Notes                                                                                                                                                                                                                                                                               |
| ----------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Edge Add-ons (recommended for Edge) | In Microsoft Edge, open [Edge Add-ons](https://microsoftedge.microsoft.com/addons/detail/pt-tools-helper/pgicnjkmgenmjfhlclodbpbedjmojbea) and search for "PT Tools Helper". It updates itself; each release waits about a week for store review. This store is for Edge only       |
| Load it yourself (for Chrome)       | Download `pt-tools-helper.zip` from the [releases page](https://github.com/sunerpy/pt-tools/releases) and unpack it to a folder you keep. In `chrome://extensions` or `edge://extensions`, turn on Developer mode and choose Load unpacked. It does not update itself               |
| Signed `.crx` package               | `pt-tools-helper.crx` on the same releases page, signed with a fixed key so the extension ID stays the same across versions. On Windows and macOS, Chrome does not allow installing a `.crx` file from outside the Chrome Web Store; load the zip yourself on those systems instead |
| Firefox                             | In `about:debugging#/runtime/this-firefox`, choose Load Temporary Add-on and pick `manifest.json` in the unpacked folder. It has to be loaded again after Firefox restarts                                                                                                          |

The extension follows the browser's language; the labels below are its English ones, with the Chinese labels in brackets. The first time you click the extension's icon, choose 🔓 Grant & Enable (授权并启用) to grant the permissions it needs.

## Connecting it to pt-tools

Choose ⚙ Settings (设置) at the top right of the popup, enter the pt-tools URL (for example `http://192.168.1.100:8080`), add your pt-tools user name and password if needed, and choose 🔗 Test Connection (连接测试). The address must be reachable from the browser you are using.

The top of the popup shows whether pt-tools is reachable. When the popup opens it shows the site list right away and asks pt-tools at the same time, waiting at most 4 seconds.

## Syncing cookies

For sites pt-tools already supports that sign in with a cookie.

The popup's main page is My sites (我的站点): every site with a cookie in this browser or enabled in pt-tools is listed (the other built-in sites appear after Show all (显示全部)). Each row shows the cookie's state in the browser and the site's keep-alive state in pt-tools (days left, about to expire); sites that need attention come first.

When the popup opens it first shows the results read last time, with Updating… (更新中…) next to the title, and replaces them once they are read again, without waiting for pt-tools. When the login status cannot be read from pt-tools (for example after pt-tools restarted and the extension's session is gone), the list keeps showing the keep-alive state read last time and the note above says when it was read; if it has never been read, each row reads Status unknown (状态未知). Choose Log in (去登录) above the list to sign in to pt-tools again in the settings.

When a cookie shows Missing (缺失), the row names the sign-in cookies that are missing (for example Missing c_secure_pass); usually you are not signed in to that site, and Sync (同步) appears once you sign in. Cookies that a site sets on its parent domain (such as XingYunGe's on `.xingyungept.org`) are recognized too.

- **One site**: choose Sync (同步) on its row; when the current tab is that site, the top of the popup also offers 🔄 Sync Cookie to pt-tools (同步 Cookie 到 pt-tools).
- **All sites**: choose Sync all (同步全部) above the list to sync every site whose cookie is valid.
- **Automatically**: open the popup on the site's page and turn on Auto-sync Cookie (自动同步 Cookie); the cookie is then synced whenever you sign in again or it changes.
- **Keeping accounts active**: Open sites (打开站点) opens the sites in background tabs; Open (打开) on a row opens one.

Sites that use an API key or a passkey (M-Team, Rousi Pro) cannot be synced by the extension; enter their credentials in pt-tools' site settings instead, as described in [Cookies and API keys](get-cookie-apikey.md). For sites such as HDDolby that need both a cookie and an API key, pt-tools stores the synced cookie first; until the API key is filled in the site stays disabled, and the extension reminds you to add the API key in pt-tools.

### Scheduled sync

Scheduled sync (定时同步) in Settings syncs on an interval (10–1440 minutes, 60 by default) while the browser is open, without opening the sites:

- By default it syncs only the sites with Auto-sync Cookie turned on. With Sync every site signed in on this browser (同步浏览器里登录着的全部站点) ticked, every site with a usable cookie in this browser is synced. For a site that is not enabled in pt-tools, only the cookie is saved and the site stays disabled: a scheduled sync never enables a site for you. In My sites the row reads Cookie saved, not enabled (Cookie 已存，待启用); enable the site in the pt-tools site list to use it. When the connected pt-tools is an older version, only the sites already enabled in pt-tools are synced.
- Only sites whose cookies changed are pushed: every push makes pt-tools refresh the site and check its sign-in once, so unchanged cookies are not pushed again. Choosing Sync always pushes.
- With CookieCloud below turned on, the same run also uploads to CookieCloud.
- Sites that should be synced but have no usable cookie in this browser (signed out, incomplete or expired) are not pushed, and the result lists them by name. Sign in to them in the browser and the next run syncs them. Sites that are disabled in pt-tools and have no cookie in this browser are not listed.
- The result of each run is shown under the settings, and the extension shows a system notification when a run has a problem the previous one did not have (for example the pt-tools sign-in expired, a site could not be pushed, or a site has no usable cookie).
- Sync now (立即同步一次) runs once right away with the same rules. When a site was not synced, has no usable cookie, or pt-tools is not connected, the message says the run had problems; the result below has the details.

### Syncing to CookieCloud

When pt-tools is not reachable from the browser (for example the browser is at work and pt-tools runs on a NAS at home), or other tools need the same cookies, the extension can encrypt the cookies and upload them to your own [CookieCloud](https://github.com/easychen/CookieCloud) server, and pt-tools fetches them from there as described in [CookieCloud import](cookiecloud.md). You do not need the CookieCloud extension as well.

1. Under CookieCloud in Settings, enter the server URL, the user KEY (UUID) and the end-to-end encryption password, pick the encryption (`aes-128-cbc-fixed` is recommended) and choose Save (保存). The first save asks you to let the extension reach that server.
2. Choose Test (测试): it tells you whether it connected, and whether the server has no data yet or has data it can decrypt.
3. Choose Upload now (立即上传) to upload once. With Upload to CookieCloud on scheduled sync ticked, every scheduled run uploads too (nothing is sent when the cookies have not changed).
4. In pt-tools, enter the same server URL, UUID and password under Sites → CookieCloud import.

The upload contains every cookie of the PT site domains pt-tools supports and nothing from other websites. Before uploading, the extension fetches the data already on the server, decrypts it locally and replaces only those PT site domains, keeping other domains and the local storage data as they are, so sharing a UUID with the CookieCloud extension does not wipe what it synced. If the password cannot decrypt the data already on the server, nothing is uploaded; after a password change, Upload now asks whether to overwrite it, and other devices then need the new password too.

The extension does not follow redirects from the server (the URL contains the UUID), so enter the final server URL.

## Collecting pages

For sites pt-tools does not support yet, and for reporting a parsing problem on one it does. The extension collects a torrent list page, a torrent details page and your profile page, and removes passkeys, cookies, session IDs, email addresses, IP addresses, API keys, tokens and invitation links as it does. Afterwards you can export a ZIP or open a pre-filled GitHub issue. The full steps are in [Request a new site](request-new-site.md).

## Privacy

- The extension sends cookies only to the pt-tools address you entered in Settings and, when it is turned on, to the CookieCloud server you entered. What goes to CookieCloud is encrypted with your password, which is used only in the browser and never sent.
- Like the CookieCloud extension, the extension keeps the CookieCloud password in the browser's extension storage; the popup never shows it, only that it is set.
- The pt-tools user name and password you enter are kept in the browser's extension storage. On a shared computer, do not save the password, and remove the extension when you are done.
- The exported ZIP is already redacted, but it is still worth opening it and checking before you submit it.
