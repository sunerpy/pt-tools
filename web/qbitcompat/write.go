package qbitcompat

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// selection 是一次写操作能动的种子。
type selection struct {
	ids      []string // 下载器自己的编号
	hashes   []string
	torrents []downloader.Torrent
	// denied 是没打开完全控制时跳过的、不是经兼容入口加的种子个数
	denied int
}

// selectTargets 解析 hashes（| 分隔；all 是全部），按设置挑出能动的种子：完全控制打开时不限，否则只要经兼容入口加进这台下载器的
// （所有权表里有、并且不是后来又从别处加回来的，见 isOwned）。
// 下载器里没有的 hash 不理（qB 也是这样）。
func (s *Server) selectTargets(ctx context.Context, b *backend, param string) (selection, error) {
	_, byHash, err := s.fetch(b)
	if err != nil {
		return selection{}, err
	}
	var owned map[string]models.QbitCompatTorrent
	if !b.cfg.FullControl {
		if owned, err = s.owned(ctx, b.setting.ID); err != nil {
			return selection{}, err
		}
	}
	var sel selection
	pick := func(h string) {
		t, ok := byHash[h]
		if !ok {
			return
		}
		if !b.cfg.FullControl && !isOwned(owned, h, t) {
			sel.denied++
			return
		}
		sel.ids, sel.hashes, sel.torrents = append(sel.ids, t.ID), append(sel.hashes, h), append(sel.torrents, t)
	}
	if strings.TrimSpace(param) == "all" {
		for _, h := range slices.Sorted(maps.Keys(byHash)) {
			pick(h)
		}
	} else {
		for _, h := range slices.Sorted(maps.Keys(splitHashes(param))) {
			pick(h)
		}
	}
	return sel, nil
}

// mutate 是改种子的写接口的共同流程：挑出能动的种子、执行、记审计。成功时回空的 200（qB 的行为），
// 有被跳过的种子时审计记 denied:not_compat；下载器报错时回 500 并写明原因。
func (s *Server) mutate(w http.ResponseWriter, r *http.Request, c *call, op string, extra map[string]any, do func(b *backend, sel selection) error) {
	if err := r.ParseForm(); err != nil {
		text(w, http.StatusBadRequest, "Bad Request")
		return
	}
	b, ok := s.withBackend(w, r)
	if !ok {
		return
	}
	sel, err := s.selectTargets(r.Context(), b, r.Form.Get("hashes"))
	if err != nil {
		text(w, http.StatusServiceUnavailable, redact(err.Error()))
		return
	}
	if len(sel.ids) > 0 {
		defer s.invalidate(b.setting.ID)
		if err := do(b, sel); err != nil {
			s.recordWrite(r, c, op, "error:downloader", sel, extra)
			text(w, http.StatusInternalServerError, redact(err.Error()))
			return
		}
	}
	result := "success"
	if sel.denied > 0 {
		result = "denied:not_compat"
	}
	s.recordWrite(r, c, op, result, sel, extra)
	text(w, http.StatusOK, "")
}

// recordWrite 记一条写操作的审计：令牌编号、种子个数（20 个以内连 hash 一起）、跳过的个数。
func (s *Server) recordWrite(r *http.Request, c *call, op, result string, sel selection, extra map[string]any) {
	c.audited = true
	args := map[string]any{"name": c.token.Name, "username": c.username, "count": len(sel.ids), "denied": sel.denied}
	if len(sel.hashes) <= 20 {
		args["hashes"] = sel.hashes
	}
	maps.Copy(args, extra)
	s.record(r, app.AuditEntry{
		ChannelUserID: strconv.FormatUint(uint64(c.token.ID), 10), Command: "POST /api/v2/" + op, Result: result, Args: args,
		LatencyMs: time.Since(c.start).Milliseconds(),
	})
}

// pause 是 torrents/pause 与 torrents/stop。
func (s *Server) pause(w http.ResponseWriter, r *http.Request, c *call) {
	s.mutate(w, r, c, "torrents/pause", nil, func(b *backend, sel selection) error { return b.dl.PauseTorrents(sel.ids) })
}

