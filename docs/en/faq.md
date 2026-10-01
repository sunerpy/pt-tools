# Questions and troubleshooting

This page collects common problems with pt-tools and how to solve them.

---

## Downloaders

### Cannot connect to a downloader

**Symptom**: after you add a downloader, its Connectivity (连通性) column shows Error (异常), and hovering over it shows a reason such as 连接失败 (connection failed).

**What to check**:

1. **The URL format**
   - Include the scheme: `http://` or `https://`
   - Make sure the port is right
   - Example: `http://192.168.1.10:8080`

2. **The user name and password**
   - Make sure they are correct
   - Check whether special characters in the password need escaping

3. **That the downloader's web UI is enabled**
   - qBittorrent: Tools → Options → Web UI → Web User Interface (Remote control)
   - Transmission: Preferences → Remote → Allow remote access

4. **The firewall**
   - Make sure the port is open
   - Check for security group restrictions

5. **Docker in particular**
   - Do not use `localhost` or `127.0.0.1`
   - Use the host's IP address or a Docker network name
   - Example: `http://host.docker.internal:8080` (Docker Desktop)
   - Or the host's actual IP address: `http://192.168.1.100:8080`

### Pushing a torrent fails

**Symptom**: a torrent cannot be pushed to the downloader, and the message 推送失败 (push failed) appears.

**What to check**:

1. **The downloader's connection**
   - Under Downloads → Downloader settings (下载 → 下载器设置), click Check connectivity (检查连通性) on the downloader's row
   - Make sure the connection works

2. **That the download folder exists**
   - Check that the configured download path exists
   - Make sure the downloader can write to that folder

3. **Disk space**
   - Make sure the target disk has enough free space

4. **The downloader's log**
   - Look for error messages in qBittorrent or Transmission
   - The torrent file may be damaged or malformed

### A paused torrent cannot be resumed

**Symptom**: on the Paused torrents (暂停任务) page, clicking Resume (恢复) does nothing.

**What to check**:

1. **That the downloader is online**
   - Make sure the downloader is running
   - Check its connection

2. **That the task still exists**
   - Check in the downloader whether the task is still there
   - If it was deleted, add the torrent again

3. **The task's state**
   - The task may be in an error state
   - Try working on it directly in the downloader

---

## Site sign-in

### A cookie has expired

**Symptom**: in the site list, the Status (状态) column shows Error (异常), and Probe now (立即探测) reports 会话已过期 (session expired).

**How to fix it**:

1. **Get a new cookie**
   - Sign in to the site
   - Copy the new cookie with the browser's developer tools
   - See [Cookies and API keys](guide/get-cookie-apikey.md)

2. **Copy all of it**
   - Cookies are long; make sure you copy the whole value
   - Do not include the `Cookie:` prefix
   - Avoid extra spaces or line breaks

3. **Check the account on the site**
   - Make sure the account has not been banned
   - Make sure it has not been signed out by force

4. **Make the cookie last longer**
   - Tick the site's Remember me option when you sign in
   - Visit the site regularly to stay active

### An API key does not work

**Symptom**: on M-Team and similar sites, the API key is reported as invalid.

**How to fix it**:

1. **Check whether the key was revoked**
   - Sign in to the site and check the API key's status in your settings there
   - If it was revoked, generate a new one

2. **Check the key's permissions**
   - On some sites an API key has a limited scope
   - Make sure it has the permissions it needs

3. **Generate a new key**
   - Delete the old key on the site
   - Generate a new API key
   - Update it in pt-tools

---

## RSS subscriptions

### An RSS feed cannot be read

**Symptom**: the feed never produces any tasks, and System → Logs (系统 → 运行日志) shows RSS任务失败 (RSS task failed) or 解析 RSS 失败 (failed to parse the RSS feed).

**What to check**:

1. **The RSS link**
   - Open the link directly in a browser
   - Make sure it returns XML

2. **That the credentials are valid**
   - Whether the cookie or API key has expired
   - Whether the passkey in the RSS link is right

3. **The network**
   - Whether pt-tools can reach the site
   - Whether a proxy is needed

4. **The logs**
   - Check the pt-tools logs for the detailed error
   - They are in `~/.pt-tools/logs/` or `/app/.pt-tools/logs/`

### The feed brings in no freeleech torrents

**Symptom**: the feed is read without errors, but no freeleech torrent is downloaded.

**Possible causes**:

1. **The site really has no freeleech torrents**
   - Check on the site whether there are any

2. **The feed returns too few items**
   - Raise the `rows` parameter in the RSS link
   - For example `rows=50`

3. **The filter rules are too strict**
   - Turn off all the filter rules for a test
   - Add them back one at a time to find the problem

---

## Pausing when freeleech ends

### Torrents are not paused when freeleech ends

**Symptom**: after the free period has ended, the torrent keeps downloading instead of being paused.

