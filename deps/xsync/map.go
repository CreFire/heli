package xsync

import (
	"game/deps/basal"
	"game/deps/xlog"
	"sync"
)

type Map[K comparable, V any] struct {
	data map[K]V
	mu   sync.RWMutex
}

func NewMap[K comparable, V any]() *Map[K, V] {
	data := &Map[K, V]{data: make(map[K]V)}
	return data
}

// 返回老数据
func (m *Map[K, V]) Set(key K, value V) (old V, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	old, ok = m.data[key]
	m.data[key] = value
	return
}

func (m *Map[K, V]) Get(key K) (value V, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok = m.data[key]
	return
}

// 函数内不可再调本Map方法和可能存在阻塞风险方法
func (m *Map[K, V]) GetOrNew(key K, newV func() V) (value V) {
	if v, ok := m.Get(key); ok {
		return v
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.data[key]; ok {
		return v
	}
	value = newV()
	m.data[key] = value
	return
}

func (m *Map[K, V]) GetOrSet(key K, value V) V {
	if v, ok := m.Get(key); ok {
		return v
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.data[key]; ok {
		return v
	}
	m.data[key] = value
	return value
}

// 函数内不可再调本Map方法和可能存在阻塞风险方法
func (m *Map[K, V]) Delete(key K, f func(value V) bool) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[key]
	if !ok {
		return false
	}
	if f != nil && !f(v) {
		return false
	}
	delete(m.data, key)
	return true
}

func (m *Map[K, V]) exec(f func(K, V) bool, key K, value V) bool {
	defer basal.Exception(func(err error) {
		xlog.Errorf("map exec func error: %v", basal.Stack(err))
	})
	return f(key, value)
}

func (m *Map[K, V]) Range(f func(key K, value V) bool) bool {
	if f == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for k, v := range m.data {
		if !m.exec(f, k, v) {
			return false
		}
	}

	return true
}

// 函数内不可再调本Map方法和可能存在阻塞风险方法
func (m *Map[K, V]) RangeDelete(rangeN int, f func(key K, value V) bool) int {
	if f == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var count, delN int
	for k, v := range m.data {
		count++
		if m.exec(f, k, v) {
			delete(m.data, k)
			delN++
		}
		if rangeN > 0 && count >= rangeN {
			break
		}
	}
	return delN
}

func (m *Map[K, V]) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.data)
}
