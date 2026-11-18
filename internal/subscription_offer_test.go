package internal

import (
	"testing"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// RSS 取到详情的种子交给订阅：没登记时不做事；详情里没有种子编号时从链接里取，再没有用 GUID
func TestOfferToSubscriptions(t *testing.T) {
	t.Cleanup(func() { SetSubscriptionOffer(nil) })
	offerToSubscriptions("hdsky", &v2.TorrentItem{Title: "A"}, &gofeed.Item{GUID: "1"})

	var got []v2.TorrentItem
	SetSubscriptionOffer(func(site string, it v2.TorrentItem) {
		assert.Equal(t, "hdsky", site)
		got = append(got, it)
	})
	offerToSubscriptions("hdsky", &v2.TorrentItem{Title: "A"}, &gofeed.Item{Link: "https://hdsky.me/details.php?id=123&hit=1", GUID: "g"})
	offerToSubscriptions("hdsky", &v2.TorrentItem{}, &gofeed.Item{Title: "B", GUID: "456"})
	offerToSubscriptions("hdsky", &v2.TorrentItem{ID: "789", Title: "C", SourceSite: "other"}, nil)
	offerToSubscriptions("hdsky", nil, &gofeed.Item{})
	if assert.Len(t, got, 3) {
		assert.Equal(t, "123", got[0].ID)
		assert.Equal(t, "hdsky", got[0].SourceSite)
		assert.Equal(t, "456", got[1].ID)
		assert.Equal(t, "B", got[1].Title)
		assert.Equal(t, "789", got[2].ID)
		assert.Equal(t, "other", got[2].SourceSite)
	}
}
