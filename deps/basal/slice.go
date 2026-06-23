package basal

import "slices"

import "math/rand"

// 重复元素的切片
func Repeat[T any](v T, count int) []T {
	data := make([]T, count)
	for i := range count {
		data[i] = v
	}
	return data
}

// map value转切片
func MapToSliceByValue[K comparable, V any](data map[K]V) []V {
	if len(data) == 0 {
		return nil
	}
	var res = make([]V, 0, len(data))
	for _, v := range data {
		res = append(res, v)
	}
	return res
}

// map key转切片
func MapToSliceByKey[K comparable, V any](data map[K]V) []K {
	if len(data) == 0 {
		return nil
	}
	var res = make([]K, 0, len(data))
	for k := range data {
		res = append(res, k)
	}
	return res
}

// 根据切片直接获取下标的值,超出范围返回类型默认值
func GetSliceValue[T any](sli []T, index int) (v T) {
	if index < len(sli) {
		return sli[index]
	}
	return
}

// 切片中查找对应数据,找不到返回index返回-1
func FindSliceValue[T any](sli []T, f func(v T) bool) (value T, index int) {
	index = -1
	for i, v := range sli {
		if f(v) {
			return v, i
		}
	}
	return
}

// 乱序
func Shuffle[T any](sli []T) {
	if len(sli) < 2 {
		return
	}
	for i := int64(len(sli)) - 1; i > 0; i-- {
		j := rand.Int63n(i + 1)
		sli[i], sli[j] = sli[j], sli[i]
	}
}

// 求和
func Sum[T Number](nums ...T) T {
	var total T
	for _, v := range nums {
		total += v
	}
	return total
}

// 求和任意结构
func SumAny[N Number, T any](f func(v T) N, nums ...T) N {
	var total N
	for _, v := range nums {
		total += f(v)
	}
	return total
}

// 返序
func Reverse[T any](sli []T) {
	dLen := len(sli)
	var temp T
	for i := 0; i < dLen/2; i++ {
		temp = sli[i]
		sli[i] = sli[dLen-1-i]
		sli[dLen-1-i] = temp
	}
}

// 轮转
func Rotate[T any](src []T, rotate int) (add int) {
	dLen := len(src)
	if dLen < 2 {
		return 0
	}
	if rotate == 0 {
		return 0
	} else if rotate > 0 {
		rotate = rotate % dLen
	} else {
		rotate = dLen - ((-rotate) % dLen)
	}
	dst := make([]T, rotate)
	copy(dst, src)
	for i := range dLen {
		if index := (i + rotate) % dLen; index < rotate {
			src[i] = dst[index]
		} else {
			src[i] = src[index]
		}
	}
	return rotate
}

// 在之中
func InNumber[T Number](n T, nums ...T) bool {
	return slices.Contains(nums, n)
}

// 切片按内容转换
func SliceTo[S, D any](src []S, f func(s S) D) []D {
	dst := make([]D, 0, len(src))
	for _, v := range src {
		dst = append(dst, f(v))
	}
	return dst
}
