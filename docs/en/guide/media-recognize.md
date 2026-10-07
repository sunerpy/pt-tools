# Media recognition

pt-tools parses torrent titles and finds the matching movie or TV show on TMDB. The results feed the library organisation and subscriptions that come later; for now you can preview and correct them, and keep recognition words, in Media → Media recognition (媒体 → 媒体识别).

Movie and TV data comes from [TMDB](https://www.themoviedb.org/). This product uses the TMDB API but is not endorsed or certified by TMDB.

## Enter a TMDB API key

1. Sign in to TMDB and request a key on the API page of your account settings (free for personal use). Both a v3 API key and a v4 API read access token work.
2. Enter the key in TMDB settings (TMDB 设置), choose a language, save, and click Test connection (测试连接).

| Setting                  | Meaning                                                                                                          |
| ------------------------ | ---------------------------------------------------------------------------------------------------------------- |
| API key                  | Stored encrypted; the page only shows whether one is set. Enter a new one to replace it, or click Clear (清除)   |
| Language                 | Simplified Chinese (default), Traditional Chinese, English or Japanese; English overviews fill in missing ones   |
| Proxy address (optional) | Proxy for TMDB requests: http, https or socks5. Left empty, the proxy from the environment is used, as for sites |

The proxy address may include a user name and password. The whole address is stored encrypted, and the page shows the password as `***`. TMDB usually needs a proxy from mainland China.

## Recognition preview

In Recognition preview (识别预览), enter a title (the subtitle and IMDb ID are optional) and click Recognize (识别).

The result lists the Chinese and English names, year, type, season and episode, resolution, source, codec, HDR, audio, release group, and flags such as Chinese subtitles, Mandarin or Cantonese. Chinese names and markers such as 第 1-10 集 or 全 30 集 in the subtitle are read too.

pt-tools looks for the TMDB entry in this order:

1. A corrected name uses the entry it was corrected to.
2. With an IMDb ID, it looks the ID up.
3. It searches by the Chinese and the English name and accepts a result only when the name matches and the year fits (later seasons of a show may be later than its first air year). When the name matches only partly, sequel numbers must agree (Inside Out is not taken for Inside Out 2) and the result must clearly beat the next candidate. A title with a year but no season or episode marker is searched as a movie first, then as a show when no movie matches.

When nothing is accepted, the page lists candidates. Click Pick this (选这个), or correct it by TMDB ID. The ID is in the TMDB page address, such as `278` in `themoviedb.org/movie/278`.

Search results are cached for a day and entry details for a week. Without an API key, only the title is parsed.

## Corrections

A correction applies to the parsed name (plus the year for movies, not for shows): other titles with the same name, such as a different resolution or release group, are recognized as the corrected entry. It is keyed by the English name when there is one, with the Chinese name as an alias, so it applies whether or not the subtitle has the Chinese name. Delete it under Corrections (手动纠正) to search by name again.

## Recognition words

Recognition words are applied in order before a title is parsed, to handle the naming habits of a site or release group.

| Kind           | Effect                                                                                                                                     |
| -------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| Block          | Removes the matching text from the title and subtitle, such as `[禁转]`                                                                    |
| Replace        | Replaces it with other text, such as the short form `TSR` with `The.Shawshank.Redemption`                                                  |
| Episode offset | When the title or subtitle matches, adds the offset (may be negative) to the episode numbers. Use `-12` when season 2 starts at episode 13 |

Plain text matches ignore case; with regex matching (按正则匹配) the pattern uses Go (RE2) syntax.

## IMDb and Douban IDs

pt-tools records the IMDb and Douban IDs a site provides: from the M-Team API, and from the IMDb and Douban links on NexusPHP detail pages. RSS downloads and brush pushes store them with the torrent, so that the upcoming library organisation and subscriptions can recognize torrents by ID. An IMDb ID entered in the recognition preview is also looked up first.

Douban IDs are for the upcoming Douban wishlist (想看) subscriptions; pt-tools does not call Douban's API.