**What to check**:

1. **That the option is on**
   - Edit the feed on the site's RSS subscriptions (RSS 订阅) tab
   - Check that Pause when freeleech ends (免费结束暂停) is on

2. **That the torrent is freeleech**
   - Torrents that are not free are not monitored
   - Check the torrent's free status in its details

3. **That the downloader is set up correctly**
   - The downloader connection works
   - pt-tools can control the downloader

4. **The free end time**
   - Some torrents have no free end time
   - Those torrents cannot be monitored

5. **The logs**
   - Check that the monitor is running
   - Look for related errors

**Suggested settings**:

| Situation                     | Suggestion                                                               |
| ----------------------------- | ------------------------------------------------------------------------ |
| **Plenty of bandwidth**       | You can leave the option off and let downloads finish sooner             |
| **Limited bandwidth**         | Turn it on, so nothing counts against your download after freeleech ends |
| **Little disk space**         | Turn it on and regularly clear out the unfinished paused tasks           |
| **Keeping an account active** | Turn it on, then resume by hand the tasks you want to keep seeding       |

### A torrent is paused soon after it starts

**Symptom**: a freeleech torrent is paused by pt-tools' freeleech-end monitor soon after it starts downloading, with little progress (1-2 %, for example).

**Cause**: the torrent had too little freeleech time left to finish before its free period ended.

**Solution**:

Set Minimum free time (最短免费时间) under Global settings → Download policy and limits (全局设置 → 下载策略与限制); the default is 30 minutes. Torrents with less freeleech time left than that are skipped and not downloaded, which avoids wasting download quota and bandwidth.

It also helps to turn on Download speed check (启用下载限速判断) and enter your real download speed: pt-tools then works out from the torrent's size and its remaining freeleech time whether it can finish in time.

---

## System administration

### Setting the proxy environment variables

**Symptom**: requests to sites time out or fail to connect, and you suspect a proxy is needed.

**How to set them**:

pt-tools reads the `HTTP_PROXY`, `HTTPS_PROXY`, `ALL_PROXY` and `NO_PROXY` environment variables, in upper or lower case. `ALL_PROXY` is used only for site access and downloader connections; fetching RSS feeds and the Telegram channel need `HTTP_PROXY`/`HTTPS_PROXY`.

The details and examples are in [Configuration: Proxy settings](configuration.md#proxy-settings).

Restart the service after changing the environment variables, so that they take effect.

### Resetting the admin password

**Method**: reset it with environment variables.

**Docker**:

```bash
docker run -d \
  -e PT_ADMIN_RESET=1 \
  -e PT_ADMIN_USER=admin \
  -e PT_ADMIN_PASS='your_new_password' \
  -v ~/pt-data:/app/.pt-tools \
  -p 8080:8080 \
  sunerpy/pt-tools:latest
```

**Docker Compose**:

```yaml
environment:
  PT_ADMIN_RESET: "1"
  PT_ADMIN_USER: "admin"
  PT_ADMIN_PASS: "your_new_password"
```

**Important**:

- After the reset, be sure to remove the `PT_ADMIN_RESET` environment variable
- Restart the container so that the change takes effect

### A damaged database

**Symptom**: a database error on start, or pages that cannot load their data.

**How to fix it**:

1. **Restore a backup**

   ```bash
   # Stop the service
   docker stop pt-tools

   # Keep a copy of the current database
   cp ~/.pt-tools/torrents.db ~/.pt-tools/torrents.db.broken

   # Restore the backup
   cp ~/backup/torrents.db ~/.pt-tools/

   # Start the service again
   docker start pt-tools
   ```

2. **Without a backup, try to repair it**

   ```bash
   # Check and repair it with the sqlite3 tool
   sqlite3 ~/.pt-tools/torrents.db "PRAGMA integrity_check;"
   sqlite3 ~/.pt-tools/torrents.db ".recover" | sqlite3 ~/.pt-tools/torrents_recovered.db
   ```

3. **As a last resort, start over**

   ```bash
   # Keep the old database
   mv ~/.pt-tools/torrents.db ~/.pt-tools/torrents.db.old

   # Restart the service (a new database is created)
   docker restart pt-tools

   # Every site and downloader then has to be configured again
   ```

---

## Other problems

If none of the above solves your problem:

1. **Check the logs**: the log files in `~/.pt-tools/logs/`
2. **Open an issue**: [GitHub Issues](https://github.com/sunerpy/pt-tools/issues)
3. **Ask the community**: when you describe the problem, include
   - the pt-tools version
   - how you deploy it (Docker or binary)
   - the full error message or log
   - the steps that reproduce it

---

Related pages:

- [Configuration](configuration.md)
- [Cookies and API keys](guide/get-cookie-apikey.md)
- [RSS subscriptions](guide/rss-subscription.md)
