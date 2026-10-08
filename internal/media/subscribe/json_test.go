package subscribe

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

// 页面按数组用这些字段：空的也要是 []，不能是 null
func TestViewsEncodeEmptyListsAsArrays(t *testing.T) {
	e := newEnv(t)
	js := func(v any) string {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		return string(b)
	}

	set, err := e.svc.Settings(e.ctx)
	require.NoError(t, err)
	assert.Contains(t, js(set), `"search_skip_sites":[]`)
	assert.Contains(t, js(set), `"notify_channels":[]`)

	p, err := e.svc.SaveProfile(e.ctx, 0, ProfileInput{Name: "空"})
	require.NoError(t, err)
	for _, k := range []string{"resolutions", "sources", "codecs", "groups"} {
		assert.Contains(t, js(p), `"`+k+`":[]`, k)
	}
	list, err := e.svc.Profiles(e.ctx)
	require.NoError(t, err)
	assert.Contains(t, js(list), `"groups":[]`)

	m := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	subs, err := e.svc.Subscriptions(e.ctx, SubscriptionQuery{})
	require.NoError(t, err)
	assert.Contains(t, js(subs), `"sites":[]`)
	d, err := e.svc.Subscription(e.ctx, m.ID)
	require.NoError(t, err)
	assert.Contains(t, js(d), `"sites":[]`)
	assert.Contains(t, js(d), `"torrent_list":[]`)
}
