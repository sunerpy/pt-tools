package events

import (
	"sync"
	"sync/atomic"
	"time"
)

type EventType string

const (
	ConfigChanged EventType = "ConfigChanged"
	DiskSpaceLow  EventType = "DiskSpaceLow"
)

type Event struct {
	Type    EventType
	Version int64
	Source  string
	At      time.Time
}

var (
	mu   sync.RWMutex
	subs = map[string]chan Event{}
	sid  int64
)

func Subscribe(buffer int) (string, <-chan Event, func()) {
	if buffer <= 0 {
		buffer = 16
	}
	id := nextID()
	ch := make(chan Event, buffer)
	mu.Lock()
	subs[id] = ch
	mu.Unlock()
	cancel := func() {
		mu.Lock()
		if c, ok := subs[id]; ok {
			delete(subs, id)
			close(c)
		}
		mu.Unlock()
	}
	return id, ch, cancel
}

func Publish(e Event) {
	mu.RLock()
	defer mu.RUnlock()
	for _, ch := range subs {
		select {
		case ch <- e:
		default:
		}
	}
}

func nextID() string {
	n := atomic.AddInt64(&sid, 1)
	return "sub-" + itoa(n)
}

func itoa(n int64) string {
	b := [20]byte{}
	i := len(b)
	u := uint64(n)
	for {
		i--
		b[i] = byte('0' + u%10)
		u /= 10
		if u == 0 {
			break
		}
	}
	return string(b[i:])
}
