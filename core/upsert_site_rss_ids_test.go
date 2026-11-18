package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

// rssKeepFixture 是保存站点前的库：春天有两条 RSS（A 关联了过滤规则 ruleA，另一条是示例配置），
// HDSky 有一条 C（关联了 ruleB）。
type rssKeepFixture struct {
	db           *models.TorrentDB
	store        *ConfigStore
	a, example   uint
	other        uint
	ruleA, ruleB uint
	dl           uint
}

func newRSSKeepFixture(t *testing.T) rssKeepFixture {
	t.Helper()
	db, err := OpenTestDB(t)
	require.NoError(t, err)
	f := rssKeepFixture{db: db, store: NewConfigStore(db)}
	ruleA := models.FilterRule{Name: "a", Pattern: "*", Enabled: true}
	ruleB := models.FilterRule{Name: "b", Pattern: "x", Enabled: true}
	require.NoError(t, db.DB.Create(&ruleA).Error)
	require.NoError(t, db.DB.Create(&ruleB).Error)
	dl := models.DownloaderSetting{Name: "qbit", Type: "qbittorrent", URL: "http://127.0.0.1:8080", IsDefault: true, Enabled: true}
	require.NoError(t, db.DB.Create(&dl).Error)
	f.ruleA, f.ruleB, f.dl = ruleA.ID, ruleB.ID, dl.ID

	require.NoError(t, f.store.UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{
		Enabled: boolPtr(true), AuthMethod: "cookie", Cookie: "c=1", APIUrl: "http://api",
		RSS: []models.RSSConfig{
			{Name: "A", URL: "https://springsunday.net/rss/a", Tag: "movie", IntervalMinutes: 10, FilterRuleIDs: []uint{ruleA.ID}},
			{Name: "示例", URL: "https://springsunday.net/rss/example", Tag: "movie", IntervalMinutes: 10},
		},
	}))
	require.NoError(t, f.store.UpsertSiteWithRSS(models.SiteGroup("hdsky"), models.SiteConfig{
		Enabled: boolPtr(true), AuthMethod: "cookie", Cookie: "c=2", APIUrl: "https://hdsky.me",
		RSS: []models.RSSConfig{{Name: "C", URL: "https://hdsky.me/rss/c", IntervalMinutes: 10, FilterRuleIDs: []uint{ruleB.ID}}},
	}))
	rows := rssByName(t, db, "springsunday")
	f.a, f.example = rows["A"].ID, rows["示例"].ID
	f.other = rssByName(t, db, "hdsky")["C"].ID
	// 示例配置只由迁移标记，保存站点不写这一列
	require.NoError(t, db.DB.Model(&models.RSSSubscription{}).Where("id = ?", f.example).Update("is_example", true).Error)
	return f
}

// rssByName 按名字取站点的全部 RSS 行。
func rssByName(t *testing.T, db *models.TorrentDB, site string) map[string]models.RSSSubscription {
	t.Helper()
	var s models.SiteSetting
	require.NoError(t, db.DB.Where("name = ?", site).First(&s).Error)
	var rows []models.RSSSubscription
	require.NoError(t, db.DB.Where("site_id = ?", s.ID).Find(&rows).Error)
	out := make(map[string]models.RSSSubscription, len(rows))
	for _, r := range rows {
		out[r.Name] = r
	}
	return out
}

// filterIDsOf 读一条 RSS 关联的过滤规则编号（按编号查关联表，RSS 行删掉以后留下的关联也查得到）。
func filterIDsOf(t *testing.T, db *models.TorrentDB, rssID uint) []uint {
	t.Helper()
	ids, err := models.NewRSSFilterAssociationDB(db.DB).GetFilterRuleIDsForRSS(rssID)
	require.NoError(t, err)
	return ids
}

