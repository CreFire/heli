package xsync

import (
	"sync"
	"time"
)

type Mutex struct {
	sync.Mutex
}

func (m *Mutex) Lock() {
	var spin uint16
	var now = time.Now()
	for !m.Mutex.TryLock() {
		spin = spinning(spin, now)
	}
}

type RWMutex struct {
	sync.RWMutex
}

func (m *RWMutex) Lock() {
	var spin uint16
	var now = time.Now()
	for !m.RWMutex.TryLock() {
		spin = spinning(spin, now)
	}
}

func (m *RWMutex) RLock() {
	var spin uint16
	var now = time.Now()
	for !m.RWMutex.TryRLock() {
		spin = spinning(spin, now)
	}
	return
}

func (m *RWMutex) RLocker() sync.Locker {
	return (*rLocker)(m)
}

type rLocker RWMutex

func (r *rLocker) Lock()   { (*RWMutex)(r).RLock() }
func (r *rLocker) Unlock() { (*RWMutex)(r).RUnlock() }
