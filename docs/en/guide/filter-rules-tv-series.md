# Filter rules and TV series

This page explains how to use pt-tools' filter rules to follow a series automatically, download particular releases and handle other advanced cases.

## What filter rules are for

> [!WARNING]
> **Most people need no filter rules.** When no filter rules are attached, an RSS subscription **downloads freeleech torrents only**. If all you want is to build ratio, add the subscription without creating any rules. Filter rules are for one situation: you want to follow a series or download particular releases, and you want them downloaded even when they are not free. Then create a filter rule, turn off its Free only (仅免费) option and attach the rule to the RSS subscription.

Filter rules are one of pt-tools' core features. They narrow down the torrents of an RSS subscription so that you can:

- **Follow a series**: download only the latest episodes of a particular show
- **Choose the quality**: download only a resolution such as 4K or 1080p
- **Choose the source**: download only releases from particular subtitle or release groups
- **Prefer freeleech**: by default only freeleech torrents are downloaded, which saves download allowance

**What sets it apart**: you can have torrents that match your rules downloaded **even when they are not free**, so you never miss an update of something you follow.

> [!NOTE]
> When a subscription's download mode is the default Smart (智能), attaching filter rules means that **only torrents matching the rules are downloaded**; unmatched freeleech torrents are no longer downloaded automatically. To follow a series and keep downloading freeleech torrents, add a separate subscription for the series. The download modes are described in [RSS subscriptions](rss-subscription.md).

## Pattern types

pt-tools supports three pattern types, from simple to complex. All three ignore case, and a pattern matches when **any part** of the title (or tag) fits it; the whole title does not have to match from start to end.

| Type                   | Difficulty | Good for                 | Example                 |
| ---------------------- | ---------- | ------------------------ | ----------------------- |
| **Keyword**            | Easy       | Containing a given word  | `REMUX`, `Black.Mirror` |
| **Wildcard**           | Medium     | Simple patterns          | `*.2024.*`, `S01E??`    |
| **Regular expression** | Hard       | Precise, complex matches | `S\d{2}E\d{2}`          |

### Keywords

The simplest type: the rule matches when the title or the tag contains the keyword.

**How it matches:**

- Case is ignored.
- The whole text is **one** keyword: `4K,2160p` matches only a title that contains `4K,2160p` exactly; it is not split into two keywords.
- When you want "any one of these words", use a regular expression with the words separated by `|`, such as `4K|2160p|UHD`.

**Example:**

```
REMUX
```

Result: every torrent whose title contains `REMUX` (or `remux`) matches.

### Wildcards

Patterns with `*` and `?`, more flexible than keywords.

**Syntax:**

- `*` matches any number of any characters
- `?` matches exactly one character

**Examples:**

| Pattern              | Meaning                 | Matches                  |
| -------------------- | ----------------------- | ------------------------ |
| `*.2024.*`           | Contains the year 2024  | `Movie.2024.1080p`       |
| `S01E??`             | Any episode of season 1 | `S01E01`, `S01E12`       |
| `*REMUX*`            | Contains REMUX          | `Movie.REMUX.2160p`      |
| `Show.Name.S??E??.*` | Any season, any episode | `Show.Name.S02E05.1080p` |

### Regular expressions

The most powerful type, for complex patterns.

**Common syntax:**

| Syntax   | Meaning            | Example                    |
| -------- | ------------------ | -------------------------- |
| `\d`     | A digit            | `\d` matches `0-9`         |
| `\d{2}`  | Two digits         | `\d{2}` matches `01`, `99` |
| `\d+`    | One or more digits | `\d+` matches `1`, `123`   |
| `.*`     | Anything           | `A.*B` matches `A123B`     |
| `(A\|B)` | A or B             | `(4K\|2160p)`              |
| `^`      | The start          | `^Movie`                   |
| `$`      | The end            | `1080p$`                   |

**Common patterns:**

| Purpose        | Regular expression  | Matches              |
| -------------- | ------------------- | -------------------- |
| Episode number | `S\d{2}E\d{2}`      | `S01E01`, `S02E15`   |
| 4K releases    | `(4K\|2160p\|UHD)`  | `4K`, `2160p`        |
| A given season | `S01E\d{2}`         | `S01E01` to `S01E99` |
| Year           | `\.(19\|20)\d{2}\.` | `.2024.`, `.1999.`   |

