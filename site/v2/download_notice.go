package v2

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// ErrNotTorrentFile 是下载到的不是种子文件：站点回了网页（登录页、两步验证页、「下载提示」页……）、JSON 或一段文字。
// 以前这些内容原样当成种子交给调用方，辅种因此报「下载到的种子与 IYUU 给的 info hash 不符」（2026-11-02 织梦）。
var ErrNotTorrentFile = errors.New("站点返回的不是种子文件")

// 上游 NexusPHP 用 Cookie 下载（download.php?id=，不带 passkey、downhash）时，账号开着「第一次下载提示」、被查出用了禁止的
// 客户端、或在分享率警告期，就先跳到 downloadnotice.php?torrentid=…&type=firsttime|client|ratio 的提示页，不给种子文件；
// 地址带上 letdown=1 就直接给，提示页上「下载种子文件」按钮提交以后也是跳到这个地址（xiaomlove/nexusphp 的
// public/download.php、public/downloadnotice.php）。提示页的表单里有隐藏的 type 与「下次不显示此提示」的 hidenotice 勾选框。
const (
	noticeFirstTime = "firsttime"
	noticeClient    = "client"
	noticeRatio     = "ratio"
)

// downloadNoticeError 是下载时站点回了「下载提示」页。
type downloadNoticeError struct {
	noticeType string
}

func (e *downloadNoticeError) Error() string {
	switch e.noticeType {
	case noticeClient:
		return ErrNotTorrentFile.Error() + "，是「被禁止客户端提示」页：站点发现你用了它禁止的 BT 客户端，" +
			"换成站点允许的客户端，再在站点网页上下载一次、确认提示"
	case noticeRatio:
		return ErrNotTorrentFile.Error() + "，是「低分享率提示」页：账号在分享率警告期，站点要每次下载都在网页上确认，" +
			"pt-tools 不替你确认；分享率改善以后再试"
	default:
		return ErrNotTorrentFile.Error() + "，是「第一次下载提示」页：在站点网页上下载一次，提示页上保持勾选「下次不显示此提示」再点「下载种子文件」"
	}
}

func (e *downloadNoticeError) Unwrap() error { return ErrNotTorrentFile }

// notTorrentError 是下载到的不是种子文件，why 接在 ErrNotTorrentFile 的话后面，说明站点回的是什么；
// 登录页、两步验证页另外让 errors.Is 认得 ErrSessionExpired、Err2FARequired。
type notTorrentError struct {
	why   string
	cause error
}

func (e *notTorrentError) Error() string { return ErrNotTorrentFile.Error() + e.why }

func (e *notTorrentError) Unwrap() []error {
	if e.cause == nil {
		return []error{ErrNotTorrentFile}
	}
	return []error{ErrNotTorrentFile, e.cause}
}

// CheckTorrentFile 检查下载到的是不是种子文件（bencode 的字典，以 d 开头）：是就返回 nil；不是时返回包着 ErrNotTorrentFile 的错误，
// 写明站点回的是什么。登录页另外包着 ErrSessionExpired，两步验证页包着 Err2FARequired。
func CheckTorrentFile(data []byte) error {
	if len(data) > 0 && data[0] == 'd' {
		return nil
	}
	if len(data) == 0 {
		return &notTorrentError{why: "（内容是空的）"}
	}
	if body := bytes.TrimSpace(data); len(body) > 0 && body[0] == '<' {
		if doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body)); err == nil {
			return notTorrentPage(doc, len(data))
		}
	}
	return &notTorrentError{why: fmt.Sprintf("（%d 字节：%s）", len(data), sanitizeRawURLs(torrentResponsePreview(data)))}
}

// notTorrentPage 说明站点回的是哪一页。
func notTorrentPage(doc *goquery.Document, size int) error {
	if t, ok := downloadNoticeType(doc); ok {
		return &downloadNoticeError{noticeType: t}
	}
	title := ""
	if t := pageTitle(doc); t != "" {
		title = "（页面标题：" + t + "）"
	}
	switch {
	case isLoginPage(doc):
		return &notTorrentError{why: "，是登录页：Cookie 失效或没有填写" + title, cause: ErrSessionExpired}
	case is2FAPage(doc):
		return &notTorrentError{why: "，是两步验证页：这份 Cookie 要先在站点上输两步验证码" + title, cause: Err2FARequired}
	case title != "":
		return &notTorrentError{why: title}
	}
	return &notTorrentError{why: fmt.Sprintf("（一个 %d 字节的网页）", size)}
}

// downloadNoticeType 认出上游 NexusPHP 的「下载提示」页：表单里有「下次不显示此提示」的 hidenotice 勾选框，
// 隐藏的 type 是 firsttime、client 或 ratio。不是这种页面时 ok 为假。
func downloadNoticeType(doc *goquery.Document) (string, bool) {
	found := ""
	doc.Find("form").EachWithBreak(func(_ int, f *goquery.Selection) bool {
		if f.Find("input[name='hidenotice']").Length() == 0 {
			return true
		}
		switch t := strings.TrimSpace(f.Find("input[name='type']").AttrOr("value", "")); t {
		case noticeFirstTime, noticeClient, noticeRatio:
			found = t
			return false
		}
		return true
	})
	return found, found != ""
}

// withLetDown 在 download.php 的下载地址后面加上 letdown=1（原来的参数原样留着）；不是 download.php、已经带了 letdown=1
// 或者地址不对时 ok 为假。
func withLetDown(downloadURL string) (string, bool) {
	u, err := url.Parse(downloadURL)
	if err != nil || !strings.EqualFold(path.Base(u.Path), "download.php") || u.Query().Get("letdown") == "1" {
		return "", false
	}
	if u.RawQuery == "" {
		u.RawQuery = "letdown=1"
	} else {
		u.RawQuery += "&letdown=1"
	}
	return u.String(), true
}

// fetchSkippingFirstNotice 用 fetch 拉种子文件；站点先回了「第一次下载提示」页时，照提示页上「下载种子文件」按钮的做法
// 带上 letdown=1 再拉一次（只这一次）。不往提示页提交表单，不改账号上「下次不显示此提示」的设置；
// 被禁止客户端、低分享率的提示不跳过，原样报给调用方：那是账号的状态，要用户自己处理。
func fetchSkippingFirstNotice(downloadURL string, fetch func(string) ([]byte, error)) ([]byte, error) {
	data, err := fetch(downloadURL)
	var notice *downloadNoticeError
	if !errors.As(err, &notice) || notice.noticeType != noticeFirstTime {
		return data, err
	}
	letDown, ok := withLetDown(downloadURL)
	if !ok {
		return data, err
	}
	return fetch(letDown)
}
