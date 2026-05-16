package notify

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

type fakeChannel struct {
	typ string
}

func (f *fakeChannel) Type() string                                             { return f.typ }
func (f *fakeChannel) Init(_ context.Context, _ *models.NotificationConf) error { return nil }
func (f *fakeChannel) SupportsInbound() bool                                    { return false }
func (f *fakeChannel) Send(_ context.Context, _ Notification) error             { return nil }
func (f *fakeChannel) OnInbound(_ InboundHandler)                               {}
func (f *fakeChannel) Close(_ context.Context) error                            { return nil }
func (f *fakeChannel) Healthy() bool                                            { return true }

func newFakeFactory(typ string) func() Channel {
	return func() Channel { return &fakeChannel{typ: typ} }
}

func TestRegistry_Register_Make(t *testing.T) {
	r := NewRegistry()
	r.Register("test_ch", newFakeFactory("test_ch"))

	ch, err := r.Make("test_ch")
	require.NoError(t, err)
	require.NotNil(t, ch)
	require.Equal(t, "test_ch", ch.Type())

	types := r.Types()
	require.Equal(t, []string{"test_ch"}, types)
}

func TestRegistry_UnknownType(t *testing.T) {
	r := NewRegistry()

	ch, err := r.Make("missing")
	require.Nil(t, ch)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrChannelTypeUnknown))
}

func TestRegistry_DuplicateRegister(t *testing.T) {
	r := NewRegistry()
	r.Register("dup", newFakeFactory("dup"))

	require.Panics(t, func() {
		r.Register("dup", newFakeFactory("dup"))
	})
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := NewRegistry()

	const workers = 16
	var wg sync.WaitGroup
	wg.Add(workers * 2)

	for i := 0; i < workers; i++ {
		typ := fakeTypeName(i)
		go func() {
			defer wg.Done()
			r.Register(typ, newFakeFactory(typ))
		}()
		go func() {
			defer wg.Done()
			_, _ = r.Make(typ)
			_ = r.Types()
		}()
	}
	wg.Wait()

	for i := 0; i < workers; i++ {
		typ := fakeTypeName(i)
		ch, err := r.Make(typ)
		require.NoError(t, err)
		require.Equal(t, typ, ch.Type())
	}
}

func fakeTypeName(i int) string {
	return "ch_" + string(rune('a'+i%26)) + string(rune('0'+i/26))
}

func TestNotification_DTORoundTrip(t *testing.T) {
	original := Notification{
		Title:        "标题",
		Text:         "正文 body",
		Image:        "https://example.com/img.png",
		Link:         "https://example.com/post",
		ChannelType:  "telegram",
		SourceConfID: 7,
		UserID:       "123",
		Targets: map[string]string{
			"chat_id": "-100",
		},
		Buttons: [][]Button{
			{
				{Text: "确认", CallbackData: "confirm"},
				{Text: "打开", URL: "https://example.com/open"},
			},
		},
		DisableWebPreview: true,
		OriginalMessageID: "42",
	}

	raw, err := json.Marshal(original)
	require.NoError(t, err)

	var got Notification
	require.NoError(t, json.Unmarshal(raw, &got))
	require.Equal(t, original, got)
}

func TestRegistry_RegisterChannel_DefaultRegistry(t *testing.T) {
	typ := "test_default_only"
	t.Cleanup(func() {
		defaultRegistry.mu.Lock()
		delete(defaultRegistry.factories, typ)
		defaultRegistry.mu.Unlock()
	})

	RegisterChannel(typ, newFakeFactory(typ))
	ch, err := DefaultRegistry().Make(typ)
	require.NoError(t, err)
	require.Equal(t, typ, ch.Type())
}
