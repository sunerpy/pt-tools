# Media recognition <Badge type="tip" text="1.0 preview" />

pt-tools parses torrent titles and finds the matching movie or TV show on TMDB. [Media library organising](media-library.md) names files from the results, and subscriptions will use them later; in Media → Media recognition (媒体 → 媒体识别) you can preview and correct them, and keep recognition words.

Movie and TV data and images come from [TMDB](https://www.themoviedb.org/). This application uses TMDB and the TMDB APIs but is not endorsed, certified, or otherwise approved by TMDB.

## TMDB API key

Released builds (the binaries in a Release and the Docker images) have a TMDB API key built in, so recognition, organising and subscriptions work without entering one; under API key in TMDB settings (TMDB 设置) the page says the built-in key is in use. You can still enter your own key, and then yours is used:

1. Sign in to TMDB and request a key on the API page of your account settings (free for personal use). Both a v3 API key and a v4 API read access token work.
2. Enter the key in TMDB settings, choose a language, save, and click Test connection (测试连接).

You need your own key when:

- you built pt-tools from source: such builds have no built-in key;
- you replaced the TMDB API address with a mirror or your own relay: the built-in key is shared by everyone, so it is only sent to TMDB's official addresses (`api.themoviedb.org` and `api.tmdb.org`), never anywhere else;
- the built-in key stops working, or you prefer to use your own.

Without a usable key, the explore, subscriptions and media library pages show a notice at the top with a link to this page.

| Setting                       | Meaning                                                                                                                                               |
| ----------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| API key                       | Stored encrypted; the page only shows whether one is set. Enter a new one to replace it, or click Clear (清除) to go back to the built-in key         |
| Language                      | Simplified Chinese (default), Traditional Chinese, English or Japanese; English overviews fill in missing ones                                        |
| Proxy address (optional)      | Proxy for TMDB requests: http, https or socks5. Left empty, the proxy from the environment is used, as for sites                                      |
| TMDB API address (optional)   | Replaces the official `https://api.themoviedb.org/3`, for example with `api.tmdb.org` or your own relay; a bare domain gets `https://` and `/3` added |
| TMDB image address (optional) | Replaces the official `https://image.tmdb.org/t/p/`, for example with an image mirror; a bare domain gets `https://` and `/t/p/` added                |

The proxy address may include a user name and password. The whole address is stored encrypted, and the page shows the password as `***`. TMDB usually needs a proxy from mainland China; when the official addresses are unreachable or slow you can also replace the API and image addresses, with or without the proxy.

Images downloaded for scraping, the poster links in organizing and subscription notifications, and the posters shown in the web UI and the mobile app all use the image address. pt-tools fetches scraping images and the app's posters itself, through the proxy above; the browser loads the web UI's posters straight from the image address, and the device that receives a notification opens its poster. If you point the image address at a relay on your LAN, make sure the browser and those devices can reach it too. When you open the web UI over https, the image address must use https as well: browsers don't load http images on an https page (the settings page warns about this).

## Recognition preview

In Recognition preview (识别预览), enter a title (the subtitle and IMDb ID are optional) and click Recognize (识别).

The result lists the Chinese and English names, year, type, season and episode, resolution, source, codec, HDR, audio, release group, and flags such as Chinese subtitles, Mandarin or Cantonese. Chinese names and markers such as 第 1-10 集 or 全 30 集 in the subtitle are read too.

pt-tools looks for the TMDB entry in this order:

1. A corrected name uses the entry it was corrected to.
2. With an IMDb ID, it looks the ID up.
3. It searches by the Chinese and the English name and accepts a result only when the name matches and the year fits (later seasons of a show may be later than its first air year). When the name matches only partly, sequel numbers must agree (Inside Out is not taken for Inside Out 2) and the result must clearly beat the next candidate. A title with a year but no season or episode marker is searched as a movie first, then as a show when no movie matches. When the torrent uses an English or romanised name (Your Name, Sousou no Frieren, Dragon Raja) that the Chinese search results do not show, the English titles, aliases and translated titles of the top candidates on TMDB are compared too; a candidate is accepted only when the name is the same and the year fits. When the English title carries a subtitle (Dragon Raja's is "Dragon Raja -The Blazing Dawn-"), the plain "Dragon Raja" in torrent names is often only in the French, German or other translations.

When nothing is accepted, the page lists candidates. Click Pick this (选这个), or correct it by TMDB ID. The ID is in the TMDB page address, such as `278` in `themoviedb.org/movie/278`.

Search results are cached for a day and entry details for a week. Without a usable API key, only the title is parsed.

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

pt-tools records the IMDb and Douban IDs a site provides: from the M-Team API, and from the IMDb and Douban links on NexusPHP detail pages. RSS downloads and brush pushes store them with the torrent: library organising looks the ID up first, and subscriptions will use it later. An IMDb ID entered in the recognition preview is also looked up first.

Douban IDs are used for Douban wishlist (想看) subscriptions, and, when a library's plot and rating come from Douban, scraping uses them to fetch Douban's plot and rating (see [scraping](media-library.md#scraping)).
