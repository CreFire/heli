package basal

import (
	"strings"
)

type sensitiveWordsNode struct {
	child map[rune]*sensitiveWordsNode
	v     rune
	count uint32
}

func (m *sensitiveWordsNode) add(r rune) (child *sensitiveWordsNode) {
	if m.child == nil {
		m.child = map[rune]*sensitiveWordsNode{}
	}
	if node, ok := m.child[r]; ok {
		return node
	}
	node := &sensitiveWordsNode{child: map[rune]*sensitiveWordsNode{}}
	m.child[r] = node
	return node
}

func (m *sensitiveWordsNode) get(r rune) *sensitiveWordsNode {
	if m.child == nil {
		return nil
	}
	if v, ok := m.child[r]; ok {
		return v
	}
	return nil
}

type SensitiveWordsMatcher struct {
	root *sensitiveWordsNode
}

func (m *SensitiveWordsMatcher) Add(rs []rune) {
	if m.root == nil {
		m.root = &sensitiveWordsNode{child: map[rune]*sensitiveWordsNode{}}
	}
	dLen := len(rs)
	node := m.root
	for i, c := range rs {
		node = node.add(c)
		if i+1 == dLen {
			node.count += 1
		}
	}
}

// 添加敏感词
func (m *SensitiveWordsMatcher) AddStr(str string) {
	m.Add([]rune(strings.ToLower(str)))
}

// 返回-1 没找到
func (m *SensitiveWordsMatcher) has(rs []rune) int {
	node := m.root
	for i, r := range rs {
		if node = node.get(r); node == nil {
			return -1
		}
		if node.count > 0 {
			return i
		}
	}
	return -1
}

func (m *SensitiveWordsMatcher) Has(rs []rune, repl rune) (res []rune) {
	for i := range rs {
		if add := m.has(rs[i:]); add >= 0 {
			res = make([]rune, len(rs))
			a, b := i, i+add
			for x := range rs {
				if x >= a && x <= b {
					res[x] = repl
				} else {
					res[x] = rs[x]
				}
			}
			return
		}
	}
	return
}

func (m *SensitiveWordsMatcher) HasStr(str string, repl rune) string {
	return string(m.Has([]rune(strings.ToLower(str)), repl))
}
