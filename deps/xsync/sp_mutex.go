package xsync

import (
	"game/deps/basal"
	"game/deps/xlog"
	"sync/atomic"
	"time"
)

const readAddMin = 1 << 1

// 自旋锁,适用写多读少
type SPMutex struct {
	state int64
}

func (m *SPMutex) TryLock() bool {
	return atomic.CompareAndSwapInt64(&m.state, 0, 1)
}

func (m *SPMutex) Lock() {
	var spin uint16
	now := time.Now()
	for !atomic.CompareAndSwapInt64(&m.state, 0, 1) {
		spin = spinning(spin, now)
	}
}

func (m *SPMutex) Unlock() {
	if !atomic.CompareAndSwapInt64(&m.state, 1, 0) {
		xlog.Errorf("SPMutex.Unlock error, 解锁时机不对(未加锁而解锁,多次解锁等): %v", basal.StackLine(2))
	}
}

func (m *SPMutex) TryRLock() bool {
	state := m.state
	before := state &^ 1
	after := before + readAddMin
	if !atomic.CompareAndSwapInt64(&m.state, before, after) {
		return false
	}
	return true
}

func (m *SPMutex) RLock() {
	var spin uint16
	var state, before, after int64
	var now = time.Now()
	for {
		state = m.state
		before = state &^ 1
		after = before + readAddMin
		if atomic.CompareAndSwapInt64(&m.state, before, after) {
			return
		}
		spin = spinning(spin, now)
	}
}

func (m *SPMutex) RUnlock() {
	if v := atomic.AddInt64(&m.state, -readAddMin); v < 0 {
		xlog.Errorf("SPMutex.RUnlock error, 解锁时机不对(未加锁而解锁,多次解锁等): %v, %v", basal.StackLine(2), v>>1)
		time.Sleep(time.Second)
	}
}
