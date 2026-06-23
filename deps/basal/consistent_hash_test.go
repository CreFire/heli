package basal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConsistentHashAddAndGet(t *testing.T) {
	ch := NewConsistentHash[string](10, nil)
	nodes := []string{"nodeA", "nodeB", "nodeC"}
	for _, n := range nodes {
		added := ch.AddNode(n)
		require.True(t, added)
	}

	keys := []string{"user-1", "user-2", "user-3", "user-4"}
	for _, key := range keys {
		first, ok := ch.GetNode(key)
		require.True(t, ok)
		assert.Contains(t, nodes, first)

		second, ok := ch.GetNode(key)
		require.True(t, ok)
		assert.Equal(t, first, second, "same key should map to same node")
	}
}

func TestConsistentHashRemove(t *testing.T) {
	ch := NewConsistentHash[string](5, nil)
	ch.AddNode("nodeA")
	ch.AddNode("nodeB")

	key := "target-key"
	first, ok := ch.GetNode(key)
	require.True(t, ok)

	assert.True(t, ch.RemoveNode(first))
	assert.Equal(t, 1, ch.NodeCount())
	assert.False(t, ch.RemoveNode("missing-node"))

	second, ok := ch.GetNode(key)
	require.True(t, ok)
	assert.NotEqual(t, first, second, "key should move after node removal")
}

func TestConsistentHashGetNodes(t *testing.T) {
	ch := NewConsistentHash[string](20, nil)
	ch.AddNode("nodeA")
	ch.AddNode("nodeB")
	ch.AddNode("nodeC")

	got := ch.GetNodes("group-key", 2)
	require.Len(t, got, 2)
	assert.Equal(t, got, ch.GetNodes("group-key", 2), "results should be stable for the same key")
	assertUnique(t, got)

	gotAll := ch.GetNodes("group-key", 10)
	assert.Len(t, gotAll, 3)
	assertUnique(t, gotAll)
	assert.ElementsMatch(t, []string{"nodeA", "nodeB", "nodeC"}, gotAll)
}

func TestConsistentHashWithInt64Nodes(t *testing.T) {
	ch := NewConsistentHash[int64](15, nil)
	ch.AddNode(1)
	ch.AddNode(2)
	ch.AddNode(3)

	primary, ok := ch.GetNode("int-key")
	require.True(t, ok)
	assert.Contains(t, []int64{1, 2, 3}, primary)

	all := ch.GetNodes("int-key", 3)
	assert.Len(t, all, 3)
	assertUniqueInt64(t, all)
}

func TestConsistentHashIntKey(t *testing.T) {
	ch := NewConsistentHash[string](10, nil)
	ch.AddNode("nodeA")
	ch.AddNode("nodeB")

	key := int64(12345)
	first, ok := ch.GetNode(key)
	require.True(t, ok)
	second, ok := ch.GetNode(key)
	require.True(t, ok)
	assert.Equal(t, first, second)

	nodes := ch.GetNodes(key, 2)
	assertUnique(t, nodes)
	assert.Len(t, nodes, 2)
}

func assertUnique(t *testing.T, values []string) {
	t.Helper()
	seen := make(map[string]struct{}, len(values))
	for _, v := range values {
		if _, ok := seen[v]; ok {
			t.Fatalf("value %s appears more than once", v)
		}
		seen[v] = struct{}{}
	}
}

func assertUniqueInt64(t *testing.T, values []int64) {
	t.Helper()
	seen := make(map[int64]struct{}, len(values))
	for _, v := range values {
		if _, ok := seen[v]; ok {
			t.Fatalf("value %d appears more than once", v)
		}
		seen[v] = struct{}{}
	}
}