## Creating a rule

Under Rules → Filter rules (规则 → 过滤规则), click Add rule (添加规则) and fill in these fields:

| Field                                             | Meaning                                                                                | Default       |
| ------------------------------------------------- | -------------------------------------------------------------------------------------- | ------------- |
| **Name (名称)**                                   | A name for the rule, such as "Series - show name"                                      | Required      |
| **Priority (优先级)**                             | Rules with a smaller number are checked first                                          | `100`         |
| **Pattern type (模式类型)**                       | Keyword, wildcard or regular expression                                                | Keyword       |
| **Match in (匹配范围)**                           | Title only, tag only, or title and tag                                                 | Title and tag |
| **Pattern (匹配模式)**                            | What to match, written for the pattern type; Templates (常用模板) fills in an example  | Required      |
| **Minimum and maximum size (最小大小、最大大小)** | The size range in GB; 0 means no limit, and the maximum cannot exceed the global limit | `0`           |
| **Purpose (规则用途)**                            | Controls downloading, notifications, or both                                           | Download      |
| **Free only (仅免费)**                            | When on, the rule lets through freeleech torrents only                                 | On            |
| **Enabled (启用规则)**                            | When off, the rule is kept but takes no part in matching                               | On            |

**Free only:**

- **On**: downloads freeleech torrents only; suited to everyday ratio building
- **Off**: downloads even when the torrent is not free; suited to things you follow and do not want to miss

Once the rule exists, edit the subscription under the RSS subscriptions (RSS 订阅) tab of the site's page and select the rule under Filter rules (过滤规则); only then does it take effect.

## Examples for following series

### Example 1: following one series

**Situation**: you follow the first season of a US series and do not want to miss an episode.

**Settings:**

```
Name: Series - US show S01
Pattern type: Regular expression
Pattern: Show.Name.*S01E\d{2}
Match in: Title only
Free only: Off
```

**Notes:**

- `Show.Name.*S01E\d{2}` matches titles such as `Show.Name.S01E01.1080p.WEB-DL`
- Turning Free only off makes sure episodes are downloaded even when they are not free

### Example 2: 4K releases only

**Situation**: you only want films in 4K/2160p.

**Settings:**

```
Name: 4K films
Pattern type: Regular expression
Pattern: 4K|2160p|UHD
Match in: Title only
Free only: On
```

**Notes:**

- Separate the alternatives with `|`; the rule matches when any of them appears. The 4K template under Templates is written the same way.
- The keyword type does not split on commas: `4K,2160p,UHD` as a keyword matches nothing.

### Example 3: a particular subtitle or release group

**Situation**: you only want releases from particular groups.

**Settings:**

```
Name: Chosen group
Pattern type: Wildcard
Pattern: *SubGroup*
Match in: Title only
Free only: On
```

**Or with a regular expression:**

```
Name: Chosen groups
Pattern type: Regular expression
Pattern: (SubGroup1|SubGroup2|SubGroup3)
Match in: Title only
Free only: On
```

### Example 4: leaving something out

**Situation**: you want to leave out torrents tagged HDSWEB.

**Approach**: pt-tools' filter rules only include what matches. To leave something out:

1. **Filter in the site's RSS**: when you generate the RSS link, leave out the categories you do not want
2. **Match precisely**: match only what you want, instead of excluding what you do not

**Example**: WEB-DL releases but not HDTV:

```
Name: WEB-DL only
Pattern type: Regular expression
Pattern: WEB-DL|WEBDL|WEBRip
Match in: Title only
Free only: On
```

### Example 5: several conditions together

**Situation**: you follow the 4K version of a series released by a particular group.

**Settings:**

```
Name: Show - 4K - group
Pattern type: Regular expression
Pattern: Show.Name.*S01E\d{2}.*(2160p|4K).*SubGroup
Match in: Title only
Free only: Off
```

**Notes:**

- A regular expression can combine several conditions
- `.*` matches anything in between

## How a torrent is judged

