package misc

import (
	"fmt"
	"game/deps/xlog"
	"strconv"
)

type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~float32 | ~float64
}

type Integer interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

func IntToStr[T Integer](i T) string {
	return fmt.Sprintf("%d", i)
}

func StrToInt(str string) (i int) {
	i, err := strconv.Atoi(str)
	if err != nil {
		xlog.Errorf("str to int failed err:%v", err)
		return 0
	}
	return i
}
func StrToInt64(str string) int64 {
	i, err := strconv.Atoi(str)
	if err != nil {
		xlog.Errorf("str to int failed err:%v", err)
		return 0
	}
	return int64(i)
}
