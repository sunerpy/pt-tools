package internal

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"go.uber.org/zap"
)

type fakeMT struct{ ctx context.Context }

func (f *fakeMT) GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[models.MTTorrentDetail], error) {
	end := time.Now().Add(30 * time.Minute).Format("2006-01-02 15:04:05")
	d := models.MTTorrentDetail{ID: item.GUID, Size: "1048576", Status: &models.Status{Discount: "free", DiscountEndTime: end}}
	return &models.APIResponse[models.MTTorrentDetail]{Code: "success", Data: d}, nil
}
func (f *fakeMT) IsEnabled() bool { return true }
func (f *fakeMT) DownloadTorrent(url, title, dir string) (string, error) {
	_ = ensureFile(filepath.Join(dir, sanitizeTitle(title)+".torrent"))
	return "hash-mt", nil
}
func (f *fakeMT) MaxRetries() int                                                      { return 1 }
func (f *fakeMT) RetryDelay() time.Duration                                            { return 0 }
func (f *fakeMT) SendTorrentToQbit(ctx context.Context, rssCfg models.RSSConfig) error { return nil }
func (f *fakeMT) Context() context.Context                                             { return f.ctx }

type fakePHP struct{ ctx context.Context }

func (f *fakePHP) GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[models.PHPTorrentInfo], error) {
	end := time.Now().Add(30 * time.Minute)
	d := models.PHPTorrentInfo{Title: item.Title, TorrentID: item.GUID, Discount: models.DISCOUNT_FREE, EndTime: end, SizeMB: 512}
	return &models.APIResponse[models.PHPTorrentInfo]{Code: "success", Data: d}, nil
}
func (f *fakePHP) IsEnabled() bool { return true }
func (f *fakePHP) DownloadTorrent(url, title, dir string) (string, error) {
	_ = ensureFile(filepath.Join(dir, sanitizeTitle(title)+".torrent"))
	return "hash-php", nil
}
func (f *fakePHP) MaxRetries() int                                                      { return 1 }
func (f *fakePHP) RetryDelay() time.Duration                                            { return 0 }
func (f *fakePHP) SendTorrentToQbit(ctx context.Context, rssCfg models.RSSConfig) error { return nil }
func (f *fakePHP) Context() context.Context                                             { return f.ctx }
func ensureFile(p string) error {
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()
	_, _ = f.WriteString("dummy")
	return nil
}

func TestDBUpdate_Mteam_Cmct_Hdsky(t *testing.T) {
	dir := t.TempDir()
	db, err := core.NewTempDBDir(dir)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	store := core.NewConfigStore(db)
	// set global download dir to temp
	if err := store.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 10, DefaultEnabled: true}); err != nil {
		t.Fatalf("save global: %v", err)
	}
	// common inputs
	rss := models.RSSConfig{Name: "test", URL: "http://local/rss", IntervalMinutes: 10, DownloadSubPath: "sub", Tag: "test-tag"}
	// feed item
	item := &gofeed.Item{GUID: "guid-1", Title: "Sample Title", Categories: []string{"Movie"}}
	// CMCT
	{
		ch := make(chan *gofeed.Item, 1)
		ch <- item
		close(ch)
		var wg sync.WaitGroup
		wg.Add(1)
		downloadWorker(context.Background(), models.CMCT, &wg, &fakePHP{ctx: context.Background()}, ch, rss.DownloadSubPath, rss)
		wg.Wait()
		// verify
		ti, err := global.GlobalDB.GetTorrentBySiteAndID(string(models.CMCT), item.GUID)
		if err != nil {
			t.Fatalf("query cmct: %v", err)
		}
		if ti == nil || ti.Title == "" || ti.Category == "" {
			t.Fatalf("cmct record incomplete: %+v", ti)
		}
		var rows []models.TorrentInfo
		_ = global.GlobalDB.DB.Order("id asc").Find(&rows).Error
		b, _ := json.MarshalIndent(rows, "", "  ")
		t.Logf("cmct rows: %s", string(b))
	}
	// HDSKY
	{
		ch := make(chan *gofeed.Item, 1)
		ch <- item
		close(ch)
		var wg sync.WaitGroup
		wg.Add(1)
		downloadWorker(context.Background(), models.HDSKY, &wg, &fakePHP{ctx: context.Background()}, ch, rss.DownloadSubPath, rss)
		wg.Wait()
		ti, err := global.GlobalDB.GetTorrentBySiteAndID(string(models.HDSKY), item.GUID)
		if err != nil {
			t.Fatalf("query hdsky: %v", err)
		}
		if ti == nil || ti.Title == "" || ti.Category == "" {
			t.Fatalf("hdsky record incomplete: %+v", ti)
		}
		var rows []models.TorrentInfo
		_ = global.GlobalDB.DB.Order("id asc").Find(&rows).Error
		b, _ := json.MarshalIndent(rows, "", "  ")
		t.Logf("hdsky rows: %s", string(b))
	}
	// MTEAM
	{
		ch := make(chan *gofeed.Item, 1)
		ch <- item
		close(ch)
		var wg sync.WaitGroup
		wg.Add(1)
		downloadWorker(context.Background(), models.MTEAM, &wg, &fakeMT{ctx: context.Background()}, ch, rss.DownloadSubPath, rss)
		wg.Wait()
		ti, err := global.GlobalDB.GetTorrentBySiteAndID(string(models.MTEAM), item.GUID)
		if err != nil {
			t.Fatalf("query mteam: %v", err)
		}
		if ti == nil || ti.Title == "" || ti.Category == "" {
			t.Fatalf("mteam record incomplete: %+v", ti)
		}
		var rows []models.TorrentInfo
		_ = global.GlobalDB.DB.Order("id asc").Find(&rows).Error
		b, _ := json.MarshalIndent(rows, "", "  ")
		t.Logf("mteam rows: %s", string(b))
	}
}
