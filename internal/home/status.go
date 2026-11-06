package home

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/health"
	"github.com/sunerpy/pt-tools/models"
)

// serverTestTimeout 是状态行里测一台媒体服务器最多等多久
const serverTestTimeout = 5 * time.Second

// HealthLine 是状态行的体检：上一次体检的时间与各状态的项数；Problem 是第一项失败（没有时第一项提醒）的标题与说明。
type HealthLine struct {
	At      time.Time `json:"at"`
	OK      int       `json:"ok"`
	Warn    int       `json:"warn"`
	Fail    int       `json:"fail"`
	Skip    int       `json:"skip"`
	Problem string    `json:"problem,omitempty"`
}

// BackupLine 是状态行的备份：Late 同待处理（开着定时备份时，最近一次失败或两天没成功），Days 是最近一次成功距今几天（-1 是没成功过）。
type BackupLine struct {
	Enabled  bool       `json:"enabled"`
	LastAt   *time.Time `json:"last_at,omitempty"`
	LastOKAt *time.Time `json:"last_ok_at,omitempty"`
	Error    string     `json:"error,omitempty"`
	Late     bool       `json:"late"`
	Days     int        `json:"days"`
}

// ServerLine 是状态行的一台媒体服务器：连不连得上与版本（现测，最多等 5 秒）、索引上次同步的时间与条目数。
// OpenURL 是服务器网页的地址（「打开」）。
type ServerLine struct {
	ID         uint       `json:"id"`
	Name       string     `json:"name"`
	Kind       string     `json:"kind"`
	Version    string     `json:"version,omitempty"`
	Online     bool       `json:"online"`
	Error      string     `json:"error,omitempty"`
	IndexedAt  *time.Time `json:"indexed_at,omitempty"`
	IndexError string     `json:"index_error,omitempty"`
	Movies     int        `json:"movies"`
	Series     int        `json:"series"`
	Episodes   int        `json:"episodes"`
	OpenURL    string     `json:"open_url,omitempty"`
}

// Status 是首页 NAS 状态最下面的几行：体检、备份、媒体服务器。
type Status struct {
	Health  *HealthLine  `json:"health"`
	Backup  *BackupLine  `json:"backup"`
	Servers []ServerLine `json:"servers"`
}

// StatusInput 是算状态行要用的：Health 是上一次体检（没体检过时为 nil）；Test 测一台媒体服务器（同体检）；
// HomeURL 是一台服务器网页的地址。
type StatusInput struct {
	DB      *gorm.DB
	Now     time.Time
	Health  *health.Report
	Backup  func() (health.BackupState, error)
	Test    func(ctx context.Context, srv models.MediaServer) (health.ServerInfo, error)
	HomeURL func(srv models.MediaServer) string
}

// firstProblem 是体检里第一项失败（没有时第一项提醒）的「标题：说明」；都正常时为空。
func firstProblem(rep *health.Report) string {
	for _, want := range []string{health.StatusFail, health.StatusWarn} {
		for _, it := range rep.Items {
			if it.Status != want {
				continue
			}
			title := problemTitle(it)
			if it.Detail == "" {
				return clean(title)
			}
			return clean(title + "：" + it.Detail)
		}
	}
	return ""
}

// problemTitle 是这一项在首页那一行里的名字：站点、下载器、媒体服务器与 NAS 的项在体检页的分组下只写了名字，
// 离开分组要写明是哪一类（「下载器「qb」」）。
func problemTitle(it health.Item) string {
	for _, k := range []struct{ prefix, kind string }{
		{"site:", "站点"}, {"downloader:", "下载器"}, {"server:", "媒体服务器"}, {"nas:", "NAS"},
	} {
		if strings.HasPrefix(it.Key, k.prefix) {
			return k.kind + "「" + it.Title + "」"
		}
	}
	return it.Title
}

// BuildStatus 算状态行。媒体服务器是启用的那几台，几台同时测。
func BuildStatus(ctx context.Context, in StatusInput) (*Status, error) {
	out := &Status{Servers: []ServerLine{}}
	if rep := in.Health; rep != nil {
		out.Health = &HealthLine{At: rep.At, OK: rep.OK, Warn: rep.Warn, Fail: rep.Fail, Skip: rep.Skip, Problem: firstProblem(rep)}
	}
	if in.Backup != nil {
		st, err := in.Backup()
		if err != nil {
			return nil, err
		}
		days, late := health.BackupLate(st, in.Now)
		out.Backup = &BackupLine{Enabled: st.Enabled, LastAt: st.LastAt, LastOKAt: st.LastOKAt, Error: clean(st.LastError), Late: late, Days: days}
	}
	var rows []models.MediaServer
	if err := in.DB.WithContext(ctx).Where("enabled = ?", true).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取媒体服务器失败: %w", err)
	}
	lines := make([]ServerLine, len(rows))
	done := make(chan struct{}, len(rows))
	for i, srv := range rows {
		go func() {
			defer func() { done <- struct{}{} }()
			lines[i] = serverLine(ctx, srv, in)
		}()
	}
	for range rows {
		<-done
	}
	out.Servers = append(out.Servers, lines...)
	return out, nil
}

func serverLine(ctx context.Context, srv models.MediaServer, in StatusInput) ServerLine {
	l := ServerLine{
		ID: srv.ID, Name: srv.Name, Kind: srv.Kind, IndexedAt: srv.IndexedAt, IndexError: clean(srv.IndexError),
		Movies: srv.IndexMovies, Series: srv.IndexSeries, Episodes: srv.IndexEpisodes,
	}
	if in.HomeURL != nil {
		l.OpenURL = in.HomeURL(srv)
	}
	if in.Test == nil {
		return l
	}
	tctx, cancel := context.WithTimeout(ctx, serverTestTimeout)
	defer cancel()
	info, err := in.Test(tctx, srv)
	if err != nil {
		l.Error = clean(err.Error())
		return l
	}
	l.Online, l.Version = true, strings.TrimSpace(info.Version)
	return l
}
