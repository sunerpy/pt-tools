package organize

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"

	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// 站点字幕：媒体库开了「下载站点字幕」时，种子里没有字幕、种子是 pt-tools 从支持字幕的站点（NexusPHP）推送的，
// 整理出新视频以后到种子详情页取站点上的字幕，放进库里视频的旁边（见 siteSubtitles）。

// extraSiteSub 是整理记录 extras 里从站点下载、pt-tools 写进库的字幕：普通文件，按文件编号认（删除时同刮削写的文件）。
const extraSiteSub = "site_sub"

// 站点字幕的上限。
const (
	siteSubDownloads = 10       // 一个种子最多下载几份
	siteSubPerVideo  = 4        // 一个视频最多放几个
	siteSubZipFiles  = 50       // 一个压缩包里最多取几个字幕文件
	siteSubZipScan   = 1000     // 一个压缩包里最多看几项
	siteSubFileMax   = 10 << 20 // 一个字幕文件最大
	siteSubZipTotal  = 50 << 20 // 一个压缩包解出来的字幕合计最大
	siteSubNames     = 9        // 同一种语言的名字被占着时往后试到第几个（.zh-CN.9.srt）
	siteSubTimeout   = 2 * time.Minute
)

// SubtitleSites 按站点 ID 给出能列出、下载种子字幕的站点（cmd 里接 UserInfoService 的共享站点实例，限速与站点别的请求共用）；
// 站点没有注册或不支持字幕时为 nil。
type SubtitleSites func(siteID string) v2.SubtitleProvider

// siteLangWords 是站点列表上语言名字里的写法（转小写后找）：繁体在中文前面（「繁體中文」里也有「中文」）。
var siteLangWords = []struct {
	tag   string
	words []string
}{
	{"zh-CN", []string{"简体", "簡體", "简中", "simplified", "chs"}},
	{"zh-TW", []string{"繁体", "繁體", "繁中", "traditional", "cht"}},
	{"zh", []string{"中文", "中字", "chinese"}},
	{"en", []string{"english", "英文", "英语", "英語"}},
	{"ja", []string{"日本語", "日语", "日語", "日文", "japanese"}},
	{"ko", []string{"한국어", "韩语", "韓語", "韩文", "韓文", "korean"}},
}

// siteFlagLangs 是国旗图的名字（去掉扩展名、转小写）对应的语言：NexusPHP 默认的语言表是 china、hongkong、uk、japan、southkorea。
var siteFlagLangs = map[string]string{
	"china": "zh-CN", "chn": "zh-CN", "cn": "zh-CN",
	"hongkong": "zh-TW", "hk": "zh-TW", "taiwan": "zh-TW", "tw": "zh-TW",
	"uk": "en", "usa": "en", "us": "en", "england": "en", "gb": "en",
	"japan": "ja", "jp": "ja",
	"southkorea": "ko", "korea": "ko", "kr": "ko",
}

// listedLang 是站点列表上这份字幕的语言：先看写的语言名，再看国旗图，最后看标题里的写法；认不出时为空。
func listedLang(sub v2.Subtitle) string {
	name := strings.ToLower(sub.Language)
	for _, l := range siteLangWords {
		for _, w := range l.words {
			if strings.Contains(name, w) {
				return l.tag
			}
		}
	}
	flag := strings.ToLower(strings.TrimSuffix(sub.Flag, path.Ext(sub.Flag)))
	if tag, ok := siteFlagLangs[flag]; ok {
		return tag
	}
	return transfer.SubtitleLang(sub.Title)
}

// listedSub 是站点列表上要下载的一份字幕与它的语言。
type listedSub struct {
	sub  v2.Subtitle
	lang string
}

// titleArchive 报告列表上的标题是不是压缩包，或者看不出是什么（不是字幕文件名）：这种要下载下来才知道里面是什么。
func titleArchive(title string) bool {
	return !transfer.IsSubtitle(title)
}

