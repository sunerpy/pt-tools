package sitelogin

import "time"

type Clock interface {
	Now() time.Time
}

type RealClock struct{}

func NewRealClock() *RealClock {
	return &RealClock{}
}

func (rc *RealClock) Now() time.Time {
	return time.Now()
}

type FakeClock struct {
	t time.Time
}

func NewFakeClock(t time.Time) *FakeClock {
	return &FakeClock{t: t}
}

func (fc *FakeClock) Now() time.Time {
	return fc.t
}

func (fc *FakeClock) Advance(d time.Duration) {
	fc.t = fc.t.Add(d)
}
