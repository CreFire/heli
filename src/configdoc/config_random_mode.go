package configdoc

import (
	randv2 "math/rand/v2"
	"sort"
)

var randomInt32N = randv2.Int32N

// 权重类型定义
const (
	MAX_WEIGHT = 10000 // 默认最大权重
)

type randomKind uint8

const (
	randomKindInvalid randomKind = iota
	randomKindProb
	randomKindMerge
)

func randProb[T comparable](weights []*TWeight[T]) []T {
	var ret []T
	randProbForEach(weights, func(item T) {
		ret = append(ret, item)
	})
	return ret
}

func randProbForEach[T comparable](weights []*TWeight[T], yield func(T)) {
	for _, v := range weights {
		if randomInt32N(MAX_WEIGHT) < v.Weight {
			yield(v.Item)
		}
	}
}

func randMergeNForEach[T comparable](staticWeights xWeight[T], num int32, repeat bool, getWeights func() []*TWeight[T], putWeights func([]*TWeight[T]), yield func(T)) {
	if num <= 0 || yield == nil {
		return
	}

	if repeat || num == 1 {
		randMergeNForEachStatic(staticWeights, num, repeat, yield)
		return
	}

	randMergeNForEachWorkState(staticWeights, num, getWeights, putWeights, yield)
}

func randMergeNForEachStatic[T comparable](staticWeights xWeight[T], num int32, repeat bool, yield func(T)) {
	for i := 0; i < int(num); i++ {
		item, _, ok := randMergeOneStatic(staticWeights, repeat)
		if ok {
			yield(item)
		}
	}
}

func randMergeNForEachWorkState[T comparable](staticWeights xWeight[T], num int32, getWeights func() []*TWeight[T], putWeights func([]*TWeight[T]), yield func(T)) {
	workWeights := getWeights()
	defer putWeights(workWeights)
	workState := xWeight[T]{
		total: staticWeights.total,
		ws:    workWeights,
	}
	for i := 0; i < int(num); i++ {
		item, newState, ok := randMergeOneWorkState(workState)
		workState = newState
		if ok {
			yield(item)
		}
		if !ok {
			return
		}
	}
}

type xWeight[T comparable] struct {
	total  int32
	ws     []*TWeight[T]
	prefix []int32
}

func newXWeight[T comparable](weights []*TWeight[T]) xWeight[T] {
	prefix := make([]int32, 0, len(weights))
	var total int32
	for _, v := range weights {
		total += v.Weight
		prefix = append(prefix, total)
	}
	return xWeight[T]{
		total:  total,
		ws:     weights,
		prefix: prefix,
	}
}

func randMergeOneStatic[T comparable](weights xWeight[T], repeat bool) (T, xWeight[T], bool) {
	if weights.total <= 0 || len(weights.ws) == 0 {
		var zero T
		return zero, weights, false
	}

	out, _, ok := randMergeOneStaticByTotal(weights, randomInt32N(weights.total))
	if !ok {
		var zero T
		return zero, weights, false
	}
	if repeat {
		return out, weights, true
	}
	return out, weights, true
}

func randMergeOneWorkState[T comparable](weights xWeight[T]) (T, xWeight[T], bool) {
	if weights.total <= 0 || len(weights.ws) == 0 {
		var zero T
		return zero, weights, false
	}

	out, idx, ok := randMergeOneWorkStateByTotal(weights, randomInt32N(weights.total))
	if !ok {
		var zero T
		return zero, weights, false
	}
	pickedWeight := weights.ws[idx].Weight
	last := len(weights.ws) - 1
	weights.ws[idx], weights.ws[last] = weights.ws[last], weights.ws[idx]
	weights.ws = weights.ws[:last]
	weights.total -= pickedWeight
	return out, weights, true
}

func calcWeightTotal[T comparable](weights []*TWeight[T]) int32 {
	var total int32
	for _, v := range weights {
		total += v.Weight
	}
	return total
}

func randMergeOneStaticByTotal[T comparable](weights xWeight[T], n int32) (T, int, bool) {
	var zero T
	if weights.total <= 0 || len(weights.ws) == 0 {
		return zero, -1, false
	}
	idx := sort.Search(len(weights.prefix), func(i int) bool {
		return weights.prefix[i] > n
	})
	if idx >= 0 && idx < len(weights.ws) {
		return weights.ws[idx].Item, idx, true
	}
	return zero, -1, false
}

func randMergeOneWorkStateByTotal[T comparable](weights xWeight[T], n int32) (T, int, bool) {
	var zero T
	if weights.total <= 0 || len(weights.ws) == 0 {
		return zero, -1, false
	}

	var tempP int32
	for i, v := range weights.ws {
		tempP += v.Weight
		if tempP > n {
			return v.Item, i, true
		}
	}
	return zero, -1, false
}