// 保存站点（网页编辑 RSS、浏览器扩展同步 Cookie 都整站保存）按编号原地更新 RSS，不再删掉重建：
// 编号不变，示例标记沿用库里的；编号为 0 或不是这个站点的新建；这次没传的连同过滤规则关联一起删掉；
// 同一个编号出现两次时报错，什么都不改。别的站点的 RSS 与关联一直不动。
func TestUpsertSiteWithRSS_KeepsRSSIDs(t *testing.T) {
	tests := []struct {
		name    string
		edit    func(f rssKeepFixture, sc *models.SiteConfig)
		wantErr string
		check   func(t *testing.T, f rssKeepFixture)
	}{
		{
			name: "原样带回全部 RSS（扩展同步 Cookie）：编号、关联与示例标记都不变",
			edit: func(_ rssKeepFixture, sc *models.SiteConfig) { sc.Cookie = "c=new" },
			check: func(t *testing.T, f rssKeepFixture) {
				rows := rssByName(t, f.db, "springsunday")
				require.Len(t, rows, 2)
				assert.Equal(t, f.a, rows["A"].ID, "编号不变")
				assert.Equal(t, f.example, rows["示例"].ID, "编号不变")
				assert.True(t, rows["示例"].IsExample, "示例标记不丢")
				assert.Equal(t, []uint{f.ruleA}, filterIDsOf(t, f.db, f.a))
				sc, err := f.store.GetSiteConf(models.SiteGroup("springsunday"))
				require.NoError(t, err)
				assert.Equal(t, "c=new", sc.Cookie)
			},
		},
		{
			name: "编辑以后编号不变，可编辑的字段换成新的，示例标记沿用库里的",
			edit: func(f rssKeepFixture, sc *models.SiteConfig) {
				for i := range sc.RSS {
					r := &sc.RSS[i]
					switch r.ID {
					case f.a:
						r.Name, r.URL, r.Category, r.Tag = "A 改名", "https://springsunday.net/rss/a2", "电影", "tv"
						r.IntervalMinutes, r.DownloaderID, r.DownloadPath = 1, &f.dl, "/data/a"
						r.PauseOnFreeEnd, r.FilterMode = true, models.FilterModeFilterOnly
						r.NotifyMode, r.NotifyConfIDs, r.MaxNotificationsPerHour = "all", "[3]", 0
						r.FilterRuleIDs = []uint{f.ruleB}
					case f.example:
						r.IsExample = false // 网页的编辑框不带这个标记
						r.IntervalMinutes = 30
					}
				}
			},
			check: func(t *testing.T, f rssKeepFixture) {
				rows := rssByName(t, f.db, "springsunday")
				require.Len(t, rows, 2)
				a := rows["A 改名"]
				assert.Equal(t, f.a, a.ID, "编号不变")
				assert.Equal(t, "https://springsunday.net/rss/a2", a.URL)
				assert.Equal(t, "电影", a.Category)
				assert.Equal(t, "tv", a.Tag)
				assert.Equal(t, models.MinIntervalMinutes, a.IntervalMinutes, "间隔不足下限时按下限存")
				require.NotNil(t, a.DownloaderID)
				assert.Equal(t, f.dl, *a.DownloaderID)
				assert.Equal(t, "/data/a", a.DownloadPath)
				assert.True(t, a.PauseOnFreeEnd)
				assert.Equal(t, models.FilterModeFilterOnly, a.FilterMode)
				assert.Equal(t, "all", a.NotifyMode)
				assert.Equal(t, "[3]", a.NotifyConfIDs)
				assert.Equal(t, 0, a.MaxNotificationsPerHour, "按传入的值存：0 = 不限制")
				assert.Equal(t, []uint{f.ruleB}, filterIDsOf(t, f.db, f.a), "过滤规则关联整体换成传入的")
				ex := rows["示例"]
				assert.Equal(t, f.example, ex.ID, "编号不变")
				assert.True(t, ex.IsExample, "示例标记沿用库里的")
				assert.Equal(t, int32(30), ex.IntervalMinutes)
			},
		},
		{
			name: "过滤规则给空列表：清掉这条 RSS 的关联",
			edit: func(f rssKeepFixture, sc *models.SiteConfig) {
				for i := range sc.RSS {
					if sc.RSS[i].ID == f.a {
						sc.RSS[i].FilterRuleIDs = []uint{}
					}
				}
			},
			check: func(t *testing.T, f rssKeepFixture) {
				assert.Equal(t, f.a, rssByName(t, f.db, "springsunday")["A"].ID, "编号不变")
				assert.Empty(t, filterIDsOf(t, f.db, f.a))
			},
		},
		{
			name: "删掉一条：只删这一条和它的过滤规则关联",
			edit: func(f rssKeepFixture, sc *models.SiteConfig) {
				kept := sc.RSS[:0]
				for _, r := range sc.RSS {
					if r.ID != f.a {
						kept = append(kept, r)
					}
				}
				sc.RSS = kept
			},
			check: func(t *testing.T, f rssKeepFixture) {
				rows := rssByName(t, f.db, "springsunday")
				require.Len(t, rows, 1)
				assert.Equal(t, f.example, rows["示例"].ID, "留下的那条编号不变")
				assert.True(t, rows["示例"].IsExample)
				var n int64
				require.NoError(t, f.db.DB.Model(&models.RSSSubscription{}).Where("id = ?", f.a).Count(&n).Error)
				assert.Zero(t, n, "这一行删掉了")
				assert.Empty(t, filterIDsOf(t, f.db, f.a), "它的过滤规则关联一起删掉，不留孤立的关联")
			},
		},
		{
			name: "别的站点的编号当新建，不改那个站点的 RSS",
			edit: func(f rssKeepFixture, sc *models.SiteConfig) {
				sc.RSS = append(sc.RSS, models.RSSConfig{ID: f.other, Name: "D", URL: "https://springsunday.net/rss/d", IntervalMinutes: 10})
			},
			check: func(t *testing.T, f rssKeepFixture) {
				rows := rssByName(t, f.db, "springsunday")
				require.Len(t, rows, 3)
				assert.Equal(t, f.a, rows["A"].ID)
				assert.Equal(t, f.example, rows["示例"].ID)
				d := rows["D"]
				assert.NotZero(t, d.ID)
				assert.NotEqual(t, f.other, d.ID, "新建一行，不接收别的站点的编号")
			},
		},
		{
			name: "编号为 0 的新建",
			edit: func(_ rssKeepFixture, sc *models.SiteConfig) {
				sc.RSS = append(sc.RSS, models.RSSConfig{Name: "E", URL: "https://springsunday.net/rss/e", IntervalMinutes: 10, FilterRuleIDs: []uint{}})
			},
			check: func(t *testing.T, f rssKeepFixture) {
				rows := rssByName(t, f.db, "springsunday")
				require.Len(t, rows, 3)
				assert.Equal(t, f.a, rows["A"].ID)
				assert.Equal(t, f.example, rows["示例"].ID)
				assert.NotZero(t, rows["E"].ID)
				assert.Empty(t, filterIDsOf(t, f.db, rows["E"].ID))
			},
		},
		{
			name: "同一个编号出现两次：报错，站点与 RSS 都不改",
			edit: func(f rssKeepFixture, sc *models.SiteConfig) {
				sc.UploadLimitKBs = 77
				sc.RSS = append(sc.RSS, models.RSSConfig{ID: f.a, Name: "A2", URL: "https://springsunday.net/rss/a-copy", IntervalMinutes: 10})
			},
			wantErr: "第 3 条 RSS 的编号与之前的重复",
			check: func(t *testing.T, f rssKeepFixture) {
				rows := rssByName(t, f.db, "springsunday")
				require.Len(t, rows, 2)
				assert.Equal(t, f.a, rows["A"].ID)
				assert.Equal(t, f.example, rows["示例"].ID)
				assert.Equal(t, []uint{f.ruleA}, filterIDsOf(t, f.db, f.a))
				sc, err := f.store.GetSiteConf(models.SiteGroup("springsunday"))
				require.NoError(t, err)
				assert.Zero(t, sc.UploadLimitKBs, "整个保存没有生效")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newRSSKeepFixture(t)
			sc, err := f.store.GetSiteConf(models.SiteGroup("springsunday"))
			require.NoError(t, err)
			tt.edit(f, &sc)
			err = f.store.UpsertSiteWithRSS(models.SiteGroup("springsunday"), sc)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			tt.check(t, f)
			c := rssByName(t, f.db, "hdsky")["C"]
			assert.Equal(t, f.other, c.ID, "别的站点的 RSS 不动")
			assert.Equal(t, []uint{f.ruleB}, filterIDsOf(t, f.db, f.other), "别的站点的过滤规则关联不动")
		})
	}
}

