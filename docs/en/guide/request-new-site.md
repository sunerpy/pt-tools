# Request a new site

If you would like pt-tools to support a new tracker but have no programming experience, you can send the site's page data and the maintainers will add the support.

**The whole process needs no programming knowledge**; all you need is to be able to use a browser.

## Overview

To support a new site, pt-tools needs to know how the site's pages are structured. You provide **3 kinds of page data**:

| No. | Data                                    | Used to parse                                                | Required |
| --- | --------------------------------------- | ------------------------------------------------------------ | -------- |
| 1   | HTML of the search or torrent list page | Torrent titles, sizes, free status and so on                 | ✅       |
| 2   | HTML of a torrent details page          | The free level, when freeleech expires, H&R status and so on | ✅       |
| 3   | HTML of your profile page               | Upload, download, ratio, bonus points and so on              | ✅       |

There are two ways to provide them:

| Method                                               | Difficulty               | Redacted automatically | Recommended    |
| ---------------------------------------------------- | ------------------------ | ---------------------- | -------------- |
| **Collect automatically with the browser extension** | ⭐ Very easy (one click) | ✅ Yes                 | ✅ Recommended |
| Save the HTML by hand                                | Harder (6 steps)         | ❌ By hand             | Alternative    |

---

## Option 1: collect automatically with the browser extension (recommended)

With the **PT Tools Helper** browser extension, one click on the site collects every page that is needed and removes sensitive information automatically.

The extension's labels follow your browser's language; this page gives the English label with the Chinese one in parentheses.

### Installing the extension

