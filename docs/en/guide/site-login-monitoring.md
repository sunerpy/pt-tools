# Login status and key backup

pt-tools can sync site cookies through PT Tools Helper and periodically probe the `last_access` / `last_login` information a site provides. When an account gets close to the site's inactivity limit, it sends a reminder through the notification channels you have configured.

## How to use it

1. Enable the site under Sites → Site list (站点 → 站点列表) and fill in its credentials. For a site that signs in with a cookie, you can install [PT Tools Helper](browser-extension.md) and sync the cookie in one click once you are signed in to the site in your browser; API keys and passkeys are entered in the web UI.
2. The Days left (剩余天数) column of the site list shows how many days remain before the site's inactivity limit; a negative number means the limit has passed. The Effective activity (判定活跃) column is the activity time the reminders are based on: the `last_access` the site returns when there is one, not the time you last signed in on the web page.
3. Click the Keep-alive settings (保号配置) button on the row to set, per site, the inactivity limit, how many days ahead to remind, the reminder cron, the notification channels and the probe mode.
4. The Probe now (立即探测) button on the row probes the login status at once, and Test reminder (测试提醒) sends a test message to the notification channels. Probe enabled sites (探测已启用) above the list probes every enabled site once, at most 3 at a time.
5. When a reminder arrives, visit the site as usual. If the cookie has expired, sync it again, then click Probe now to confirm the status has been refreshed.

> [!NOTE]
> Probing refreshes `last_access` (recent activity) on most sites. A few sites decide whether to remove an account by `last_login` (an actual sign-in) or by seeding activity; on those you still need to sign in by hand from time to time, whatever the days-left figure says.

## Per-site settings

The fields of the Keep-alive settings dialog:

| Field                                   | Default         | Meaning                                                                                                     |
| --------------------------------------- | --------------- | ----------------------------------------------------------------------------------------------------------- |
| Inactivity limit in days (封号判定天数) | `30`            | After this many days without activity, the site may disable the account                                     |
| Days ahead to remind (提前提醒天数)     | `10`            | How many days before the limit the reminders start                                                          |
| Reminder cron (提醒 cron)               | `0 10,22 * * *` | A standard 5-field cron (minute, hour, day, month, weekday); by default every day at `10:00` and `22:00`    |
| Notification channels (通知通道)        | Empty           | When set, reminders go only to the selected channels that are enabled; when empty, to every enabled channel |
| Probe mode (探测模式)                   | Auto            | Auto: follows the scheduled probes; Manual: only when you click Probe now; Disabled: never probes           |

The scheduled probe runs about every 6 hours, with a random offset per site. When probes have failed continuously for more than 24 hours, pt-tools sends a reminder that the cookie may have expired. When a site does not provide a reliable last-access field, the figures on the page are only indicative.

## Backing up the encryption key

`~/.pt-tools/secret.key` encrypts site cookies and other credentials with AES-256-GCM. If the key is lost, the stored credentials cannot be decrypted and have to be synced or entered again.

Export a backup:

```bash
pt-tools secret export > ~/secret.key.backup
chmod 600 ~/secret.key.backup
```

Restore a backup:

```bash
pt-tools secret import --force < ~/secret.key.backup
```

> [!CAUTION]
> A key backup is as sensitive as the key itself. Keep it in a password manager, on encrypted media or in a controlled backup system; never commit it to a repository or store it unencrypted on cloud storage.

### Persistence in Docker

A container deployment must mount `/app/.pt-tools` on the host or on a managed volume. The folder holds `secret.key`, the database, the logs and the staged torrent files; before recreating the container, make sure the mount is in place and a backup is available.

```yaml
volumes:
  - ./data:/app/.pt-tools
```

Deleting `./data` loses the database and the key together. Back up the key and the database as one consistent unit.

## Limits and compliance

This feature periodically visits a site's pages or profile API with your own cookie or API key. Before turning it on, make sure the site's rules allow automated access, and choose how often to probe according to the risk you are willing to take:

- Some sites may treat frequent scripted access as a violation.
- The login status fields come from the site and may be missing, delayed or mean something different.
- pt-tools does not guarantee that this feature prevents an account from being disabled, and accepts no responsibility for warnings, demotions or bans caused by automated access.

How to obtain and sync credentials is described in [Cookies and API keys](get-cookie-apikey.md).
