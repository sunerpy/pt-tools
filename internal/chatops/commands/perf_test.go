package commands

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/chatops"
)

func TestTorrentsCmd_Performance5000(t *testing.T) {
	const total = 5000
	page := make([]app.TorrentDTO, torrentsPageSize)
	for i := range page {
		page[i] = app.TorrentDTO{
			ID:       fmt.Sprintf("h%d", i),
			Name:     fmt.Sprintf("Torrent.Name.%d.S01E%02d.1080p.WEB-DL.x265", i, i%24),
			State:    "downloading",
			Progress: float64(i%100) / 100.0,
			Size:     int64(i+1) * 1024 * 1024,
		}
	}
	setupServices(t, &Services{
		Torrent: &mockTorrentService{listResult: page, listTotal: total},
	})
	spec, ok := chatops.DefaultRegistry().Get("torrents")
	if !ok {
		t.Fatal("torrents command not registered")
	}

	start := time.Now()
	reply, err := spec.Handler(context.Background(), []string{"qb1"}, chatops.Source{ReplyLang: "zh"})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("handler err: %v", err)
	}
	if reply.Text == "" {
		t.Fatal("empty reply")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("torrents handler took %v on 5000-item dataset, want < 2s", elapsed)
	}
	t.Logf("torrents handler latency: %v (page=%d total=%d)", elapsed, len(page), total)
}

func BenchmarkTorrentsHandler(b *testing.B) {
	page := make([]app.TorrentDTO, torrentsPageSize)
	for i := range page {
		page[i] = app.TorrentDTO{
			ID: fmt.Sprintf("h%d", i), Name: fmt.Sprintf("name-%d", i),
			State: "seeding", Progress: 1.0, Size: int64(i+1) * 1024,
		}
	}
	prev := getServices()
	SetServices(&Services{Torrent: &mockTorrentService{listResult: page, listTotal: 5000}})
	b.Cleanup(func() { SetServices(prev) })

	spec, _ := chatops.DefaultRegistry().Get("torrents")
	src := chatops.Source{ReplyLang: "zh"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = spec.Handler(context.Background(), []string{"qb1"}, src)
	}
}
