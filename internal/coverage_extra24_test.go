// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"context"
	"testing"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func TestFetchUnified_ContextCancelledDuringDispatch(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	items := ""
	for i := 0; i < 200; i++ {
		items += itemXML("ucc"+itoa(int64(i)), "T", "http://x/t.torrent")
	}
	srv := feedServerUnified(t, rssBody(items))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	site := &unifiedFake{
		enabled: true,
		detail:  &v2.TorrentItem{ID: "x", Title: "T", DiscountLevel: v2.DiscountFree, SizeBytes: 1024},
	}
	_ = FetchAndDownloadFreeRSSUnified(ctx, site, models.RSSConfig{Name: "r", URL: srv.URL})
}
