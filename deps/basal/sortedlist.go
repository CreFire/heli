package basal

import (
	"fmt"
	"sort"
)

// 泛型有序列表
type SortedList[T any] struct {
	buf         []T
	scoreRepeat bool            // 排序值是否可重复
	reverse     bool            // 是否反序
	getScore    func(v T) int64 // 获取分数函数
	getKey      func(v T) int64 // 获取键值函数
}

func (my *SortedList[T]) String() string {
	return fmt.Sprintf("%v", my.buf)
}

func (my *SortedList[T]) GetSafe(index int) (T, error) {
	if index < 0 || index >= len(my.buf) {
		return *new(T), fmt.Errorf("index out of bounds")
	}
	return my.buf[index], nil
}

func (my *SortedList[T]) BatchRemoveByKeys(keys []int64) {
	for _, key := range keys {
		my.RemoveByKey(key)
	}
}

// 优化后的 `reduceSpace` 方法
func (my *SortedList[T]) reduceSpace() {
	if len(my.buf) == 0 {
		my.buf = nil
	} else if cap(my.buf) > len(my.buf)*2 {
		my.buf = my.buf[:len(my.buf)]
	}
}

func (my *SortedList[T]) Len() int {
	return len(my.buf)
}

func (my *SortedList[T]) Cap() int {
	return cap(my.buf)
}

func (my *SortedList[T]) Slice() []T {
	return my.buf
}

func (my *SortedList[T]) cmp(min, max int64) int {
	if min < max {
		return 1
	} else if min > max {
		return -1
	} else {
		return 0
	}
}

func (my *SortedList[T]) SearchByKey(key int64) (int, bool) {
	for idx, item := range my.buf {
		if my.getKey(item) == key {
			return idx, true
		}
	}
	return 0, false
}

func (my *SortedList[T]) SearchByScore(score int64) (int, bool) {
	length := len(my.buf)
	if length == 0 {
		return 0, false
	}
	index := sort.Search(length, func(i int) bool {
		cmp := my.getScore(my.buf[i])
		if my.reverse {
			return cmp <= score
		}
		return cmp >= score
	})
	if index < length && my.getScore(my.buf[index]) == score {
		return index, true
	}
	return index, false
}

func (my *SortedList[T]) Front() (v T, found bool) {
	if len(my.buf) > 0 {
		return my.buf[0], true
	}
	var zero T
	return zero, false
}

func (my *SortedList[T]) Back() (v T, found bool) {
	length := len(my.buf)
	if length > 0 {
		return my.buf[length-1], true
	}
	var zero T
	return zero, false
}

func (my *SortedList[T]) PopFront() (v T, found bool) {
	if len(my.buf) > 0 {
		v = my.buf[0]
		my.buf = my.buf[1:]
		my.reduceSpace()
		return v, true
	}
	var zero T
	return zero, false
}

func (my *SortedList[T]) PopBack() (v T, found bool) {
	length := len(my.buf)
	if length > 0 {
		v = my.buf[length-1]
		my.buf = my.buf[:length-1]
		my.reduceSpace()
		return v, true
	}
	var zero T
	return zero, false
}

func (my *SortedList[T]) Get(index int) (v T, found bool) {
	length := len(my.buf)
	if index >= 0 && index < length {
		return my.buf[index], true
	}
	var zero T
	return zero, false
}
func (my *SortedList[T]) Add(v T) bool {
	index, found := my.SearchByScore(my.getScore(v))
	if found && !my.scoreRepeat {
		return false
	}
	my.buf = append(my.buf, v)             // 扩展容量
	copy(my.buf[index+1:], my.buf[index:]) // 向右移动元素
	my.buf[index] = v                      // 插入元素
	return true
}

// 优化后的 `SortedList` 添加方法，避免重复插入后重新排序
func (my *SortedList[T]) BatchAdd(values []T) {
	sortedValues := make([]T, len(values))
	copy(sortedValues, values)
	sort.Slice(sortedValues, func(i, j int) bool {
		return my.getScore(sortedValues[i]) > my.getScore(sortedValues[j]) // 根据分数排序
	})
	for _, v := range sortedValues {
		my.buf = append(my.buf, v)
	}
	// 重新排序一次
	sort.Slice(my.buf, func(i, j int) bool {
		return my.getScore(my.buf[i]) > my.getScore(my.buf[j]) // 根据分数排序
	})
}
func (my *SortedList[T]) RemoveByIndex(index int) bool {
	if index < 0 || index >= len(my.buf) {
		return false
	}
	my.buf = append(my.buf[:index], my.buf[index+1:]...)
	my.reduceSpace()
	return true
}

func (my *SortedList[T]) RemoveByScore(score int64) bool {
	index, found := my.SearchByScore(score)
	if found {
		my.buf = append(my.buf[:index], my.buf[index+1:]...)
		my.reduceSpace()
		return true
	}
	return false
}

func (my *SortedList[T]) RemoveByKey(key int64) bool {
	index, found := my.SearchByKey(key)
	if found {
		my.buf = append(my.buf[:index], my.buf[index+1:]...)
		my.reduceSpace()
		return true
	}
	return false
}

func (my *SortedList[T]) Clear() {
	my.buf = nil
}

func NewSortedList[T any](scoreRepeat, reverse bool, getScore func(v T) int64, getKey func(v T) int64) *SortedList[T] {
	return &SortedList[T]{scoreRepeat: scoreRepeat, reverse: reverse, getScore: getScore, getKey: getKey}
}
func NewSortedListWithComparator[T any](comparator func(a, b T) bool) *SortedList[T] {
	return &SortedList[T]{
		buf:         make([]T, 0, 10), // 预留初始容量，减少扩容次数
		scoreRepeat: true,
		reverse:     false,
		getScore: func(v T) int64 {
			// 自定义比较器转为分数形式，0 和 1 控制顺序比较
			if comparator(v, v) {
				return 1
			}
			return 0
		},
		getKey: func(v T) int64 {
			// 模拟唯一 key，可根据需求扩展
			return 0
		},
	}
}