// resume 是 torrents/resume 与 torrents/start。
func (s *Server) resume(w http.ResponseWriter, r *http.Request, c *call) {
	s.mutate(w, r, c, "torrents/resume", nil, func(b *backend, sel selection) error { return b.dl.ResumeTorrents(sel.ids) })
}

// deleteTorrents 是 torrents/delete（deleteFiles=true 时连数据一起删）。
func (s *Server) deleteTorrents(w http.ResponseWriter, r *http.Request, c *call) {
	_ = r.ParseForm()
	withFiles := formBool(r.Form.Get("deleteFiles"))
	s.mutate(w, r, c, "torrents/delete", map[string]any{"delete_files": withFiles}, func(b *backend, sel selection) error {
		if err := b.dl.RemoveTorrents(sel.ids, withFiles); err != nil {
			return err
		}
		// 删掉了就不再归兼容入口：之后同一个种子从别处加回来，没有完全控制时动不了它
		return s.disown(r.Context(), b.setting.ID, sel.hashes)
	})
}

// setCategory 是 torrents/setCategory（category 为空时去掉分类）。qB 要求分类已经存在：后端是 qB 时先建一下（已经有了不算失败）。
func (s *Server) setCategory(w http.ResponseWriter, r *http.Request, c *call) {
	_ = r.ParseForm()
	category := strings.TrimSpace(r.Form.Get("category"))
	if len(category) > 255 {
		text(w, http.StatusBadRequest, "Bad Request")
		return
	}
	s.mutate(w, r, c, "torrents/setCategory", map[string]any{"category": category}, func(b *backend, sel selection) error {
		if usesLabels(b) {
			// Transmission：分类是第一个 label，改写整份 labels，标签留着
			for i, id := range sel.ids {
				_, tags := splitLabels(sel.torrents[i])
				if err := b.dl.SetTorrentTags(id, joinLabels(category, tags)); err != nil {
					return err
				}
			}
			return nil
		}
		if cc, ok := b.dl.(downloader.CategoryCreator); ok && category != "" {
			if err := cc.CreateCategory(category, categoryMap(b.cfg)[category]); err != nil {
				return err
			}
		}
		for _, id := range sel.ids {
			if err := b.dl.SetTorrentCategory(id, category); err != nil {
				return err
			}
		}
		return nil
	})
}

// splitTags 把逗号分隔的标签拆开，去掉空的与重复的。
func splitTags(s string) []string {
	var out []string
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(t); t != "" && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out
}

// addTags 是 torrents/addTags：给的是原有标签加上新的（Transmission 的 labels 是整份改写，qB 的 addTags 只会新增，两边都对）。
func (s *Server) addTags(w http.ResponseWriter, r *http.Request, c *call) {
	_ = r.ParseForm()
	// OwnerTag 只由兼容入口加，客户端加不上
	tags := visibleTags(splitTags(r.Form.Get("tags")))
	if len(tags) == 0 {
		text(w, http.StatusBadRequest, "Bad Request")
		return
	}
	s.mutate(w, r, c, "torrents/addTags", map[string]any{"tags": tags}, func(b *backend, sel selection) error {
		for i, id := range sel.ids {
			if usesLabels(b) {
				category, merged := splitLabels(sel.torrents[i])
				if err := b.dl.SetTorrentTags(id, joinLabels(category, append(merged, tags...))); err != nil {
					return err
				}
				continue
			}
			merged := splitTags(sel.torrents[i].Tags)
			for _, t := range tags {
				if !slices.Contains(merged, t) {
					merged = append(merged, t)
				}
			}
			if err := b.dl.SetTorrentTags(id, strings.Join(merged, ",")); err != nil {
				return err
			}
		}
		return nil
	})
}

