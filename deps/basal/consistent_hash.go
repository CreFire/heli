package basal

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"sync"
)

// HashFunc defines the hashing function used by the consistent hash ring.
type HashFunc func(key string) uint64

// NodeKey constrains the supported node types.
type NodeKey interface {
	~string | ~int64 | ~int32 | ~int
}

type ConsistentHash[T NodeKey] struct {
	replicas int
	hashFunc HashFunc

	ring    []uint64     // sorted hash ring
	nodes   map[uint64]T // virtual node hash -> physical node
	nodeSet map[T]struct{}

	mu sync.RWMutex
}

const defaultReplicas = 64

func NewConsistentHash[T NodeKey](replicas int, hashFunc HashFunc) *ConsistentHash[T] {
	if replicas <= 0 {
		replicas = defaultReplicas
	}
	if hashFunc == nil {
		hashFunc = SimpleStrHash
	}

	return &ConsistentHash[T]{
		replicas: replicas,
		hashFunc: hashFunc,
		ring:     make([]uint64, 0, 4*replicas),
		nodes:    make(map[uint64]T),
		nodeSet:  make(map[T]struct{}),
	}
}

func (c *ConsistentHash[T]) AddNode(node T) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.nodeSet[node]; ok {
		return false
	}

	for i := 0; i < c.replicas; i++ {
		hash := c.hashFunc(c.virtualNodeKey(node, i))
		c.ring = append(c.ring, hash)
		c.nodes[hash] = node
	}
	slices.Sort(c.ring)
	c.nodeSet[node] = struct{}{}
	return true
}

func (c *ConsistentHash[T]) RemoveNode(node T) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.nodeSet[node]; !ok {
		return false
	}

	delete(c.nodeSet, node)
	for i := 0; i < c.replicas; i++ {
		hash := c.hashFunc(c.virtualNodeKey(node, i))
		delete(c.nodes, hash)
	}

	// rebuild and sort ring after deletions
	c.ring = c.ring[:0]
	for hash := range c.nodes {
		c.ring = append(c.ring, hash)
	}
	slices.Sort(c.ring)

	return true
}

func (c *ConsistentHash[T]) GetNode(key any) (T, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.ring) == 0 {
		return *new(T), false
	}

	keyStr, ok := keyToString(key)
	if !ok {
		return *new(T), false
	}

	hash := c.hashFunc(keyStr)
	idx := sort.Search(len(c.ring), func(i int) bool {
		return c.ring[i] >= hash
	})
	if idx == len(c.ring) {
		idx = 0
	}
	return c.nodes[c.ring[idx]], true
}

func (c *ConsistentHash[T]) GetNodes(key any, count int) []T {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if count <= 0 || len(c.ring) == 0 {
		return nil
	}

	if count > len(c.nodeSet) {
		count = len(c.nodeSet)
	}

	keyStr, ok := keyToString(key)
	if !ok {
		return nil
	}

	hash := c.hashFunc(keyStr)
	idx := sort.Search(len(c.ring), func(i int) bool {
		return c.ring[i] >= hash
	})

	result := make([]T, 0, count)
	seen := make(map[T]struct{}, count)
	for len(result) < count {
		if idx == len(c.ring) {
			idx = 0
		}
		node := c.nodes[c.ring[idx]]
		if _, ok := seen[node]; !ok {
			result = append(result, node)
			seen[node] = struct{}{}
		}
		idx++

		// All physical nodes have been visited.
		if len(seen) == len(c.nodeSet) {
			break
		}
	}

	return result
}

func (c *ConsistentHash[T]) NodeCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.nodeSet)
}

func (c *ConsistentHash[T]) virtualNodeKey(node T, replica int) string {
	return fmt.Sprintf("%v#%d", node, replica)
}

func keyToString(key any) (string, bool) {
	switch v := key.(type) {
	case string:
		return v, true
	case int:
		return strconv.Itoa(v), true
	case int64:
		return strconv.FormatInt(v, 10), true
	case int32:
		return strconv.FormatInt(int64(v), 10), true
	default:
		return "", false
	}
}
