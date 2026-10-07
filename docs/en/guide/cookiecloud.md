# CookieCloud import

CookieCloud import (CookieCloud 导入), under Sites (站点), fetches the cookies the browser extension syncs to your self-hosted [CookieCloud](https://github.com/easychen/CookieCloud) server, decrypts them on this machine, picks the cookies that work for pt-tools sites by their addresses, and writes them into the sites after a preview. Scheduled sync is available and off by default.

## Before you start

- You need a working CookieCloud: the CookieCloud extension in your browser, syncing to your own server.
- The extension shows the server address, the user KEY (UUID) and the end-to-end encryption password; enter the same three in pt-tools.
- pt-tools only fetches the encrypted data from the server (`GET <server address>/get/<UUID>`) and decrypts it on this machine with the password; the password is never sent to the CookieCloud server.
- The password is stored on this machine, encrypted with the same key as the site cookies; the page and the API never show it, only whether it is set.
- Only cookies that work for the addresses of pt-tools sites are imported; cookies of other websites are not stored.

Both encryption modes of the extension are supported: the original default, and `aes-128-cbc-fixed`, available from 0.3.0.

## Settings

| Setting                     | Meaning                                                                                                              |
| --------------------------- | -------------------------------------------------------------------------------------------------------------------- |
| Server address (服务地址)   | The CookieCloud server, starting with `http://` or `https://`                                                        |
| User KEY (用户 KEY（UUID）) | The user KEY in the extension                                                                                        |
| Password (端对端加密密码)   | The password in the extension; left empty, the saved one stays; Clear (清除) deletes it and turns scheduled sync off |
| Scheduled sync (定时同步)   | Syncs at the interval when on, updating only enabled sites whose cookie changed; it never enables a site             |
| Interval (同步间隔（小时）) | 1 to 168 hours, 24 by default                                                                                        |

The result of each sync shows as Last sync (上次同步) below the settings.

## Importing

1. Click Read CookieCloud (读取 CookieCloud): pt-tools fetches and decrypts the data on this machine and lists the sites it can import, the cookie names and each site's state. Nothing is written, and the cookie contents are not shown.
2. Tick the sites to import, or click Select changed (选中有变化的) for the enabled sites whose cookie changed.
3. Click Import selected (导入选中) and confirm: the selected sites get the cookies from CookieCloud, and sites not enabled yet are enabled. pt-tools then refreshes these sites and checks their login once, as it does when the browser extension syncs a cookie.

A site's state is one of: not enabled (enabled by the import), cookie changed, or unchanged (importing it writes nothing).

### How cookies are picked

pt-tools picks cookies by the host name of the address it uses for the site:

- A cookie whose domain starts with a dot (such as `.hdsky.me`) works for that domain and its subdomains; one without the dot works only for the same host name.
- Cookies with a path other than `/`, and expired ones, are left out.
- Of cookies with the same name, the one with the most specific domain wins.

Only sites that sign in with a cookie are supported; sites that use only an API key or a passkey (such as M-Team) are not listed.

## API

| Method and path                        | Purpose                                                                                                                                                          |
| -------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET`, `PUT /api/cookiecloud/settings` | Read or save the settings: everything but `password` is saved as a whole and unknown fields are rejected; without `password` the saved one stays, `""` clears it |
| `POST /api/cookiecloud/preview`        | Fetch, decrypt and match; returns the sites that can be imported and their cookie names (no cookie values)                                                       |
| `POST /api/cookiecloud/import`         | `{"sites": [...]}`: import the cookies of the selected sites; returns the sites updated, unchanged, not found and failed                                         |

All of them require a signed-in session.
