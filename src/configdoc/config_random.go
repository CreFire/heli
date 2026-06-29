package configdoc

import (
	"fmt"
	"sync"
)

type TWeight[T any] struct {
	Weight int32
	Item   T
}

func NewTWeight[T comparable](weight int32, item T) *TWeight[T] {
	return &TWeight[T]{Weight: weight, Item: item}
}

type TWeightSelector[T comparable] struct {
	kind   randomKind
	static xWeight[T]
	pool   *sync.Pool
}

func MapToWeightList[K comparable](m map[K]int32) []*TWeight[K] {
	var result []*TWeight[K]
	for itemId, weight := range m {
		result = append(result, NewTWeight(weight, itemId))
	}
	return result
}

func NewWeightSelector[T comparable](list []*TWeight[T], kind randomKind) (*TWeightSelector[T], error) {

	if len(list) == 0 {
		return nil, fmt.Errorf("new weight selector err. empty list")
	}

	for _, v := range list {
		if v == nil {
			return nil, fmt.Errorf("new weight selector err. v is nil list:%v", list)
		}
		if v.Weight <= 0 {
			return nil, fmt.Errorf("new weight selector err . weight err. list:%v", list)
		}
	}
	static := newXWeight(list)
	return &TWeightSelector[T]{
		kind:   kind,
		static: static,
		pool:   &sync.Pool{New: func() any { return make([]*TWeight[T], len(static.ws), len(static.ws)) }},
	}, nil
}

func (s *TWeightSelector[T]) copyWeights() []*TWeight[T] {
	weights := make([]*TWeight[T], len(s.static.ws), len(s.static.ws))
	copy(weights, s.static.ws)
	return weights
}

func (s *TWeightSelector[T]) getWeightsFromPool() []*TWeight[T] {
	weights, _ := s.pool.Get().([]*TWeight[T])
	if weights == nil || cap(weights) < len(s.static.ws) {
		return s.copyWeights()
	}

	weights = weights[:len(s.static.ws)]
	copy(weights, s.static.ws)
	return weights
}

func (s *TWeightSelector[T]) putWeightsToPool(weights []*TWeight[T]) {
	if weights == nil || cap(weights) < len(s.static.ws) {
		return
	}
	weights = weights[:0]
	s.pool.Put(weights)
}

func (s *TWeightSelector[T]) RandWeightOne() T {
	if out := s.RandWeight(); len(out) > 0 {
		return out[0]
	}
	var zero T
	return zero
}

func (s *TWeightSelector[T]) RandWeight() []T {
	if len(s.static.ws) == 0 {
		return nil
	}

	switch s.kind {
	case randomKindMerge:
		item, _, ok := randMergeOneStatic(s.static, true)
		if !ok {
			return nil
		}
		return []T{item}
	case randomKindProb:
		return randProb(s.static.ws)
	default:
		panic("invalid random kind")
	}
}

func (s *TWeightSelector[T]) RandomWeightN(num int32, repeat bool) []T {
	if num <= 0 {
		return nil
	}
	ret := make([]T, 0, num)
	s.RandomWeightNForEach(num, repeat, func(item T) {
		ret = append(ret, item)
	})
	return ret
}

func (s *TWeightSelector[T]) RandomWeightNForEach(num int32, repeat bool, yield func(T)) {
	if num <= 0 || yield == nil {
		return
	}
	switch s.kind {
	case randomKindMerge:
		randMergeNForEach(s.static, num, repeat, s.getWeightsFromPool, s.putWeightsToPool, yield)
		return
	case randomKindProb:
		for i := 0; i < int(num); i++ {
			randProbForEach(s.static.ws, yield)
		}
		return
	default:
		panic("invalid random kind")
	}
}

func (s *TWeightSelector[T]) RandWeightNByFilter(num int32, repeat bool, existingCount int32, filter func(T) bool) []T {
	if num <= 0 {
		return nil
	}
	if filter == nil {
		return s.RandomWeightN(num, repeat)
	}

	switch s.kind {
	case randomKindMerge:
		if s.shouldUseFilterFast(num, repeat, existingCount) {
			ret, ok := s.randWeightNByFilterFast(num, repeat, filter)
			if ok {
				return ret
			}
			remain := num - int32(len(ret))
			return append(ret, s.randWeightNByFilterSlow(remain, repeat, filter, ret)...)
		}
		return s.randWeightNByFilterSlow(num, repeat, filter, nil)
	case randomKindProb:
		return s.randWeightNByFilterSlow(num, repeat, filter, nil)
	default:
		panic("invalid random kind")
	}
}

func (s *TWeightSelector[T]) shouldUseFilterFast(num int32, repeat bool, existingCount int32) bool {
	if repeat {
		return true
	}

	total := int32(len(s.static.ws))
	if (existingCount+num)*5 >= 3*total {
		return false
	}

	return true
}

func (s *TWeightSelector[T]) randWeightNByFilterFast(num int32, repeat bool, filter func(T) bool) ([]T, bool) {
	budget := int(num) * 3

	ret := make([]T, 0, num)
	for i := 0; i < budget && int32(len(ret)) < num; i++ {
		item, _, ok := randMergeOneStatic(s.static, true)
		if !ok {
			return ret, false
		}
		if filter(item) {
			continue
		}
		if !repeat && weightFilterContains(ret, item) {
			continue
		}
		ret = append(ret, item)
	}
	return ret, int32(len(ret)) == num
}

func (s *TWeightSelector[T]) randWeightNByFilterSlow(num int32, repeat bool, filter func(T) bool, exclude []T) []T {
	if num <= 0 {
		return nil
	}

	weights := make([]*TWeight[T], 0, len(s.static.ws))
	for _, v := range s.static.ws {
		if filter != nil && filter(v.Item) {
			continue
		}
		if !repeat && weightFilterContains(exclude, v.Item) {
			continue
		}
		weights = append(weights, v)
	}
	if len(weights) == 0 {
		return nil
	}

	var ret []T
	switch s.kind {
	case randomKindMerge:
		state := newXWeight(weights)
		if repeat || num == 1 {
			randMergeNForEachStatic(state, num, repeat, func(item T) {
				ret = append(ret, item)
			})
			return ret
		}
		randMergeNForEachWorkState(state, num, func() []*TWeight[T] {
			return weights
		}, func([]*TWeight[T]) {}, func(item T) {
			ret = append(ret, item)
		})
		return ret
	case randomKindProb:
		for i := 0; i < int(num); i++ {
			randProbForEach(weights, func(item T) {
				ret = append(ret, item)
			})
		}
		return ret
	default:
		panic("invalid random kind")
	}
}

func weightFilterContains[T comparable](items []T, target T) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