// pickSiteSubs 挑要下载的字幕：有几个视频时，标题是字幕文件名的要能按文件名或季集对上一个视频、而且 eligible 说这个视频要放
// （eligible 为空时都要；压缩包与看不出是什么的照下）；剩下的有中文的只要中文的；按语言排（简体 → 繁体 → 中文 → 英 → 日 → 韩
// → 认不出的），最多 siteSubDownloads 份。videos 是这个种子里所有的视频（对得上的视频可能以前就整理过，这次不放）。
func pickSiteSubs(list []v2.Subtitle, videos []transfer.File, key func(transfer.File) string, eligible func(int) bool) []listedSub {
	var out []listedSub
	for _, sub := range list {
		if len(videos) > 1 && !titleArchive(sub.Title) {
			groups := transfer.MatchSubtitles(videos, []transfer.File{{Rel: sub.Title}}, key)
			i := slices.IndexFunc(groups, func(g []transfer.File) bool { return len(g) > 0 })
			if i < 0 || (eligible != nil && !eligible(i)) {
				continue
			}
		}
		out = append(out, listedSub{sub: sub, lang: listedLang(sub)})
	}
	if slices.ContainsFunc(out, func(l listedSub) bool { return transfer.ChineseSubtitle(l.lang) }) {
		out = slices.DeleteFunc(out, func(l listedSub) bool { return !transfer.ChineseSubtitle(l.lang) })
	}
	slices.SortStableFunc(out, func(a, b listedSub) int {
		return transfer.SubtitleLangRank(a.lang) - transfer.SubtitleLangRank(b.lang)
	})
	if len(out) > siteSubDownloads {
		out = out[:siteSubDownloads]
	}
	return out
}

// gotSub 是下载（解开）出来的一个字幕文件。
type gotSub struct {
	name string // 文件名（压缩包里的只留最后一段）
	data []byte
	lang string // 站点列表上写的语言（文件名里认不出语言时用）
}

var (
	zipMagic = []byte("PK\x03\x04")
	rarMagic = []byte("Rar!\x1a\x07")
	sevenZip = []byte("7z\xbc\xaf\x27\x1c")
	srtStart = regexp.MustCompile(`\A\s*\d+\s*\r?\n\s*\d{1,2}:\d{2}:\d{2}[,.]\d{1,3}\s*-->`)
)

// sniffSubExt 按内容认字幕的格式（文件名里没有扩展名时）：ASS/SSA、WebVTT、SRT；认不出时为空。
func sniffSubExt(data []byte) string {
	head := data[:min(len(data), 512)]
	head = bytes.TrimPrefix(head, []byte("\xef\xbb\xbf"))
	trimmed := bytes.TrimLeft(head, " \t\r\n")
	switch {
	case bytes.HasPrefix(trimmed, []byte("[Script Info]")):
		return ".ass"
	case bytes.HasPrefix(trimmed, []byte("WEBVTT")):
		return ".vtt"
	case srtStart.Match(head):
		return ".srt"
	}
	return ""
}