**Recommended: install it from the Edge store (it updates itself, but each release waits about a week for Microsoft's review)**

Go to [Edge Add-ons](https://microsoftedge.microsoft.com/addons/detail/pt-tools-helper/pgicnjkmgenmjfhlclodbpbedjmojbea), find "PT Tools Helper" and install it. An extension installed from the Edge store also works in other Chromium-based browsers, such as Chrome.

**Alternative: install it by hand (it does not update itself, so you update it by hand)**

1. Download the latest `pt-tools-helper.zip` from [GitHub Releases](https://github.com/sunerpy/pt-tools/releases)
2. Unpack it to a folder you keep
3. Open the browser's extensions page:
   - Chrome → `chrome://extensions`
   - Edge → `edge://extensions`
4. Turn on **Developer mode** → choose **Load unpacked** → select the unpacked folder
5. The first time you click the extension's icon, click **🔓 Grant & Enable** (🔓 授权并启用)

### Collecting in one click

1. **Sign in** to the tracker in your browser
2. Open **any page** of the site (the home page or the torrent list will do)
3. Click the extension's icon in the browser toolbar
4. The extension recognises the site and shows **❓ Unknown Site** (❓ 未知站点), with the platform it detected, such as NexusPHP, on the **Detected Schema** (检测架构) line
5. Click **🚀 Auto Collect All** (🚀 一键自动采集)
6. The extension then:
   - opens the torrent list page and extracts its HTML
   - picks a torrent from the list (a free one if possible), opens its details page and extracts the HTML
   - finds your user ID, opens your profile page and extracts the HTML
   - also collects optional samples such as the home page, the free torrent listing, a details page without promotion and the H&R page (missing ones do not prevent basic support)
   - redacts all the HTML automatically (removing passkeys, cookies, email addresses, IP addresses and so on)
7. When it has finished, **Collection Progress** (采集进度) shows 3/3, and a message says **Auto collection complete: N page(s)** (自动采集完成 N 页)

> If automatic collection does not get all 3 pages (because no torrent ID was found, for example), expand **Manual Page-by-Page Capture** (手动逐页采集) and use the capture button on each missing page.

### Exporting and submitting

After collecting, use the **Collection Records** (采集记录) area at the bottom of the extension's pop-up:

**Option A: open a GitHub issue (recommended)**

1. Click **🐙 Create Issue** (🐙 提交 Issue)
2. The extension downloads a ZIP file and opens a GitHub issue page with the title and body already filled in
3. **Drag the downloaded ZIP file into the issue editor** to upload it
4. Click "Submit new issue"

**Option B: send it to a community group**

1. Click **📦 Export ZIP** (📦 导出 ZIP) to download the collected data
2. Send the ZIP file to a community group:
   - Telegram: https://t.me/+7YK2kmWIX0s1Nzdl
   - QQ group: 274984594

> The collected data is already redacted and safe to submit. If you still have doubts, open the HTML files in the ZIP with a text editor and check them.

---

## Option 2: collect the pages by hand

If you would rather not install the browser extension, collect the pages by hand with the steps below.

### Preparation

You need:

- a browser signed in to the tracker (Chrome or Edge recommended)
- a text editor (Notepad will do)

> **Important**: make sure you have a normal account on the site, in good standing.

## Step 1: identify the site's platform

Most trackers are built on one of these platforms, and each platform structures its pages differently:

| Platform     | How to recognise it                                               | Typical sites                              |
| ------------ | ----------------------------------------------------------------- | ------------------------------------------ |
| **NexusPHP** | "NexusPHP" at the bottom of the page; the torrent list is a table | HDSky, SpringSunday, most Chinese trackers |
| **mTorrent** | A modern interface that uses an API                               | M-Team                                     |
| **Gazelle**  | Looks like What.CD, with collapsible torrent groups               | Some music sites                           |
| **Unit3D**   | A modern interface with "UNIT3D" at the bottom of the page        | Some newer sites                           |

**How to check**:

1. Open the site's home page and scroll to the very bottom
2. Look for text such as "Powered by NexusPHP" or "UNIT3D"
3. If you are not sure, do not worry: send a screenshot of the page to the maintainers

## Step 2: save the search or torrent list page

In this step you save the **complete HTML source** of the site's torrent list page.

> **Why this is needed**: the maintainers write the parsing rules from the HTML structure, to extract torrent titles, sizes, free status, upload times and so on.

### Steps

1. **Sign in to the site** and open the torrent list page (usually `torrents.php` or the home page)

2. **Make sure the list includes freeleech torrents**. If there are none at the moment, you can:
   - wait for the site to run a freeleech event
   - or save the current page anyway (free torrents help, but they are not required)

3. **Open the browser's developer tools**:
   - press `F12` (or `Ctrl+Shift+I`)
   - a panel opens on the right or at the bottom of the browser

4. **Copy the page's HTML**:

   **Method 1: with the developer tools (recommended)**

   - In the developer tools, click the **Elements** tab at the top
   - At the very top of the HTML tree on the left, find the `<html>` tag
   - **Right-click** the `<html>` tag
   - Choose **Copy → Copy outerHTML**
   - Open Notepad, paste (`Ctrl+V`) and save it as `search.html`

   **Method 2: with view-source**
   - In the address bar, put `view-source:` in front of the current address
     - for example `view-source:https://example.com/torrents.php`
   - Press `Enter`
   - Select everything on the page (`Ctrl+A`) and copy it (`Ctrl+C`)
   - Open Notepad, paste and save it as `search.html`

   **Method 3: with Ctrl+S (the simplest)**
   - On the torrent list page, press `Ctrl+S`
   - As the file type, choose **Webpage, HTML Only** (not "Webpage, Complete")
   - Save it as `search.html`

### Checking the saved file

Open the saved file in Notepad and check that:

- its size is **between a few dozen KB and a few hundred KB**
- it starts with `<html` or `<!DOCTYPE`
- you can find torrent titles in it

## Step 3: save a torrent details page

In this step you save the HTML of the details page of a **freeleech torrent**.

> **Why this is needed**: the maintainers need the structure of the details page to parse key information such as the free level (Free, 2xFree and so on), when the freeleech expires and whether an H&R (Hit and Run) rule applies.

### Steps

1. In the torrent list, **find a torrent with a freeleech tag** (an icon or text such as "Free" or "2X Free")

2. **Open that torrent's details page**

3. **Save the page's complete HTML the same way as in step 2**, as `detail.html`

### Notes

- **Prefer a torrent with a freeleech tag**, so that the saved HTML includes the DOM structure of the free status
- It is even better if the free torrent also shows **when the freeleech expires** (such as "free for a limited time, 3 days left")
- If there is no free torrent at the moment, the details page of an ordinary torrent will do, but say so in your description

## Step 4: save your profile page

In this step you save the HTML of your **profile page**.

> **Why this is needed**: the maintainers parse upload, download, ratio, bonus points, user class and other information from the page's structure.

### Steps

1. Click your **user name** on the site (usually at the top right) to open your profile page
   - The address is usually like `userdetails.php?id=xxxxx`

2. **Save the page's complete HTML the same way as in step 2**, as `userinfo.html`

3. If the site's home page (`index.php`) also shows your user information (upload, number of seeding torrents and so on), save the home page's HTML as well, as `index.html`

### Extra pages (optional but helpful)

If the site has these pages, save them too:

| Page                    | Usual address                | Used to parse                           |
| ----------------------- | ---------------------------- | --------------------------------------- |
| Bonus points page       | `mybonus.php`                | The bonus earned per hour               |
| Seeding statistics page | `getusertorrentlistajax.php` | The number and size of seeding torrents |

## Step 5: describe the site

Put this information in a text file named `site-info.txt`:

```
Site name: (for example: HDSky)
Site address: (for example: https://hdsky.me/)
Platform: (NexusPHP / mTorrent / Gazelle / Unit3D / not sure)
Sign-in method: (Cookie / API Key / Passkey / not sure)

Torrent list page address: (for example: https://hdsky.me/torrents.php)
Torrent details page address: (for example: https://hdsky.me/details.php?id=12345)
Profile page address: (for example: https://hdsky.me/userdetails.php?id=99999)

Notes: (anything you think is useful, such as:
  - whether the site marks freeleech in a special way
  - whether it has an H&R rule
  - whether it needs a special sign-in method
  - the address of the page with the class rules
)
```

## Step 6: submit the files

### Option 1: through a community group (recommended)

Join one of these groups and **send the files privately to the group owner**:

| Platform     | How to join                                      |
| ------------ | ------------------------------------------------ |
| **Telegram** | [Join the group](https://t.me/+7YK2kmWIX0s1Nzdl) |
| **QQ group** | Group number: 274984594                          |

**What to send**:

1. `search.html`: the torrent list page HTML
2. `detail.html`: the torrent details page HTML
3. `userinfo.html`: the profile page HTML
4. `index.html`: the home page HTML (if you have it)
5. `site-info.txt`: the basic site information

> 💡 **Tip**: pack all the files into one **zip archive** before sending; that makes them easier for the maintainers to receive.

### Option 2: through a GitHub issue

1. Go to [GitHub Issues](https://github.com/sunerpy/pt-tools/issues)
2. Click **New issue**
3. Use the title format `[New Site Request] Site name`
4. Fill in the basic site information in the body
5. **Upload the HTML files as attachments** (drag them into the editor)

> ⚠️ **Note**: GitHub issues are public, so be sure to redact the files before you submit them (see the privacy notes below). Sending them privately in a group is safer.

## Privacy and security

> ⚠️ **Very important**: redact the HTML files before you submit them!

HTML files may contain sensitive personal information; **check for it and remove it before submitting**:

### Information you must remove

| Sensitive information | How it appears in the HTML                                       | What to do                                                                       |
| --------------------- | ---------------------------------------------------------------- | -------------------------------------------------------------------------------- |
| **Cookie / session**  | Saving with "Copy outerHTML" usually does not include the cookie | Search for `cookie` and `PHPSESSID`, and delete anything you find                |
| **Passkey**           | Usually in RSS links, in the form `passkey=abc123def456...`      | Search for `passkey` and replace the value with `REMOVED`                        |
| **User name**         | Your user name as shown on the page                              | You can keep it (it helps confirm the parsing) or replace it with a made-up name |
| **Email**             | May be shown on the profile page                                 | Search for it and replace it with `user@example.com`                             |
| **IP address**        | Some sites show your last sign-in IP address on the profile page | Search for it and replace it with `127.0.0.1`                                    |
| **Invitation links**  | Some sites may show your invitation link                         | Search for `invite` and delete the related content                               |

### Redacting quickly

1. Open the HTML file in Notepad
2. Use `Ctrl+H` (find and replace)
3. Search for and replace each item in the table above
4. Save the file

> **If you are not sure whether something is sensitive**, ask the maintainers in the group first. It is better to remove too much than to leak personal information.

## Questions

### Q: I do not know which platform the site uses. What should I do?

Do not worry. Send the HTML files and the maintainers will work it out.

### Q: The site has no freeleech torrents at the moment. What now?

Save the pages of an ordinary torrent for now, and send a page with a freeleech torrent later, once there is one.

### Q: Can I send screenshots only?

**No.** Screenshots cannot be used to parse the HTML structure; the HTML source files are required. Screenshots can add explanation (for example, pointing out which icon marks freeleech), but they cannot replace the HTML files.

### Q: What if the site uses an API rather than web pages?

Some sites (such as M-Team) use an API. In that case:

1. Open the browser's developer tools (`F12`)
2. Switch to the **Network** tab
3. Run a search on the site
4. In the Network panel, find the API request (usually one starting with `api/`)
5. Right-click the request → **Copy → Copy response**
6. Save the response as `search-api.json`

In the same way, save the response of the user information API as `userinfo-api.json`.

### Q: How long until the site is supported?

It depends on the maintainers' time; usually:

- a standard NexusPHP site: 1-3 days
- a site that needs a custom driver: 3-7 days
- if the data is incomplete and more is needed: it depends on how quickly you respond

### Q: I can program. Can I add the site myself?

Of course. The full development process is described in [Development guide: adding a new site](../../development.md#添加新站点支持), which is in Chinese.

---

Thank you for helping pt-tools support more sites! Every new site is a contribution to the community.
