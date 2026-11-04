package v2

import (
	"bytes"
	"context"
	"fmt"
	"mime"
	"net/url"
	"path"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// NexusPHP 的种子字幕（按 NexusPHP 公开源码 details.php 与 downloadsubs.php 的写法，php8 分支与 1.5 相同）：
//   - 一个种子的字幕列在 details.php?id=<id> 的「字幕」一行：每份一行，第一格是国旗图（alt、title 是语言名，src 是 pic/flag/<图>），
//     第二格是 downloadsubs.php?torrentid=<种子>&subid=<字幕> 的链接（链接文字是标题）；没有字幕时写「该种子暂无字幕」。
//   - downloadsubs.php 要登录，回文件本身，文件名在 Content-Disposition 里（Chrome 一类 UA 是原样的、加引号的文件名，
//     别的 UA 是 rawurlencode 过的）；字幕或文件不在了回纯文本「Not found」「File not found」。
//
// 站点改过详情页、页面上没有这种链接时取到空列表。

// nexusSubtitleMissing 是 downloadsubs.php 找不到字幕时回的话（die 的原文，后面跟换行）。
var nexusSubtitleMissing = []string{"Not found", "File not found", "File name missing", "Cannot open file"}

// nexusNumericID 报告 s 是不是 NexusPHP 的编号（正整数，intval 认的写法）。
func nexusNumericID(s string) bool {
	if s == "" || len(s) > 20 || s[0] == '0' {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// ListSubtitles 读种子详情页「字幕」一行里的字幕。编号不是数字时不发请求（RSS 的 guid 是 info hash，取不到详情）。
func (d *NexusPHPDriver) ListSubtitles(ctx context.Context, torrentID string) ([]Subtitle, error) {
	torrentID = strings.TrimSpace(torrentID)
	if !nexusNumericID(torrentID) {
		return nil, fmt.Errorf("%w：%q 不是 NexusPHP 的种子编号", ErrInvalidTorrentID, torrentID)
	}
	req, err := d.PrepareDetail(torrentID)
	if err != nil {
		return nil, err
	}
	res, err := d.Execute(ctx, req)
	if err != nil {
		return nil, err
	}
	if res.Document == nil {
		return nil, ErrParseError
	}
	if text, missing := nexusTorrentMissing(res.Document); missing {
		return nil, fmt.Errorf("%w（详情页：%s）", ErrTorrentNotFound, text)
	}
	return parseNexusSubtitles(res.Document), nil
}

// parseNexusSubtitles 取页面里所有 downloadsubs.php 的链接（按字幕编号去重）：编号从链接取，语言取同一行国旗图的 alt 或 title。
func parseNexusSubtitles(doc *goquery.Document) []Subtitle {
	out := []Subtitle{}
	seen := map[string]bool{}
	doc.Find(`a[href*="downloadsubs.php"]`).Each(func(_ int, a *goquery.Selection) {
		href, _ := a.Attr("href")
		u, err := url.Parse(strings.TrimSpace(href))
		// 链接写成绝对地址时也认（下载时按站点地址拼，只用这两个编号）
		if err != nil || path.Base(u.Path) != "downloadsubs.php" {
			return
		}
		q := u.Query()
		sub := Subtitle{ID: q.Get("subid"), TorrentID: q.Get("torrentid")}
		if !nexusNumericID(sub.ID) || !nexusNumericID(sub.TorrentID) || seen[sub.ID] {
			return
		}
		seen[sub.ID] = true
		sub.Title = strings.Join(strings.Fields(a.Text()), " ")
		if img := a.Closest("tr").Find(`img[src*="flag/"]`).First(); img.Length() > 0 {
			sub.Language = strings.TrimSpace(img.AttrOr("alt", ""))
			if sub.Language == "" {
				sub.Language = strings.TrimSpace(img.AttrOr("title", ""))
			}
			if src := img.AttrOr("src", ""); !strings.HasSuffix(src, "/") {
				sub.Flag = path.Base(src)
			}
		}
		out = append(out, sub)
	})
	return out
}

// DownloadSubtitle 下载列表里的一份字幕（downloadsubs.php?torrentid=&subid=）。
func (d *NexusPHPDriver) DownloadSubtitle(ctx context.Context, sub Subtitle) (SubtitleFile, error) {
	if !nexusNumericID(sub.ID) || !nexusNumericID(sub.TorrentID) {
		return SubtitleFile{}, fmt.Errorf("%w：字幕 %q（种子 %q）", ErrInvalidTorrentID, sub.ID, sub.TorrentID)
	}
	params := url.Values{}
	params.Set("torrentid", sub.TorrentID)
	params.Set("subid", sub.ID)
	res, err := d.Execute(ctx, NexusPHPRequest{Path: "/downloadsubs.php", Params: params, Method: "GET"})
	if err != nil {
		return SubtitleFile{}, err
	}
	body := bytes.TrimSpace(res.RawBody)
	if len(body) == 0 {
		return SubtitleFile{}, fmt.Errorf("%w：站点回的是空的", ErrSubtitleNotFound)
	}
	if len(body) < 64 {
		for _, m := range nexusSubtitleMissing {
			if string(body) == m {
				return SubtitleFile{}, fmt.Errorf("%w：站点回「%s」", ErrSubtitleNotFound, m)
			}
		}
	}
	return SubtitleFile{Name: dispositionName(res.Header.Get("Content-Disposition")), Data: res.RawBody}, nil
}

// dispositionName 是 Content-Disposition 里的文件名（只留最后一段）：认 filename*、加引号的原样文件名（后面跟「 ; charset=utf-8」
// 也认）与 rawurlencode 过的文件名。没有时为空。
func dispositionName(v string) string {
	if v == "" {
		return ""
	}
	name := ""
	if _, params, err := mime.ParseMediaType(v); err == nil {
		name = params["filename"]
	} else if _, after, ok := strings.Cut(v, "filename="); ok {
		name = strings.Trim(strings.TrimSpace(strings.Split(after, ";")[0]), `"`)
	}
	if strings.Contains(name, "%") {
		if un, err := url.PathUnescape(name); err == nil {
			name = un
		}
	}
	name = strings.ReplaceAll(name, `\`, "/")
	name = strings.TrimSpace(path.Base(name))
	if name == "." || name == "/" || name == ".." {
		return ""
	}
	return name
}