// unpackSiteSub 把下载下来的一份字幕换成字幕文件：zip 解开只取字幕文件，rar 与 7z 不解；返回字幕文件与要写进说明的话。
// 站点没给文件名时用列表上的标题。
func unpackSiteSub(f v2.SubtitleFile, ls listedSub) ([]gotSub, []string) {
	name := f.Name
	if name == "" {
		name = path.Base(strings.ReplaceAll(strings.TrimSpace(ls.sub.Title), `\`, "/"))
	}
	ext := strings.ToLower(path.Ext(name))
	switch {
	case ext == ".zip" || bytes.HasPrefix(f.Data, zipMagic):
		return unzipSubs(f.Data, name, ls.lang)
	case ext == ".rar" || bytes.HasPrefix(f.Data, rarMagic):
		return nil, []string{name + " 是 RAR 压缩包，没有解开（只解 zip）"}
	case ext == ".7z" || bytes.HasPrefix(f.Data, sevenZip):
		return nil, []string{name + " 是 7z 压缩包，没有解开（只解 zip）"}
	case len(f.Data) > siteSubFileMax:
		return nil, []string{name + " 太大，没有放"}
	case transfer.IsSubtitle(name):
		return []gotSub{{name: name, data: f.Data, lang: ls.lang}}, nil
	}
	if sniffed := sniffSubExt(f.Data); sniffed != "" {
		return []gotSub{{name: name + sniffed, data: f.Data, lang: ls.lang}}, nil
	}
	return nil, []string{name + " 不是字幕文件，没有放"}
}

// zipEntryName 是压缩包里一项的名字：不是 UTF-8 时按 GB18030 认（中文字幕包多半是 GBK 写的名字）。
func zipEntryName(name string) string {
	if !utf8.ValidString(name) {
		if dec, err := simplifiedchinese.GB18030.NewDecoder().String(name); err == nil {
			name = dec
		}
	}
	return path.Base(strings.ReplaceAll(name, `\`, "/"))
}

// unzipSubs 解开 zip 里的字幕文件（只取最后一段名字，不按包里的目录放）。
func unzipSubs(data []byte, label, lang string) ([]gotSub, []string) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, []string{fmt.Sprintf("%s 解不开（%v）", label, err)}
	}
	var out []gotSub
	var notes []string
	total := 0
	for i, zf := range zr.File {
		if i >= siteSubZipScan {
			notes = append(notes, fmt.Sprintf("%s 里的文件太多，只看了前 %d 个", label, siteSubZipScan))
			break
		}
		name := zipEntryName(zf.Name)
		if zf.FileInfo().IsDir() || !transfer.IsSubtitle(name) {
			continue
		}
		if len(out) >= siteSubZipFiles {
			notes = append(notes, fmt.Sprintf("%s 里的字幕太多，只取了前 %d 个", label, siteSubZipFiles))
			break
		}
		if zf.UncompressedSize64 > siteSubFileMax {
			notes = append(notes, fmt.Sprintf("%s 里的 %s 太大，没有放", label, name))
			continue
		}
		b, err := readZipEntry(zf)
		if err != nil {
			notes = append(notes, fmt.Sprintf("%s 里的 %s 解不开（%v）", label, name, err))
			continue
		}
		if total+len(b) > siteSubZipTotal {
			notes = append(notes, label+" 解出来的字幕太大，后面的没有放")
			break
		}
		total += len(b)
		out = append(out, gotSub{name: name, data: b, lang: lang})
	}
	if len(out) == 0 && len(notes) == 0 {
		notes = append(notes, label+" 里没有字幕文件")
	}
	return out, notes
}

// readZipEntry 读压缩包里的一项，最多 siteSubFileMax 字节（头里写的大小不可信）。
func readZipEntry(zf *zip.File) ([]byte, error) {
	rc, err := zf.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, siteSubFileMax+1))
	if err != nil {
		return nil, err
	}
	if len(b) > siteSubFileMax {
		return nil, errors.New("太大")
	}
	return b, nil
}

// placeSiteSubs 把分给这个视频的字幕放进库：按语言排、有中文的只放中文的，最多 siteSubPerVideo 个，放成「视频名.语言.后缀」
// （语言先看文件名里的写法，没有时用站点列表上的）。名字被别的文件占着时往后换一个序号，不覆盖；同一个名字上已经是一样的内容时不再放。
// 返回放好的与要写进说明的话。
func placeSiteSubs(it *PlanItem, subs []gotSub) ([]extra, []string) {
	type cand struct {
		gotSub
		lang, ext string
	}
	cands := make([]cand, 0, len(subs))
	for _, g := range subs {
		cands = append(cands, cand{gotSub: g, lang: firstNonEmpty(transfer.SubtitleLang(g.name), g.lang), ext: strings.ToLower(path.Ext(g.name))})
	}
	if slices.ContainsFunc(cands, func(c cand) bool { return transfer.ChineseSubtitle(c.lang) }) {
		cands = slices.DeleteFunc(cands, func(c cand) bool { return !transfer.ChineseSubtitle(c.lang) })
	}
	slices.SortStableFunc(cands, func(a, b cand) int { return transfer.SubtitleLangRank(a.lang) - transfer.SubtitleLangRank(b.lang) })
	var notes []string
	if over := len(cands) - siteSubPerVideo; over > 0 {
		notes = append(notes, fmt.Sprintf("还有 %d 个没有放（一个视频最多放 %d 个）", over, siteSubPerVideo))
		cands = cands[:siteSubPerVideo]
	}
	next := map[string]int{}
	var placed []extra
	for _, c := range cands {
		target, dup, err := writeSiteSub(it.Target, c.lang, c.ext, c.data, next)
		switch {
		case dup:
			continue
		case err != nil:
			notes = append(notes, fmt.Sprintf("%s 没放成（%v）", c.name, err))
			continue
		}
		id, _ := transfer.FileID(target)
		h := sha256.Sum256(c.data)
		placed = append(placed, extra{Target: target, FileID: id, Kind: extraSiteSub, SHA256: hex.EncodeToString(h[:])})
	}
	return placed, notes
}

// writeSiteSub 把一个字幕写到视频旁边：从这种语言与格式的下一个序号起找一个没被占的名字（O_EXCL 新建，不覆盖）。
// 占着的名字上已经是一样的内容时 dup 为真，不再写。next 记着每种语言与格式下一个序号。
func writeSiteSub(video, lang, ext string, data []byte, next map[string]int) (string, bool, error) {
	k := lang + ext
	for n := max(next[k], 1); n <= siteSubNames; n++ {
		target := transfer.SubtitleTargetLang(video, lang, ext, n)
		err := writeNewFile(target, data)
		if err == nil {
			next[k] = n + 1
			return target, false, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", false, err
		}
		if old, rerr := os.ReadFile(target); rerr == nil && bytes.Equal(old, data) {
			next[k] = n + 1
			return target, true, nil
		}
	}
	return "", false, fmt.Errorf("%s 一类的名字都被占着", filepath.Base(transfer.SubtitleTargetLang(video, lang, ext, 1)))
}

// writeNewFile 新建一个文件写进 data（已经有了时返回 os.ErrExist，不覆盖）；没写完时删掉。
func writeNewFile(p string, data []byte) error {
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(p)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(p)
		return err
	}
	return nil
}

// hasSiteSubs 报告这条记录以前放过的站点字幕还在（文件编号对得上）：重新整理出的视频不再到站点上取。
func hasSiteSubs(row *models.MediaTransferHistory) bool {
	for _, e := range rowExtras(row) {
		if e.Kind != extraSiteSub {
			continue
		}
		if id, err := transfer.FileID(e.Target); err == nil && id == e.FileID {
			return true
		}
	}
	return false
}

// siteSubtitles 给这次新放进库的视频下载站点上的字幕（条件见文件开头；原盘不取）。放进去的记进整理记录的 extras，
// 出的问题写进整理记录的说明（前缀「站点字幕：」），不影响整理的结果。返回给这次整理的消息。
func (s *Service) siteSubtitles(ctx context.Context, p *Plan, created []*PlanItem) []string {
	if p.Library == nil || !p.Library.SiteSubtitles || p.torrentSubs > 0 || p.siteName == "" || p.siteTorrentID == "" || s.cfg.SubtitleSites == nil {
		return nil
	}
	eligible := map[*PlanItem]bool{}
	for _, it := range created {
		if it.disc == nil && it.HistoryID > 0 && !hasSiteSubs(it.row) {
			eligible[it] = true
		}
	}
	if len(eligible) == 0 {
		return nil
	}
	prov := s.cfg.SubtitleSites(p.siteName)
	if prov == nil {
		return nil
	}
	// 按文件名、季集对字幕时看这个种子里所有的视频（以前整理过的、这次没整理成的也算）：别的视频的字幕不会分给这次放进库的
	videos := make([]transfer.File, 0, len(p.Items))
	owners := make([]*PlanItem, 0, len(p.Items))
	for i := range p.Items {
		videos = append(videos, transfer.File{Rel: p.Items[i].Rel, Path: p.Items[i].Source})
		owners = append(owners, &p.Items[i])
	}
	sctx, cancel := context.WithTimeout(ctx, siteSubTimeout)
	defer cancel()
	var torrentNotes []string
	itemNotes := map[*PlanItem][]string{}
	placed := map[*PlanItem][]extra{}
	list, err := prov.ListSubtitles(sctx, p.siteTorrentID)
	switch {
	case err != nil:
		torrentNotes = append(torrentNotes, fmt.Sprintf("取不到站点上的字幕（%v）", err))
	case len(list) == 0:
		torrentNotes = append(torrentNotes, "站点上没有这个种子的字幕")
	default:
		key := s.subtitleKey(ctx, p)
		picks := pickSiteSubs(list, videos, key, func(i int) bool { return eligible[owners[i]] })
		var got []gotSub
		for _, ls := range picks {
			f, err := prov.DownloadSubtitle(sctx, ls.sub)
			if err != nil {
				torrentNotes = append(torrentNotes, fmt.Sprintf("%s 没下载成（%v）", firstNonEmpty(ls.sub.Title, "字幕 "+ls.sub.ID), err))
				continue
			}
			g, notes := unpackSiteSub(f, ls)
			got = append(got, g...)
			torrentNotes = append(torrentNotes, notes...)
		}
		files := make([]transfer.File, len(got))
		for i, g := range got {
			files[i] = transfer.File{Rel: g.name, Path: strconv.Itoa(i)}
		}
		groups := transfer.MatchSubtitles(videos, files, key)
		assigned := 0
		for vi, owner := range owners {
			assigned += len(groups[vi])
			if !eligible[owner] || len(groups[vi]) == 0 {
				continue
			}
			mine := make([]gotSub, 0, len(groups[vi]))
			for _, f := range groups[vi] {
				i, _ := strconv.Atoi(f.Path)
				mine = append(mine, got[i])
			}
			placed[owner], itemNotes[owner] = placeSiteSubs(owner, mine)
		}
		if n := len(got) - assigned; n > 0 {
			torrentNotes = append(torrentNotes, fmt.Sprintf("%d 个字幕对不上哪个视频，没有放", n))
		}
	}
	msgs := prefixed("站点字幕：", torrentNotes)
	for _, it := range created {
		if !eligible[it] {
			continue
		}
		notes := append(append([]string(nil), torrentNotes...), itemNotes[it]...)
		for _, n := range itemNotes[it] {
			msgs = append(msgs, fmt.Sprintf("站点字幕：%s：%s", filepath.Base(it.Target), n))
		}
		if err := s.noteRecord(ctx, it.HistoryID, prefixed("站点字幕：", notes), placed[it]); err != nil {
			for _, e := range placed[it] {
				_ = transfer.RemoveIfOurs(e.Target, models.MediaModeCopy, e.FileID, "")
			}
			msgs = append(msgs, fmt.Sprintf("站点字幕：%s：字幕记不进整理记录，放进去的已撤回（%v）", filepath.Base(it.Target), err))
			continue
		}
		if len(placed[it]) > 0 {
			s.cfg.Logger.Infof("[整理入库] 站点字幕：%s 放了 %d 个", filepath.Base(it.Target), len(placed[it]))
		}
	}
	for _, m := range msgs {
		s.cfg.Logger.Warnf("[整理入库] %s", m)
	}
	return msgs
}

// noteRecord 在整理记录的说明后面接上 notes、把 add 并进 extras（不动 updated_at，同刮削）。
func (s *Service) noteRecord(ctx context.Context, id uint, notes []string, add []extra) error {
	if id == 0 || (len(notes) == 0 && len(add) == 0) {
		return nil
	}
	var row models.MediaTransferHistory
	if err := s.cfg.DB.WithContext(ctx).Where("id = ?", id).Limit(1).Find(&row).Error; err != nil {
		return fmt.Errorf("读取整理记录失败: %w", err)
	}
	if row.ID == 0 {
		return ErrNotFound
	}
	cols := map[string]any{}
	if len(notes) > 0 {
		msg := strings.Join(notes, "；")
		if row.Message != "" {
			msg = row.Message + "；" + msg
		}
		cols["message"] = truncate(msg, 1024)
	}
	if len(add) > 0 {
		b, _ := json.Marshal(mergeExtras(decodeExtras(row.Extras), add))
		cols["extras"] = string(b)
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).Where("id = ?", id).UpdateColumns(cols).Error; err != nil {
		return fmt.Errorf("写整理记录失败: %w", err)
	}
	return nil
}