// removeTags 是 torrents/removeTags（tags 为空时去掉全部标签）。qB 用 removeTags；Transmission 改写成剩下的标签，分类不动。
// OwnerTag 去不掉：去掉全部标签时只去客户端看得到的。
func (s *Server) removeTags(w http.ResponseWriter, r *http.Request, c *call) {
	_ = r.ParseForm()
	given := splitTags(r.Form.Get("tags"))
	all, tags := len(given) == 0, visibleTags(given)
	s.mutate(w, r, c, "torrents/removeTags", map[string]any{"tags": tags}, func(b *backend, sel selection) error {
		if !all && len(tags) == 0 {
			return nil
		}
		if tr, ok := b.dl.(downloader.TorrentTagRemover); ok && !usesLabels(b) {
			remove := tags
			if all {
				remove = nil
				for _, t := range sel.torrents {
					for _, tag := range visibleTags(splitTags(t.Tags)) {
						if !slices.Contains(remove, tag) {
							remove = append(remove, tag)
						}
					}
				}
				if len(remove) == 0 {
					return nil
				}
			}
			return tr.RemoveTorrentTags(sel.ids, strings.Join(remove, ","))
		}
		// Transmission：改写成剩下的标签，分类（第一个 label）与 OwnerTag 留着
		for i, id := range sel.ids {
			category, current := splitLabels(sel.torrents[i])
			var keep []string
			for _, t := range current {
				if t == OwnerTag || (!all && !slices.Contains(tags, t)) {
					keep = append(keep, t)
				}
			}
			if err := b.dl.SetTorrentTags(id, joinLabels(category, keep)); err != nil {
				return err
			}
		}
		return nil
	})
}

// createCategory 是 torrents/createCategory：记在设置里（加种子时没给目录就用它）；后端是 qB 时也在 qB 里建。
func (s *Server) createCategory(w http.ResponseWriter, r *http.Request, c *call) {
	if err := r.ParseForm(); err != nil {
		text(w, http.StatusBadRequest, "Bad Request")
		return
	}
	name, savePath := strings.TrimSpace(r.Form.Get("category")), strings.TrimSpace(r.Form.Get("savePath"))
	if name == "" {
		text(w, http.StatusBadRequest, "Category name is empty")
		return
	}
	if len(name) > 255 || len(savePath) > 1024 || strings.ContainsAny(name, "\\") {
		text(w, http.StatusConflict, "Category name is invalid")
		return
	}
	b, ok := s.withBackend(w, r)
	if !ok {
		return
	}
	extra := map[string]any{"category": name, "save_path": savePath}
	if cc, ok := b.dl.(downloader.CategoryCreator); ok {
		if err := cc.CreateCategory(name, savePath); err != nil {
			s.recordWrite(r, c, "torrents/createCategory", "error:downloader", selection{}, extra)
			text(w, http.StatusInternalServerError, redact(err.Error()))
			return
		}
	}
	if err := s.saveCategory(r.Context(), name, savePath); err != nil {
		text(w, http.StatusInternalServerError, fmt.Sprintf("记下分类失败: %v", err))
		return
	}
	s.recordWrite(r, c, "torrents/createCategory", "success", selection{}, extra)
	text(w, http.StatusOK, "")
}

// createTags 是 torrents/createTags：记在设置里，torrents/tags 会列出来（下载器里的标签在用到时才建）。
func (s *Server) createTags(w http.ResponseWriter, r *http.Request, c *call) {
	if err := r.ParseForm(); err != nil {
		text(w, http.StatusBadRequest, "Bad Request")
		return
	}
	tags := visibleTags(splitTags(r.Form.Get("tags")))
	if len(tags) == 0 || len(tags) > 100 {
		text(w, http.StatusBadRequest, "Bad Request")
		return
	}
	if err := s.saveTags(r.Context(), tags); err != nil {
		text(w, http.StatusInternalServerError, fmt.Sprintf("记下标签失败: %v", err))
		return
	}
	s.recordWrite(r, c, "torrents/createTags", "success", selection{}, map[string]any{"tags": tags})
	text(w, http.StatusOK, "")
}
