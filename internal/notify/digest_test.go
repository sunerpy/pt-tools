package notify

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDigestBuffer_BelowThreshold_NoImmediateFlush(t *testing.T) {
	var calls int32
	var mu sync.Mutex
	var got [][]DigestItem
	flush := func(_ context.Context, _ uint, items []DigestItem) {
		atomic.AddInt32(&calls, 1)
		mu.Lock()
		got = append(got, items)
		mu.Unlock()
	}
	b := NewDigestBufferWithWindow(context.Background(), flush, 200*time.Millisecond, 5)
	for i := 0; i < 4; i++ {
		b.Add(1, DigestItem{LogID: uint(i + 1), Title: "t", Text: "x"})
	}
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(0), atomic.LoadInt32(&calls), "should not flush before window or threshold")

	time.Sleep(250 * time.Millisecond)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "should flush once after window")
	mu.Lock()
	assert.Len(t, got[0], 4)
	mu.Unlock()
}

func TestDigestBuffer_ThresholdReached_ImmediateFlush(t *testing.T) {
	var calls int32
	flushed := make(chan []DigestItem, 1)
	flush := func(_ context.Context, _ uint, items []DigestItem) {
		atomic.AddInt32(&calls, 1)
		flushed <- items
	}
	b := NewDigestBufferWithWindow(context.Background(), flush, 5*time.Second, 5)
	for i := 0; i < 5; i++ {
		b.Add(2, DigestItem{LogID: uint(i + 1), Title: "t", Text: "x"})
	}
	select {
	case items := <-flushed:
		assert.Len(t, items, 5)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected immediate flush at threshold")
	}
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestDigestBuffer_FlushAll_DrainsPending(t *testing.T) {
	var calls int32
	flushed := make(chan []DigestItem, 4)
	flush := func(_ context.Context, _ uint, items []DigestItem) {
		atomic.AddInt32(&calls, 1)
		flushed <- items
	}
	b := NewDigestBufferWithWindow(context.Background(), flush, 5*time.Second, 5)
	b.Add(1, DigestItem{LogID: 10, Title: "a"})
	b.Add(2, DigestItem{LogID: 20, Title: "b"})
	b.FlushAll()

	collected := 0
	deadline := time.After(300 * time.Millisecond)
	for collected < 2 {
		select {
		case <-flushed:
			collected++
		case <-deadline:
			t.Fatalf("expected 2 flushes from FlushAll, got %d", collected)
		}
	}
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
}

func TestDigestBuffer_PerConfIsolation(t *testing.T) {
	flushed := make(chan uint, 4)
	flush := func(_ context.Context, confID uint, _ []DigestItem) {
		flushed <- confID
	}
	b := NewDigestBufferWithWindow(context.Background(), flush, 5*time.Second, 2)
	b.Add(7, DigestItem{LogID: 1, Title: "a"})
	b.Add(8, DigestItem{LogID: 2, Title: "b"})
	b.Add(7, DigestItem{LogID: 3, Title: "c"})
	select {
	case got := <-flushed:
		assert.Equal(t, uint(7), got, "conf 7 should flush at threshold first")
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected flush for conf 7")
	}
}

func TestCombineDigest_Empty(t *testing.T) {
	title, text := CombineDigest(nil)
	assert.Empty(t, title)
	assert.Empty(t, text)
}

func TestCombineDigest_Single(t *testing.T) {
	items := []DigestItem{{Title: "T1", Text: "Body"}}
	title, text := CombineDigest(items)
	assert.Equal(t, "T1", title)
	assert.Equal(t, "Body", text)
}

func TestCombineDigest_Multiple(t *testing.T) {
	items := []DigestItem{
		{Title: "alpha"},
		{Title: "beta"},
		{Title: "gamma"},
	}
	title, text := CombineDigest(items)
	assert.Contains(t, title, "3")
	assert.Contains(t, text, "📦 3 条新通知")
	assert.Contains(t, text, "1. alpha")
	assert.Contains(t, text, "2. beta")
	assert.Contains(t, text, "3. gamma")
}

func TestCombineDigest_TruncatedAtMax(t *testing.T) {
	items := make([]DigestItem, DigestMaxItems+10)
	for i := range items {
		items[i] = DigestItem{Title: "x"}
	}
	_, text := CombineDigest(items)
	assert.Contains(t, text, "还有 10 条已省略")
}

// TestNewDigestBuffer_Defaults verifies the convenience constructor wires the
// default window/threshold.
func TestNewDigestBuffer_Defaults(t *testing.T) {
	b := NewDigestBuffer(context.Background(), func(_ context.Context, _ uint, _ []DigestItem) {})
	require.NotNil(t, b)
	assert.Equal(t, DigestWindow, b.window)
	assert.Equal(t, DigestThreshold, b.threshold)
}

// TestNewDigestBufferWithWindow_DefaultsOnNonPositive covers the <=0 branches.
func TestNewDigestBufferWithWindow_DefaultsOnNonPositive(t *testing.T) {
	b := NewDigestBufferWithWindow(context.Background(), func(_ context.Context, _ uint, _ []DigestItem) {}, 0, 0)
	assert.Equal(t, DigestWindow, b.window)
	assert.Equal(t, DigestThreshold, b.threshold)
}
