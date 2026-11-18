package feishuapp

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/notify"
)

// fakeClock 是可以手动拨动的时钟。
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func TestDedupe_TTLAndForget(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_800_000_000, 0)}
	d := newDedupe(time.Hour, 100)
	d.now = clock.now

	assert.False(t, d.check("a"))
	assert.True(t, d.check("a"), "重推的同一条")
	clock.add(59 * time.Minute)
	assert.True(t, d.check("a"))
	clock.add(2 * time.Minute)
	assert.False(t, d.check("a"), "过期以后重新算")

	assert.False(t, d.check("b"))
	d.forget("b")
	assert.False(t, d.check("b"), "移出表以后能再处理")
	assert.True(t, d.check("b"))
}

func TestDedupe_EvictsOldestWhenFull(t *testing.T) {
	d := newDedupe(time.Hour, 3)
	for i := range 4 {
		assert.False(t, d.check(strconv.Itoa(i)))
	}
	assert.False(t, d.check("0"), "最旧的被挤掉了")
	assert.True(t, d.check("3"))
	assert.LessOrEqual(t, len(d.at), 3)
}

func TestDedupe_ForgetThenReAddSurvivesStaleEntry(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_800_000_000, 0)}
	d := newDedupe(time.Hour, 100)
	d.now = clock.now
	require.False(t, d.check("k"))
	d.forget("k")
	clock.add(30 * time.Minute)
	require.False(t, d.check("k"))
	clock.add(40 * time.Minute) // 第一次记下的已经过期，第二次的还没有
	assert.True(t, d.check("k"), "过期的旧顺序项不能把新记下的删掉")
}

func TestThrottle(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_800_000_000, 0)}
	th := newThrottle(10 * time.Minute)
	th.now = clock.now
	assert.True(t, th.allow("ou_a"))
	assert.False(t, th.allow("ou_a"))
	assert.True(t, th.allow("ou_b"))
	clock.add(10 * time.Minute)
	assert.True(t, th.allow("ou_a"))

	for i := range throttleMax {
		th.allow("ou_" + strconv.Itoa(i))
	}
	clock.add(11 * time.Minute)
	assert.True(t, th.allow("ou_new"))
	assert.Less(t, len(th.last), throttleMax, "满了以后清掉过期的")
}

// 名单外提示的节流表有硬上限：清掉过期的以后仍然满了（10 分钟里来的全是不同的人），就丢掉最旧的一条。
func TestThrottle_HardCap(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_800_000_000, 0)}
	th := newThrottle(10 * time.Minute)
	th.now = clock.now
	for i := range throttleMax {
		require.True(t, th.allow("ou_"+strconv.Itoa(i)))
		clock.add(time.Millisecond)
	}
	for i := range 50 {
		assert.True(t, th.allow("ou_new"+strconv.Itoa(i)))
		assert.LessOrEqual(t, len(th.last), throttleMax, "表不超过上限")
	}
	assert.True(t, th.allow("ou_0"), "最旧的被挤掉了，再来时又能提示")
	assert.False(t, th.allow("ou_new49"), "新的还在表里")
}

func TestAppRegistry(t *testing.T) {
	r := newAppRegistry()
	a := &Channel{confID: 1}
	b := &Channel{confID: 2}
	_, ok, _, _ := r.claim("cli_x", a)
	require.True(t, ok)
	_, ok, _, _ = r.claim("cli_x", a)
	assert.True(t, ok, "自己重复登记没问题")
	holder, ok, wake, _ := r.claim("cli_x", b)
	assert.False(t, ok)
	assert.Equal(t, uint(1), holder)
	r.release("cli_x", b, handoff{})
	_, ok, _, _ = r.claim("cli_x", b)
	assert.False(t, ok, "别人释放不了我的登记")
	select {
	case <-wake:
		t.Fatal("还没释放就叫醒了")
	default:
	}
	r.release("cli_x", a, handoff{})
	select {
	case <-wake:
	default:
		t.Fatal("释放时要叫醒等着的实例")
	}
	_, ok, _, _ = r.claim("cli_x", b)
	assert.True(t, ok)
}

// 留下的消息只交给通道编号相同的下一个实例，取走以后就没有了；不是登记主人的实例释放不了、也留不下消息。
func TestAppRegistry_LeftoversGoToSameConf(t *testing.T) {
	r := newAppRegistry()
	defer r.reset()
	item := newQueued(notify.InboundMessage{ChannelUserID: "ou_a", Text: "x"}, time.Unix(1_800_000_000, 0), "om_x")
	old := &Channel{confID: 7}
	_, ok, _, _ := r.claim("cli_m", old)
	require.True(t, ok)
	r.release("cli_m", &Channel{confID: 7}, handoff{items: map[string][]*queued{"ou_a": {item}}})
	assert.Empty(t, r.left, "不是登记的主人：什么都不做")
	r.release("cli_m", old, handoff{items: map[string][]*queued{"ou_a": {item}}})

	other := &Channel{confID: 8}
	_, ok, _, got := r.claim("cli_m", other)
	require.True(t, ok)
	assert.True(t, got.empty(), "别的通道编号拿不到")
	r.release("cli_m", other, handoff{})

	next := &Channel{confID: 7}
	_, ok, _, got = r.claim("cli_m", next)
	require.True(t, ok)
	assert.Equal(t, map[string][]*queued{"ou_a": {item}}, got.items)
	r.release("cli_m", next, handoff{})
	_, _, _, got = r.claim("cli_m", &Channel{confID: 7})
	assert.True(t, got.empty(), "取走以后就没有了")
}
