# MCP <Badge type="tip" text="1.0 preview" />

MCP (Model Context Protocol) is the protocol AI assistants use to call external tools. pt-tools serves MCP on its web port: once connected, the assistant in an MCP client such as Claude, Cursor or Cherry Studio can look at your torrents, downloaders, site statistics and subscriptions, search torrents, and, after you confirm, pause, delete or push torrents, add subscriptions and follow shows (schedules, version lock, episodes, episode rules), and organize or scrape the library again; with the Config scope on the token, it can also read and change pt-tools' configuration.

The MCP endpoint shares the web interface's port; its default address is `http://<pt-tools host>:<port>/mcp`. The System → MCP access (系统 → MCP 接入) page turns MCP access on and off and changes the path, and shows this instance's current address, how to set up each kind of client, and the tool list.

## Turn MCP access on

Under Settings on the System → MCP access page:

- MCP access: clients can connect only when it is on; while it is off the MCP address responds with 404 saying MCP access is off (MCP 接入没有打开). A new installation starts with it off. An instance upgraded from an earlier version that already has a token with an MCP permission starts with it on, so connected clients keep working.
- Path: `/mcp` by default; you can use another one (for example `/agent/mcp` when a reverse proxy only forwards one prefix). It may contain letters, digits and `. _ ~ -`, with `/` between levels; it cannot sit under `/api`, `/assets`, `/static` or `/login`, nor clash with an address pt-tools already uses. After a change the old `/mcp` responds with 404 (without telling where the new address is), so update the address in your clients.
- Changes apply as soon as they are saved, without a restart. The built-in [AI assistant](assistant.md) calls the same tools inside pt-tools rather than through this address, so the switch does not affect it.
- The switch and path can only be changed on the web page: neither API tokens nor `change_config` can change them.

## Create a token

Create a token under System → API tokens (系统 → API 令牌; when no token can use MCP yet, the MCP access page links there with the permissions already picked), see [API tokens](api-tokens.md):

| Permission           | What it allows                                                                                                                                                        |
| -------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| MCP read (MCP 读取)  | Read-only tools: fetched torrents, torrents in the downloaders, downloader status, search, site statistics, update check, finding titles and the subscription list    |
| MCP write (MCP 操作) | Tools that change your downloaders, subscriptions or libraries: pause, resume, delete and push torrents, manage subscriptions and followed shows, organize and scrape |
| Config (配置)        | An extra scope: `get_config` reads the configuration (with MCP read), `change_config` changes it (with MCP write)                                                     |

Neither MCP permission includes the other: a token with only MCP write cannot call the read-only tools, so you usually pick both. Pick only MCP read when the assistant should only look. Add Config to let it read and change pt-tools' configuration; existing tokens do not get it automatically.

## Set up the client

