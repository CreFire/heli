package xsync

import (
	"fmt"
	"game/deps/basal"
	"math"
	"reflect"
	"unsafe"
)

// n是否为2的指数幂
func IsPow2(n int) bool {
	return n > 0 && (n&(n-1)) == 0
}

// 找到最小的大于n的2的指数幂
func NextPow2(n int) int {
	if n <= 0 {
		return 1
	}
	n -= 1
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	return n + 1
}

const prime32 = uint32(16777619)
const offsetBasis = uint32(2166136261)

var hashFNV32Uint64 = func(key uint64) uint32 {
	hash := offsetBasis
	hash *= prime32
	hash ^= uint32(uint8(key >> 56))
	hash *= prime32
	hash ^= uint32(uint8(key >> 48))
	hash *= prime32
	hash ^= uint32(uint8(key >> 40))
	hash *= prime32
	hash ^= uint32(uint8(key >> 32))
	hash *= prime32
	hash ^= uint32(uint8(key >> 24))
	hash *= prime32
	hash ^= uint32(uint8(key >> 16))
	hash *= prime32
	hash ^= uint32(uint8(key >> 8))
	hash *= prime32
	hash ^= uint32(uint8(key))
	return hash
}

var hashFNV32String = func(key string) uint32 {
	hash := offsetBasis
	for i := 0; i < len(key); i++ {
		hash *= prime32
		hash ^= uint32(key[i])
	}
	return hash
}

type KEY interface {
	basal.Integer | string
}

func NewShardMap[K KEY, V any](shardNum int) *ShardMap[K, V] {
	shardNum = NextPow2(shardNum)
	if shardNum > math.MaxUint32 {
		shardNum = math.MaxUint32 + 1
	}
	shard := make([]*Map[K, V], shardNum)
	for i := 0; i < shardNum; i++ {
		shard[i] = NewMap[K, V]()
	}
	var hashFNV32 func(K) uint32
	var key K
	kind := reflect.ValueOf(&key).Elem().Type().Kind()
	switch kind {
	case reflect.Int8, reflect.Uint8, reflect.Int16, reflect.Uint16, reflect.Int32, reflect.Uint32, reflect.Int64, reflect.Uint64, reflect.Int, reflect.Uint:
		hashFNV32 = *(*func(K) uint32)(unsafe.Pointer(&hashFNV32Uint64))
	case reflect.String:
		hashFNV32 = *(*func(K) uint32)(unsafe.Pointer(&hashFNV32String))
	default:
		panic(fmt.Errorf("shard map unsupported key type %T of kind %v", key, kind))
	}
	return &ShardMap[K, V]{shard: shard, mask: uint32(shardNum - 1), hashFNV32: hashFNV32}
}

type ShardMap[K KEY, V any] struct {
	shard     []*Map[K, V]
	hashFNV32 func(K) uint32
	mask      uint32
}

func (m *ShardMap[K, V]) Set(key K, value V) (old V, ok bool) {
	return m.shard[m.hashFNV32(key)&m.mask].Set(key, value)
}

func (m *ShardMap[K, V]) Get(key K) (value V, ok bool) {
	return m.shard[m.hashFNV32(key)&m.mask].Get(key)
}

// 函数内不可再调本Map方法和可能存在阻塞风险方法
func (m *ShardMap[K, V]) GetOrNew(key K, newV func() V) (value V) {
	return m.shard[m.hashFNV32(key)&m.mask].GetOrNew(key, newV)
}

func (m *ShardMap[K, V]) GetOrSet(key K, newValue V) (value V) {
	return m.shard[m.hashFNV32(key)&m.mask].GetOrSet(key, newValue)
}

// 函数内不可再调本Map方法和可能存在阻塞风险方法
func (m *ShardMap[K, V]) Delete(key K, f func(value V) bool) bool {
	return m.shard[m.hashFNV32(key)&m.mask].Delete(key, f)
}

func (m *ShardMap[K, V]) Range(f func(key K, value V) bool) bool {
	for _, v := range m.shard {
		if !v.Range(f) {
			return false
		}
	}
	return true
}

// 函数内不可再调本Map方法和可能存在阻塞风险方法
func (m *ShardMap[K, V]) RangeDelete(rangeN int, f func(key K, value V) bool) int {
	if f == nil {
		return 0
	}
	var delN int
	for _, v := range m.shard {
		delN += v.RangeDelete(rangeN, f)
	}
	return delN
}

func (m *ShardMap[K, V]) Lens() (lens []int) {
	lens = make([]int, len(m.shard))
	for i, v := range m.shard {
		lens[i] = v.Len()
	}
	return lens
}

func (m *ShardMap[K, V]) Len() int {
	return basal.Sum(m.Lens()...)
}
