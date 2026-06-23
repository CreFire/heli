package controller

import (
	"math/rand"
	"sync"
	"time"
)

type WeightItem struct {
	Name   string
	Weight int32
}

type WeightPicker struct {
	mu     sync.RWMutex
	items  []WeightItem
	prefix []int32 // 前缀和
	sum    int32
	rnd    *rand.Rand
	index  map[string]int // 名称到下标的映射
}

func NewWeightPicker(items ...WeightItem) *WeightPicker {
	wp := &WeightPicker{
		rnd:   rand.New(rand.NewSource(time.Now().UnixNano())),
		index: make(map[string]int),
	}
	wp.reset2(items)
	return wp
}

func (w *WeightPicker) reset2(items []WeightItem) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.items = w.items[:0]
	w.prefix = w.prefix[:0]
	w.sum = 0
	w.index = make(map[string]int)

	for _, it := range items {
		if it.Weight <= 0 || it.Name == "" {
			continue
		}
		w.items = append(w.items, it)
		w.sum += it.Weight
		w.prefix = append(w.prefix, w.sum)
		w.index[it.Name] = len(w.items) - 1
	}
}

// 更新或新增某个权重；权重小于等于 0 表示移除
func (w *WeightPicker) Update(name string, weight int32) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	// 先把当前条目拷贝出来，更新后再整体重建
	tmp := make([]WeightItem, 0, len(w.items)+1)
	seen := false
	for _, it := range w.items {
		if it.Name == name {
			seen = true
			if weight > 0 {
				tmp = append(tmp, WeightItem{Name: name, Weight: weight})
			}
		} else {
			tmp = append(tmp, it)
		}
	}
	if !seen && weight > 0 {
		tmp = append(tmp, WeightItem{Name: name, Weight: weight})
	}
	w.items = nil
	w.prefix = nil
	w.sum = 0
	w.index = make(map[string]int)

	for _, it := range tmp {
		w.items = append(w.items, it)
		w.sum += it.Weight
		w.prefix = append(w.prefix, w.sum)
		w.index[it.Name] = len(w.items) - 1
	}
}

// 按权重随机返回一个名称
func (w *WeightPicker) Pick() (string, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.sum <= 0 || len(w.items) == 0 {
		return "", false
	}
	x := w.rnd.Int31n(w.sum)
	// 这里直接线性遍历即可，当前项目吞吐下足够使用
	for i, ps := range w.prefix {
		if x < ps {
			return w.items[i].Name, true
		}
	}
	return "", false
}
