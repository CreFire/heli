package xlist

//官方库, 改成泛型

type Node[T any] struct {
	next, prev *Node[T]
	list       *List[T]
	Value      T
}

func (m *Node[T]) Next() *Node[T] {
	if p := m.next; m.list != nil && p != &m.list.root {
		return p
	}
	return nil
}

func (m *Node[T]) Prev() *Node[T] {
	if p := m.prev; m.list != nil && p != &m.list.root {
		return p
	}
	return nil
}

type List[T any] struct {
	root Node[T]
	len  int
}

func (m *List[T]) Init() *List[T] {
	m.root.next = &m.root
	m.root.prev = &m.root
	m.len = 0
	return m
}

func New[T any]() *List[T] { return new(List[T]).Init() }

func (m *List[T]) Len() int { return m.len }

func (m *List[T]) Front() *Node[T] {
	if m.len == 0 {
		return nil
	}
	return m.root.next
}

func (m *List[T]) Back() *Node[T] {
	if m.len == 0 {
		return nil
	}
	return m.root.prev
}

func (m *List[T]) lazyInit() {
	if m.root.next == nil {
		m.Init()
	}
}

func (m *List[T]) insert(e, at *Node[T]) *Node[T] {
	e.prev = at
	e.next = at.next
	e.prev.next = e
	e.next.prev = e
	e.list = m
	m.len++
	return e
}

func (m *List[T]) insertValue(v T, at *Node[T]) *Node[T] {
	return m.insert(&Node[T]{Value: v}, at)
}

func (m *List[T]) remove(e *Node[T]) {
	e.prev.next = e.next
	e.next.prev = e.prev
	e.next = nil
	e.prev = nil
	e.list = nil
	m.len--
}

func (m *List[T]) move(e, at *Node[T]) {
	if e == at {
		return
	}
	e.prev.next = e.next
	e.next.prev = e.prev

	e.prev = at
	e.next = at.next
	e.prev.next = e
	e.next.prev = e
}

func (m *List[T]) Remove(e *Node[T]) T {
	if e.list == m {
		m.remove(e)
	}
	return e.Value
}

func (m *List[T]) PushFront(v T) *Node[T] {
	m.lazyInit()
	return m.insertValue(v, &m.root)
}

func (m *List[T]) PushBack(v T) *Node[T] {
	m.lazyInit()
	return m.insertValue(v, m.root.prev)
}

func (m *List[T]) InsertBefore(v T, mark *Node[T]) *Node[T] {
	if mark.list != m {
		return nil
	}
	return m.insertValue(v, mark.prev)
}

func (m *List[T]) InsertAfter(v T, mark *Node[T]) *Node[T] {
	if mark.list != m {
		return nil
	}
	return m.insertValue(v, mark)
}

func (m *List[T]) MoveToFront(e *Node[T]) {
	if e.list != m || m.root.next == e {
		return
	}
	m.move(e, &m.root)
}

func (m *List[T]) MoveToBack(e *Node[T]) {
	if e.list != m || m.root.prev == e {
		return
	}
	m.move(e, m.root.prev)
}

func (m *List[T]) MoveBefore(e, mark *Node[T]) {
	if e.list != m || e == mark || mark.list != m {
		return
	}
	m.move(e, mark.prev)
}

func (m *List[T]) MoveAfter(e, mark *Node[T]) {
	if e.list != m || e == mark || mark.list != m {
		return
	}
	m.move(e, mark)
}

func (m *List[T]) PushBackList(other *List[T]) {
	m.lazyInit()
	for i, e := other.Len(), other.Front(); i > 0; i, e = i-1, e.Next() {
		m.insertValue(e.Value, m.root.prev)
	}
}

func (m *List[T]) PushFrontList(other *List[T]) {
	m.lazyInit()
	for i, e := other.Len(), other.Back(); i > 0; i, e = i-1, e.Prev() {
		m.insertValue(e.Value, &m.root)
	}
}

// 队列相关
func (m *List[T]) Range(f func(i int, node *Node[T]) bool) {
	m.lazyInit()
	for i, node := 0, m.root.next; i < m.Len(); i, node = i+1, node.next {
		f(i, node)
	}
}

func (m *List[T]) Push(v T) *Node[T] {
	m.lazyInit()
	return m.insertValue(v, m.root.prev)
}

func (m *List[T]) Pop() *Node[T] {
	m.lazyInit()
	if node := m.Front(); node != nil {
		m.remove(node)
		return node

	}
	return nil
}