// 保存 RSS 的哪一步写库出错，都把错误交回调用方，整个保存回滚：站点设置、RSS 行与过滤规则关联都和保存前一样。
func TestUpsertSiteWithRSS_KeepsRSSIDsRollsBackOnDBError(t *testing.T) {
	dropA := func(f rssKeepFixture, sc *models.SiteConfig) {
		kept := sc.RSS[:0]
		for _, r := range sc.RSS {
			if r.ID != f.a {
				kept = append(kept, r)
			}
		}
		sc.RSS = kept
	}
	editA := func(apply func(r *models.RSSConfig)) func(f rssKeepFixture, sc *models.SiteConfig) {
		return func(f rssKeepFixture, sc *models.SiteConfig) {
			for i := range sc.RSS {
				if sc.RSS[i].ID == f.a {
					apply(&sc.RSS[i])
				}
			}
		}
	}
	tests := []struct {
		name    string
		trigger string // 让这一步写库失败的触发器
		edit    func(f rssKeepFixture, sc *models.SiteConfig)
		wantErr string
	}{
		{
			name:    "删掉没传的那条 RSS 的过滤规则关联失败",
			trigger: "CREATE TRIGGER deny_assoc_delete BEFORE DELETE ON rss_filter_associations BEGIN SELECT RAISE(ABORT, 'deny assoc delete'); END;",
			edit:    dropA,
			wantErr: "deny assoc delete",
		},
		{
			name:    "删掉没传的那条 RSS 失败（它的关联已经删了，也要回滚）",
			trigger: "CREATE TRIGGER deny_rss_delete BEFORE DELETE ON rss_subscriptions BEGIN SELECT RAISE(ABORT, 'deny rss delete'); END;",
			edit:    dropA,
			wantErr: "deny rss delete",
		},
		{
			name:    "原地更新 RSS 失败",
			trigger: "CREATE TRIGGER deny_rss_update BEFORE UPDATE ON rss_subscriptions BEGIN SELECT RAISE(ABORT, 'deny rss update'); END;",
			edit:    editA(func(r *models.RSSConfig) { r.Name = "A 改名" }),
			wantErr: "deny rss update",
		},
		{
			name:    "换过滤规则关联失败",
			trigger: "CREATE TRIGGER deny_assoc_insert BEFORE INSERT ON rss_filter_associations BEGIN SELECT RAISE(ABORT, 'deny assoc insert'); END;",
			edit: func(f rssKeepFixture, sc *models.SiteConfig) {
				editA(func(r *models.RSSConfig) { r.FilterRuleIDs = []uint{f.ruleB} })(f, sc)
			},
			wantErr: "deny assoc insert",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newRSSKeepFixture(t)
			sc, err := f.store.GetSiteConf(models.SiteGroup("springsunday"))
			require.NoError(t, err)
			sc.UploadLimitKBs = 77
			tt.edit(f, &sc)
			require.NoError(t, f.db.DB.Exec(tt.trigger).Error)

			require.ErrorContains(t, f.store.UpsertSiteWithRSS(models.SiteGroup("springsunday"), sc), tt.wantErr)

			rows := rssByName(t, f.db, "springsunday")
			require.Len(t, rows, 2)
			assert.Equal(t, f.a, rows["A"].ID, "A 还在，名字没改")
			assert.Equal(t, f.example, rows["示例"].ID)
			assert.Equal(t, []uint{f.ruleA}, filterIDsOf(t, f.db, f.a), "过滤规则关联没变")
			var site models.SiteSetting
			require.NoError(t, f.db.DB.Where("name = ?", "springsunday").First(&site).Error)
			assert.Zero(t, site.UploadLimitKBs, "站点设置也没保存")
		})
	}
}

