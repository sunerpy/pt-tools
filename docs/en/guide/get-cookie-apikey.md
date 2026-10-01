# Cookies and API keys

This page explains how to obtain a site's credentials (a cookie, an API key or a passkey), which pt-tools needs before it can do anything with the site.

## The ways sites sign in

pt-tools supports three ways of authenticating with a site:

| Method      | Sites                                | Characteristics                                | Recommended setup                                |
| ----------- | ------------------------------------ | ---------------------------------------------- | ------------------------------------------------ |
| **Cookie**  | HDSky, SpringSunday, HDDolby, NovaHD | The traditional way; expires from time to time | ✅ Synced automatically by the browser extension |
| **API key** | M-Team                               | Valid for a long time                          | Entered by hand                                  |
| **Passkey** | Rousi Pro                            | Valid for a long time                          | Entered by hand                                  |

**For sites that use a cookie, the browser extension is strongly recommended.** It saves you copying and pasting by hand, and updates the cookie when it expires.

---

## Option 1: sync automatically with the browser extension (recommended)

Once you are signed in to a site, the **PT Tools Helper** browser extension sends its cookie to pt-tools in one click, **with no developer tools and no copying and pasting**.

The extension follows your browser's language; the labels below are its English ones.

### Installing the extension

**Recommended: install it from the Edge Add-ons store (updates itself, but each release waits about a week for Microsoft's review)**

Go to [Edge Add-ons](https://microsoftedge.microsoft.com/addons/detail/pt-tools-helper/pgicnjkmgenmjfhlclodbpbedjmojbea), search for "PT Tools Helper" and install it. An extension installed from Edge Add-ons works in other Chromium browsers, such as Chrome, too.

**Alternative: install it by hand (does not update itself)**

1. Download the latest `pt-tools-helper.zip` from [GitHub Releases](https://github.com/sunerpy/pt-tools/releases)
2. Unpack it into a folder you keep (do not delete it afterwards: the browser keeps reading it)
3. Open the browser's extensions page:
   - Chrome → `chrome://extensions`
   - Edge → `edge://extensions`
4. Turn on **Developer mode** at the top right
5. Click **Load unpacked** and choose the unpacked folder
6. The extension's icon appears in the toolbar (the same icon as pt-tools: a teal chevron and an orange block on a dark square)
7. Click the icon for the first time and choose **🔓 Grant & Enable** to give it the permissions it needs

### Connecting to pt-tools

The extension needs pt-tools' address before it can sync cookies:

1. Click the extension's icon in the toolbar
2. In **Global Settings** at the bottom, fill in:
   - **pt-tools URL**: the address of your pt-tools service (such as `http://localhost:8080` or `http://192.168.1.100:8080`)
   - **Username (optional)**: your pt-tools user name
   - **Password (optional)**: your pt-tools password
3. Click **🔗 Test Connection** to check the connection
4. When it works, the extension shows "Connected to pt-tools. Settings saved."

> If pt-tools runs on another machine or in Docker, make sure the address you enter can be reached from this browser.

### Syncing one site's cookie

1. **Sign in** to the site in the browser as usual (HDSky, for example)
2. Click the extension's icon; it shows **✅ HDSky (NexusPHP)**
3. Check that the cookie status is **Valid**
4. Click **🔄 Sync Cookie to pt-tools**
5. A confirmation appears once the sync succeeds

After the sync, the site's cookie in pt-tools is updated; there is nothing to fill in in the pt-tools web interface.

### Syncing every site's cookie at once

If you are signed in to several sites, you can sync all their cookies in one go:

1. Click the extension's icon
2. Expand **Global Settings** at the bottom
3. **Batch Cookie Sync** lists every built-in site
4. Each site shows its cookie status (Valid, Expiring Soon, Expired or Missing)
5. Tick the sites you want to sync (or click **Select All**)
6. Click **🔄 Sync Selected**

> **Note**: sites that use an API key or a passkey (such as M-Team and Rousi Pro) are greyed out and cannot be selected; their credentials are entered in pt-tools by hand.

### Turning on automatic sync

With automatic sync on, a cookie change is pushed to pt-tools whenever you visit the site in the browser:

1. Visit the site
2. Click the extension's icon
3. Turn on **Auto-sync Cookie**

From then on, every cookie update (signing in again, a refreshed cookie) is synced to pt-tools automatically. Syncs of the same site are at least 30 seconds apart, and each result is shown as a browser notification.

---

## Option 2: obtain the credentials by hand

If you would rather not install the extension, or the site uses an API key or a passkey, use the manual methods below.

### Cookie authentication

#### What a cookie is

A cookie is a small piece of data a website stores in the browser to keep you signed in. Sites recognise a signed-in user by the cookie.

**Sites**: HDSky, SpringSunday, HDDolby, NovaHD and the other sites that run NexusPHP.

### Getting it by hand in Chrome or Edge

**Step 1: sign in to the site**

Make sure you are signed in to the site and can browse its pages normally.

**Step 2: open the developer tools**

Open the developer tools in any of these ways:

- Press `F12`
- Press `Ctrl + Shift + I` (Windows/Linux) or `Cmd + Option + I` (Mac)
- Right-click an empty part of the page and choose Inspect

**Step 3: switch to the Network tab**

In the developer tools, click the Network tab at the top.

**Step 4: reload the page**

Press `F5` or click the browser's reload button, so the developer tools capture the requests.

**Step 5: choose a request and copy the cookie**

1. In the list of requests, click the first one (usually the page itself)
2. In the panel on the right, open the Headers tab
3. Scroll down to Request Headers
4. Find the `Cookie` field
5. Copy the whole value of the cookie (it is usually long and holds several key-value pairs)

**What a cookie looks like:**

```
c_secure_uid=xxxxx; c_secure_pass=xxxxx; c_secure_ssl=xxxxx; c_secure_tracker_ssl=xxxxx
```

### Getting it by hand in Firefox

**Steps 1 and 2**: as in Chrome; sign in to the site and press `F12` to open the developer tools.

**Step 3**: click the Network tab.

**Step 4**: reload the page.

**Step 5:**

1. Click any request
2. Under Headers on the right, find Request Headers
3. Copy the full value of the `Cookie` field

### Questions about cookies

**Q: How long does a cookie last?**

A: On most sites a cookie lasts between one and four weeks. With the extension and automatic sync on, cookie updates are pushed to pt-tools without any work on your part.

**Q: How can I make a cookie last longer?**

A: Visiting the site regularly refreshes the cookie. On some sites the "remember me" option makes it last longer. With the extension's automatic sync, the cookie is updated in pt-tools whenever you visit the site.

**Q: What do I do when the cookie has expired?**

A:

- With the extension: sign in to the site again; the extension notices and syncs the new cookie
- By hand: get the new cookie from the browser and update it in pt-tools

**Q: What should I watch out for when copying a cookie?**

A:

- Copy the complete cookie string; do not leave anything out
- Do not include the `Cookie:` prefix; copy only the value
- Do not add spaces or line breaks while copying

## API key authentication

### What an API key is

An API key is an access token the site generates for you. Compared with a cookie:

- **It lasts a long time**: it does not expire as often as a cookie
- **It can be revoked at any time**: if it leaks, you can generate a new one immediately

**Sites**: M-Team

### Getting an M-Team API key

**Step 1: sign in to M-Team**

Sign in to M-Team in the browser.

**Step 2: open the security settings**

1. Click your avatar or user name at the top right
2. Choose Control panel (控制面板) or Settings
3. In the menu on the left, find Lab (实验室)
4. Click Access tokens (存取令牌)

**Step 3: generate an API key**

1. If you have no API key yet, click Create token (生成新令牌)
2. Give the token a name (such as `pt-tools`) so you remember what it is for
3. Confirm to generate it

**Step 4: copy the API key**

The API key is shown only once after it is generated. Copy it straight away and keep it somewhere safe.

**What an API key looks like:**

```
xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
```

### Keeping an API key safe

1. **Never share it**: an API key is as good as your account password
2. **Revoke a leaked key at once**: if you suspect a leak, revoke the key on the site and generate a new one
3. **Rotate it**: replace the API key every few months
4. **Never commit it to a repository**: keep it in an environment variable or a separate configuration file

## Entering the credentials in pt-tools

### Way A: through the browser extension (recommended for cookie sites)

After the extension syncs a cookie, the site's credentials in pt-tools are **updated automatically**; there is nothing to do in the web interface. Make sure that:

1. The extension is connected to pt-tools (see [Connecting to pt-tools](#connecting-to-pt-tools) above)
2. You clicked Sync Cookie in the extension, or turned on automatic sync
3. The site is enabled in pt-tools' site list

### Way B: by hand in the web interface

1. Sign in to the pt-tools web interface
2. Open Sites → Site list (站点 → 站点列表) and click the Site settings and RSS (站点配置与 RSS 订阅) button on the site's row
3. On the Overview (概览) tab, turn on Enable site (启用站点)
4. Switch to the Credentials (凭据) tab and enter the cookie, API key or passkey
5. Click Save settings (保存配置) at the top of the page

> [!NOTE]
> A saved cookie is never shown again, for security; saving with the field empty keeps the stored value. API keys and passkeys show their current value and cannot be saved empty; to replace one, enter the new value.

### Checking the result

After saving, the Status (状态) column of the site list shows how the site is doing:

| Status                | Meaning                                                                        |
| --------------------- | ------------------------------------------------------------------------------ |
| **OK (正常)**         | The site is available and the last login probe succeeded (or none has run yet) |
| **Problem (异常)**    | The site is unavailable, or the last login probe failed                        |
| **Disabled (已禁用)** | The site is not enabled                                                        |

Click Probe now (立即探测) on the row to check the login status at once; see [Login status and key backup](site-login-monitoring.md).

## The methods compared

| Aspect        | Cookie (synced by the extension) | Cookie (by hand)         | API key                     |
| ------------- | -------------------------------- | ------------------------ | --------------------------- |
| Effort        | ⭐ Very low (one click)          | Higher (developer tools) | Low (generated on the site) |
| Lifetime      | Renewed automatically            | One to four weeks        | Long                        |
| How it is set | Pushed by the extension          | Pasted by hand           | Pasted by hand              |
| Security      | Average                          | Average                  | Better                      |
| Sites         | NexusPHP                         | NexusPHP                 | mTorrent                    |

## Troubleshooting

### 1. Authentication fails

**Possible causes:**

- The cookie or API key has expired or is no longer valid
- Part of it was left out when copying
- Something is wrong with the account (it has been banned, for example)

**What to do:**

1. Sign in to the site again and check the account is in order
2. Get the cookie or API key again
3. Make sure you copied all of it, with no extra spaces

### 2. The connection times out

**Possible causes:**

- A network problem
- The site is down or under maintenance
- A firewall is blocking the connection

**What to do:**

1. Check the network connection
2. Try opening the site in a browser
3. Check whether the site has to be reached through a proxy

### 3. The cookie keeps expiring

**Recommended**: install the browser extension and turn on automatic sync, so every cookie update is pushed to pt-tools.

**Common causes when you manage it by hand:**

- The site has a strict security policy
- You have not visited the site for a long time
- Signing in on another device signed you out

**What to do:**

1. Visit the site regularly to stay active
2. Tick "remember me" when you sign in
3. Avoid being signed in on several devices at once

### 4. The API key does not work

**Possible causes:**

- The key has been revoked
- It lacks the permissions needed
- The site's API service has a problem

**What to do:**

1. Check the key's status on the site
2. Try generating a new key
3. Make sure the site supports API access

---

Once the credentials are in place, the next step is [RSS subscriptions](rss-subscription.md), to set up automatic downloads.