The examples assume pt-tools runs at `192.168.1.10:8080` with the default path `/mcp` and the token is `ptt_1_xxxxxxxx`; use your own values (the MCP access page fills in this instance's current address).

### Clients that speak HTTP

Clients such as Cursor, Cherry Studio and VS Code take the address and a request header; choose the Streamable HTTP transport. Where the configuration lives depends on the client; the fields look like this:

```json
{
  "mcpServers": {
    "pt-tools": {
      "url": "http://192.168.1.10:8080/mcp",
      "headers": { "Authorization": "Bearer ptt_1_xxxxxxxx" }
    }
  }
}
```

Claude Code adds it with a command:

```bash
claude mcp add --transport http pt-tools http://192.168.1.10:8080/mcp --header "Authorization: Bearer ptt_1_xxxxxxxx"
```

### Clients that only speak stdio

Clients such as Claude Desktop only start a local program and talk to it over standard input and output. Put a copy of the pt-tools program on the client's machine (it does not have to run the web service) and let `pt-tools mcp` forward to the running pt-tools:

```json
{
  "mcpServers": {
    "pt-tools": {
      "command": "/usr/local/bin/pt-tools",
      "args": ["mcp", "--url", "http://192.168.1.10:8080"],
      "env": { "PT_TOOLS_MCP_TOKEN": "ptt_1_xxxxxxxx" }
    }
  }
}
```

- `pt-tools mcp` opens no database and starts no scheduler; it forwards the running pt-tools' tools as they are. See the [command line](../reference/cli.md#pt-tools-mcp) for its options.
- With only a host and port, `--url` forwards to `/mcp`; after changing the path give the full address, such as `http://192.168.1.10:8080/agent/mcp`.
- Put the token in the `PT_TOOLS_MCP_TOKEN` environment variable rather than in `args`: command-line arguments show up in the process list.
- On Windows, set `command` to the full path of `pt-tools.exe`.

## Tools

| Tool                              | Permission | What it does                                                                                                                                 |
| --------------------------------- | ---------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| `list_tasks`                      | MCP read   | Fetched torrents: what RSS, subscriptions, the app and MCP pushed, filterable by site and keyword                                            |
| `list_downloader_torrents`        | MCP read   | Torrents in the downloaders, paginated, filterable by downloader, state and keyword, sortable                                                |
| `get_downloader_stats`            | MCP read   | Whether each downloader is reachable, its upload and download speed, totals and free space                                                   |
| `search_torrents`                 | MCP read   | Search the enabled sites; results carry the site and torrent ID, never a download link                                                       |
| `get_site_userinfo`               | MCP read   | Upload, billed download, ratio, bonus, seeding count, unread messages, login state and check-in per site                                     |
| `check_updates`                   | MCP read   | Whether a newer release exists; it never upgrades                                                                                            |
| `explore_media`                   | MCP read   | Find movies and TV shows on TMDB by title, or list this week's trending and popular ones                                                     |
| `list_subscriptions`              | MCP read   | Subscriptions and their progress: aired, in the library, downloading and missing episodes                                                    |
| `get_subscription`                | MCP read   | One subscription in detail: its settings, each episode's state and its torrents (how each file was organized and scraped)                    |
| `list_subscription_runs`          | MCP read   | A subscription's run history: what each search found, which torrents were rejected and why, what was pushed and what failed                  |
| `get_episode_rules`               | MCP read   | A followed show's episode-number rules: how the number in a title maps to the TMDB episode (variety shows by issue or air date)              |
| `preview_schedule`                | MCP read   | When a schedule (interval, daily, weekly, monthly, cron) would search next, without saving                                                   |
| `recognize_media`                 | MCP read   | Recognize a torrent title: parse name, year, season, episode and specs, then find the TMDB entry with your rules                             |
| `list_media_libraries`            | MCP read   | Media libraries: type, directory, organize mode and scraping                                                                                 |
| `list_media_history`              | MCP read   | Organize history: where each video went and why it was skipped or failed, filterable by status and keyword                                   |
| `preview_media_organize`          | MCP read   | Preview organizing a torrent: the recognized entry, the library and each video's target path, changing nothing                               |
| `get_config`                      | MCP read   | Read pt-tools configuration (also needs the Config scope): the catalog when no resource is given; secrets only show whether they are set     |
| `get_skill`                       | MCP read   | Built-in skills: `pt-tools-config` explains how to read and change configuration (with the catalog), `pt-tools-follow` following shows       |
| `pause_torrent`                   | MCP write  | Pause one torrent                                                                                                                            |
| `resume_torrent`                  | MCP write  | Resume one paused torrent                                                                                                                    |
| `delete_torrent`                  | MCP write  | Delete one torrent, optionally with its data                                                                                                 |
| `push_torrent`                    | MCP write  | Push a torrent to a downloader, given the site and torrent ID from a search or a download URL of a configured site                           |
| `add_subscription`                | MCP write  | Subscribe to a movie or follow a TV season by TMDB ID, optionally with a schedule, version lock, start episode or a torrent to start from    |
| `update_subscription`             | MCP write  | Change a subscription's settings (schedule, version lock, start episode, sites, downloader, library, episode count); only the fields given   |
| `set_episode_skip`                | MCP write  | Skip one episode of a followed show (not followed), or want it again                                                                         |
| `unlock_episode`                  | MCP write  | Let one episode ignore the version lock and use another version (searched right away), or put it back                                        |
| `save_episode_rule`               | MCP write  | Set a release group's episode-number rule (by issue and part, by air date, an offset)                                                        |
| `delete_episode_rule`             | MCP write  | Delete a release group's episode-number rule                                                                                                 |
| `reorganize_subscription_torrent` | MCP write  | Organize one torrent a subscription downloaded again (a season pack the same as a single episode), with the subscription's title and library |
| `rescrape_subscription_torrent`   | MCP write  | Scrape the show or movie one subscription torrent was organized into again                                                                   |
| `set_subscription_status`         | MCP write  | Pause or resume a subscription                                                                                                               |
| `search_subscription`             | MCP write  | Search for a subscription right now and download a suitable torrent when one turns up                                                        |
| `delete_subscription`             | MCP write  | Delete a subscription; downloaded torrents and library files stay                                                                            |
| `organize_media`                  | MCP write  | Organize a finished torrent into a media library: place it by the naming template, scrape, ask media servers to scan                         |
| `retry_media_organize`            | MCP write  | Organize the torrent of one organize record again (usually a failed one)                                                                     |
| `rescrape_media`                  | MCP write  | Scrape the movie or show of one organize record again                                                                                        |
| `change_config`                   | MCP write  | Change pt-tools configuration (also needs the Config scope): returns a preview first, saves only after confirmation                          |

- A tool that changes your downloaders, subscriptions, libraries or configuration does nothing when called without `confirm=true` and returns a preview instead: what would change (each item before and after) and the possible impact. The client's assistant shows you the preview and calls again with `confirm=true` only after you agree.
- `organize_media` and `preview_media_organize` take the downloader ID and `info_hash` from `list_downloader_torrents`. When the entry is recognized wrongly, pass `media_type` and `tmdb_id` (from `recognize_media` or `explore_media`); pass `library_id` to choose the library. The rules are the same as organizing by hand on the web page, see [Media library organising](media-library.md).
- `push_torrent` has pt-tools download the torrent file with its own site configuration and goes through the disk-space protection and the site seeding-capacity check as usual. It does not take magnet links and never requests any other address. Torrents pushed through MCP show the source `mcp_push` in the task list.
- The download figure `get_site_userinfo` returns (`downloaded`) is the billed download: what the site counts toward your ratio after free and discount promotions, not the amount actually transferred. See [Statistics](user-stats.md#two-kinds-of-download-figures).
- The data returned is the same as the [App API](../reference/app-api.md) returns: no cookies, passkeys, passwords, RSS addresses or PT torrent download links, and downloader error messages have the downloader's address removed. `check_updates` returns the public GitHub release pages and asset addresses.
- Invalid arguments (a missing required field, a value out of range, an unknown field) make the tool fail right away without calling pt-tools.

## Following shows

When the assistant follows a show for you, it first reads the built-in skill `pt-tools-follow` (`get_skill`): the steps, how to write schedules and cron expressions, the version lock, variety-show episode numbers, and how to retry after a failure. Some typical requests:

- "Follow season 4 of 现在就出发, searching Friday at 10 pm and Saturday morning": `explore_media` finds the TMDB ID, then `add_subscription` with a `schedule` (`{"kind":"cron","cron":"0 22 * * 5; 0 10 * * 6"}`) returns a preview for you to confirm.
- "Search every 3 hours instead": `update_subscription` changes the `schedule` (`{"kind":"interval","interval_minutes":180}`); the preview lists the next few search times.
- "Episode 5 never shows up from HHWEB": `unlock_episode` releases only that episode.
- "Why was nothing downloaded": `list_subscription_runs` shows each search; an episode number that does not match gets a rule with `save_episode_rule`, a failed push is retried with `search_subscription`, and a download that never reached the library is organized again with `reorganize_subscription_torrent`.

For the schedule options see [Follow and subscribe · When to search](media-subscribe.md#when-to-search). The subscription master switch, global search interval, default quality profile and downloader, and the Douban wish list are the `subscribe`, `quality_profiles` and `douban_sources` resources under Changing the configuration.

## Changing the configuration

With the Config scope, the assistant can read and change pt-tools' configuration with three tools:

- `get_skill`: read the built-in skill `pt-tools-config` first. It has the steps, rules and examples, and the catalog of configuration (each kind's operations, writable fields, fields required when adding, secret fields and field notes).
- `get_config`: without arguments it returns the catalog; with `resource` (such as `downloaders`) the current values of that kind, and for lists `id` narrows it to one item.
- `change_config`: `resource`, `op` (`set` for objects; `add`, `update` or `delete` for lists), `id` (for update and delete) and `values` (only the fields to change). Without `confirm=true` it returns the preview; with it the change is saved and the result is the item as it is now.

```json
{ "resource": "downloaders", "op": "update", "id": "2", "values": { "enabled": false } }
```

- A change goes through the endpoint the web settings use, so validation, encryption and hot reload work as on the web, and that endpoint's errors reach the assistant as they are.
- Secrets are write-only: passwords, cookies, API keys, passkeys, tokens, RSS URLs, and the tokens and URLs in notification channel settings read as `<已设置>` or an empty string. A new value changes a secret, giving back `<已设置>` keeps it and an empty string clears it.
- Not changeable through the configuration tools: the login password, API tokens, remote access, the encryption key, the AI assistant's own settings (including voice input), ChatOps bindings and admins, the qB-compatible entrance, and MCP access (the switch and address).
- For what can be changed see [AI assistant · Changing the configuration](assistant.md#changing-the-configuration); for the endpoints see [App API · Configuration](../reference/app-api.md#configuration).

## Audit and security

- The MCP address only accepts API tokens, not a browser session: no token or a wrong one gets 401, a token without an MCP permission gets 403. Revoking a token takes effect immediately. When you do not use MCP, turn MCP access off and the address responds with 404.
- Every run of a tool that changes your downloaders, subscriptions, libraries or configuration is recorded under ChatOps → Audit log (操作审计), including the ones refused for a missing permission or invalid arguments (previews are not recorded): the channel is MCP, the command is the tool name and the user is the token ID. `change_config` only records which kind of configuration and which fields changed, not the values.
- Over the internet, use HTTPS (for example behind a reverse proxy), or the token crosses the network in plain text. The reverse proxy has to keep the `Authorization` header.