// 读不出这个站点现有的 RSS 时（这里是表没了）报错，站点设置也不保存。
func TestUpsertSiteWithRSS_StoredRSSReadError(t *testing.T) {
	f := newRSSKeepFixture(t)
	sc, err := f.store.GetSiteConf(models.SiteGroup("springsunday"))
	require.NoError(t, err)
	// 不带 RSS：前面的校验不读 RSS 表，读库出错发生在保存 RSS 的那一步
	sc.RSS, sc.UploadLimitKBs = nil, 77
	require.NoError(t, f.db.DB.Migrator().DropTable(&models.RSSSubscription{}))

	require.ErrorContains(t, f.store.UpsertSiteWithRSS(models.SiteGroup("springsunday"), sc), "rss_subscriptions")

	var site models.SiteSetting
	require.NoError(t, f.db.DB.Where("name = ?", "springsunday").First(&site).Error)
	assert.Zero(t, site.UploadLimitKBs, "整个保存回滚")
}

// 站点列表（GET /api/sites，读 ListSites）上的启用开关把列表里的站点配置原样整站保存：列表里的 RSS 带上过滤规则编号，
// 存回去时过滤规则关联不丢。以前列表不带这一项，在列表上启用、停用一次站点，它的 RSS 就没有过滤规则了。
func TestUpsertSiteWithRSS_ListedConfigKeepsFilterRules(t *testing.T) {
	f := newRSSKeepFixture(t)
	sites, err := f.store.ListSites()
	require.NoError(t, err)
	sc := sites["springsunday"]
	for _, r := range sc.RSS {
		if r.ID == f.a {
			assert.Equal(t, []uint{f.ruleA}, r.FilterRuleIDs, "列表里的 RSS 带上过滤规则编号")
		}
	}
	disabled := false
	sc.Enabled = &disabled
	require.NoError(t, f.store.UpsertSiteWithRSS("springsunday", sc))
	assert.Equal(t, []uint{f.ruleA}, filterIDsOf(t, f.db, f.a), "存回去以后过滤规则关联还在")
	assert.Equal(t, []uint{f.ruleB}, filterIDsOf(t, f.db, f.other), "别的站点不动")
}
