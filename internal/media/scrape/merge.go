package scrape

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

const header = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"

// node 是 NFO 里根元素下的一个元素：raw 是它原样的 XML，text 是直接写在它里面的文字。
type node struct {
	name string
	raw  []byte
	text string
}

// root 是 NFO 的一个根元素（多集的文件有几个 <episodedetails>）。
type root struct {
	name     string
	children []node
}

var bom = []byte{0xEF, 0xBB, 0xBF}

// parseDoc 读出 NFO 的每个根元素与根元素下的元素。读不出来（不是 XML、编码不是 UTF-8）时返回错误。
// encoding/xml 不读 DTD 与外部实体，只认 XML 预定义的几个实体。
func parseDoc(b []byte) ([]root, error) {
	b = bytes.TrimPrefix(b, bom)
	d := xml.NewDecoder(bytes.NewReader(b))
	var roots []root
	var cur *node
	var start int64
	depth := 0
	for {
		off := d.InputOffset()
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch depth {
			case 1:
				roots = append(roots, root{name: t.Name.Local})
			case 2:
				cur, start = &node{name: t.Name.Local}, off
			}
		case xml.EndElement:
			if depth == 2 && cur != nil {
				cur.raw = bytes.Clone(b[start:d.InputOffset()])
				r := &roots[len(roots)-1]
				r.children = append(r.children, *cur)
				cur = nil
			}
			depth--
		case xml.CharData:
			if depth == 2 && cur != nil {
				cur.text += string(t)
			}
		}
	}
	if len(roots) == 0 {
		return nil, errors.New("没有根元素")
	}
	return roots, nil
}

// lockRe 认读不出来的 NFO 里的 <lockdata>true</lockdata>。
var lockRe = regexp.MustCompile(`(?i)<lockdata>\s*true\s*</lockdata>`)

// locked 报告 NFO 是不是锁定了（<lockdata>true</lockdata>，Jellyfin 与 Emby 里锁定了的条目）：锁定了的不改。
func locked(b []byte, roots []root) bool {
	if roots == nil {
		return lockRe.Match(b)
	}
	for _, r := range roots {
		for _, c := range r.children {
			if c.name == "lockdata" && strings.EqualFold(strings.TrimSpace(c.text), "true") {
				return true
			}
		}
	}
	return false
}

// managed 是 pt-tools 管的元素：覆盖已有的 NFO 时换成这次的（这次没有的就不写），其余的原样留着（播放记录、
// 添加日期、标签、媒体信息、排序标题等）。评分、编号与图片地址一类的也算：别的工具写的这些和这次的放在一起，
// 媒体服务器按顺序读，会用错。
var managed = map[string]bool{
	"title": true, "originaltitle": true, "year": true, "premiered": true, "releasedate": true, "aired": true,
	"plot": true, "outline": true, "tagline": true, "runtime": true, "mpaa": true, "certification": true, "status": true,
	"rating": true, "ratings": true, "votes": true, "criticrating": true, "communityrating": true,
	"uniqueid": true, "id": true, "tmdbid": true, "imdbid": true, "tvdbid": true,
	"genre": true, "studio": true, "set": true, "director": true, "credits": true, "writer": true, "actor": true,
	"showtitle": true, "season": true, "episode": true, "seasonnumber": true,
	"thumb": true, "fanart": true,
}

// lockedFieldTags 是 Jellyfin 的锁定字段（<lockedfields>，用 | 分隔）对应的元素：锁定了的字段用原来的。
var lockedFieldTags = map[string][]string{
	"name":           {"title"},
	"overview":       {"plot", "outline"},
	"genres":         {"genre"},
	"studios":        {"studio"},
	"cast":           {"actor", "director", "credits", "writer"},
	"officialrating": {"mpaa"},
	"runtime":        {"runtime"},
}

func lockedTags(r root) map[string]bool {
	out := map[string]bool{}
	for _, c := range r.children {
		if c.name != "lockedfields" {
			continue
		}
		for _, f := range strings.Split(c.text, "|") {
			for _, tag := range lockedFieldTags[strings.ToLower(strings.TrimSpace(f))] {
				out[tag] = true
			}
		}
	}
	return out
}

// build 生成 NFO：docs 是这次的每个根元素。old 是已经有的那份（覆盖时，读得出来才有）：根元素一一对得上时，
// 原来的元素里 pt-tools 不管的（见 managed）留在后面，媒体服务器里锁定了的字段用原来的。
func build(docs []any, old []root) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(header)
	merge := len(old) == len(docs)
	for i, v := range docs {
		out, err := xml.MarshalIndent(v, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("生成 NFO 失败: %w", err)
		}
		cur, err := parseDoc(out)
		if err != nil {
			return nil, fmt.Errorf("生成 NFO 失败: %w", err)
		}
		r := cur[0]
		var kids []node
		lock := map[string]bool{}
		if merge && old[i].name == r.name {
			lock, kids = lockedTags(old[i]), old[i].children
		}
		placed := map[string]bool{}
		b.WriteString("<" + r.name + ">\n")
		for _, c := range r.children {
			if !lock[c.name] {
				writeNode(&b, c)
				continue
			}
			// 锁定的项用原来的，写在它本来的位置
			if !placed[c.name] {
				placed[c.name] = true
				for _, o := range kids {
					if o.name == c.name {
						writeNode(&b, o)
					}
				}
			}
		}
		// 不管的元素与这次没有的锁定项，按原来的顺序写在后面
		for _, o := range kids {
			if (lock[o.name] && !placed[o.name]) || (!lock[o.name] && !managed[o.name]) {
				writeNode(&b, o)
			}
		}
		b.WriteString("</" + r.name + ">\n")
	}
	return b.Bytes(), nil
}

func writeNode(b *bytes.Buffer, c node) {
	b.WriteString("  ")
	b.Write(c.raw)
	b.WriteByte('\n')
}
