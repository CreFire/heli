package basal

// 泛型有序集合
// SortedSet 使用泛型封装 SortedList

type SortedSet[T any] struct {
	*SortedList[T]
}

// my 与 b 的差集
func (my *SortedSet[T]) Difference(b *SortedSet[T]) *SortedSet[T] {
	c := &SortedSet[T]{SortedList: &SortedList[T]{
		buf:         append([]T{}, my.buf...),
		scoreRepeat: my.scoreRepeat,
		reverse:     my.reverse,
		getScore:    my.getScore,
		getKey:      my.getKey,
	}}
	for _, value := range b.buf {
		c.RemoveByKey(my.getKey(value))
	}
	return c
}

// 交集
func (my *SortedSet[T]) Intersection(b *SortedSet[T]) *SortedSet[T] {
	c := NewSortedSet[T](my.reverse, my.getScore, my.getKey)
	for _, value := range my.buf {
		if _, found := b.SearchByKey(my.getKey(value)); found {
			c.Add(value)
		}
	}
	return c
}

// 并集
func (my *SortedSet[T]) Union(b *SortedSet[T]) *SortedSet[T] {
	c := &SortedSet[T]{SortedList: &SortedList[T]{
		buf:         append([]T{}, my.buf...),
		scoreRepeat: my.scoreRepeat,
		reverse:     my.reverse,
		getScore:    my.getScore,
		getKey:      my.getKey,
	}}
	for _, value := range b.buf {
		c.Add(value)
	}
	return c
}

func (my *SortedSet[T]) Add(v T) bool {
	my.RemoveByKey(my.getKey(v))
	return my.SortedList.Add(v)
}

func NewSortedSet[T any](reverse bool, getScore func(v T) int64, getKey func(v T) int64) *SortedSet[T] {
	return &SortedSet[T]{SortedList: NewSortedList[T](true, reverse, getScore, getKey)}
}

func NewSortedSetInt(reverse bool) *SortedSet[int64] {
	getScore := func(v int64) int64 {
		return v
	}
	return NewSortedSet[int64](reverse, getScore, getScore)
}