```
The RSS subscription fetches a new torrent
        |
        v
+-----------------------------+
| Check the subscription's     |
| rules, smallest priority     |
| number first                 |
+-----------------------------+
        |
        v
  Does any rule match?
    /          \
  Yes           No
   |             |
   v             v
+--------------------------+  +-------------------------+
| Only the first matching  |  | Handled by the          |
| rule counts:             |  | subscription's download |
| - Free only, not free?   |  | mode (skipped in Smart  |
| - Size outside its range?|  | mode once rules are     |
+--------------------------+  | attached)               |
   |                          +-------------------------+
 Either is yes → the torrent is skipped
 Both are no   → it is pushed to the downloader
```

The maximum and minimum torrent sizes in the global settings are checked before the rules; a torrent outside them is skipped at once.

## Regular expression cheat sheet

### Episodes

| Need                      | Regular expression      |
| ------------------------- | ----------------------- |
| Any episode               | `S\d{2}E\d{2}`          |
| Season 1                  | `S01E\d{2}`             |
| Season 1, episodes 1 to 9 | `S01E0[1-9]`            |
| Season 1, episode 10 on   | `S01E(1[0-9]\|[2-9]\d)` |
| A complete season         | `S\d{2}\.Complete`      |

### Quality

| Need            | Regular expression           |
| --------------- | ---------------------------- |
| 4K              | `(4K\|2160p\|UHD)`           |
| 1080p           | `1080p`                      |
| High definition | `(1080p\|2160p\|4K)`         |
| REMUX           | `REMUX`                      |
| Blu-ray         | `(BluRay\|Blu-ray\|BDREMUX)` |

### Source

| Need         | Regular expression                |
| ------------ | --------------------------------- |
| WEB releases | `(WEB-DL\|WEBDL\|WEBRip)`         |
| HDTV         | `HDTV`                            |
| Not HDTV     | `WEB` (matches WEB releases only) |

## Good practice

### 1. Name rules clearly

Clear names make rules easier to manage:

- `Series-name-season`
- `4K-films`
- `Group-name`

### 2. Set priorities on purpose

Rules with a smaller priority number are checked first, and only the first rule that matches decides:

- Small numbers: things you follow (the series you are watching, for example)
- Middle numbers: quality rules
- Large numbers: general rules

### 3. Build rules up step by step

1. Start with a simple keyword rule
2. Refine it once it matches what you expect
3. Use a regular expression for precise control

### 4. Maintain them

- A series has ended: disable or delete its rule
- A new season starts: update the season in the rule
- Check what the rules match: make sure nothing unwanted gets through

### 5. Combine with the site's RSS filters

Filter roughly in the site's RSS settings first:

- Choose categories (films, TV series, anime)
- Limit the number of items returned

Then filter precisely with pt-tools' rules.

## Questions

### Q: Why does my rule not work?

A: Check the following:

1. Is the rule enabled, and is it attached to the RSS subscription?
2. Is the pattern right (watch the escape characters; the keyword type does not split on commas)?
3. Is Match in set correctly (title or tag)?
4. Does the torrent's title really fit the rule?
5. Did a rule with a smaller priority number match first and turn the torrent away because of Free only or its size range?

### Q: How do I debug a regular expression?

A: Suggestions:

1. When adding or editing a rule, choose an RSS subscription under Dry run (试跑) and click Test match (测试匹配) to see whether the results are what you expect
2. Start with a simple pattern and add complexity step by step
3. Check pt-tools' logs to see what matched

### Q: How do several rules work together?

A: When a subscription has several rules attached:

- They are checked from the smallest priority number up; a torrent that matches any of them may be downloaded
- The first rule that matches decides; later rules are not checked
- If that rule requires freeleech and the torrent is not free, or the size is outside its range, the torrent is skipped

### Q: How are duplicate downloads avoided?

A: pt-tools records the torrents it has seen:

- The same torrent is never downloaded twice
- Torrents are identified by site and torrent ID
- The history is kept in the database

### Q: How much allowance do downloads that are not free use?

A: It depends on the site's rules and the size of the torrents:

- Make sure you have enough download allowance
- Set a maximum torrent size
- When following a series, one episode is usually 2 to 10 GB

---

More settings are described in [Configuration](../configuration.md).
