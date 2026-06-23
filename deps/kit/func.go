package kit

import (
	"encoding/json"
	"game/deps/basal"
	"game/deps/xlog"
	"runtime/debug"
	"strconv"

	"google.golang.org/protobuf/proto"
)

func Atoi(str string) int {
	i, err := strconv.Atoi(str)
	if err != nil {
		return 0
	}
	return i
}

func Atoi32(str string) int32 {
	return int32(Atoi(str))
}

func Atoi64(str string) int64 {
	i, err := strconv.ParseInt(str, 10, 64)
	if err != nil {

		xlog.Errorf("str to int64 failed err:%v", string(debug.Stack()))
		return 0
	}
	return i
}

func UAtoi64(str string) uint64 {
	i, err := strconv.ParseUint(str, 10, 64)
	if err != nil {
		xlog.Errorf("str to int64 failed err:%v", string(debug.Stack()))
		return 0
	}
	return i
}

func Atof(str string) float32 {
	i, err := strconv.ParseFloat(str, 32)
	if err != nil {
		xlog.Errorf("str to int64 failed err:%v", err)
		return 0
	}
	return float32(i)
}

func Atof64(str string) float64 {
	i, err := strconv.ParseFloat(str, 64)
	if err != nil {
		xlog.Errorf("str to int64 failed err:%v", err)
		return 0
	}
	return i
}

func Itoa(num any) string {
	switch n := num.(type) {
	case int:
		return strconv.Itoa(n)
	case int8:
		return strconv.FormatInt(int64(n), 10)
	case int16:
		return strconv.FormatInt(int64(n), 10)
	case int32:
		return strconv.FormatInt(int64(n), 10)
	case int64:
		return strconv.FormatInt(n, 10)
	case uint:
		return strconv.FormatUint(uint64(n), 10)
	case uint8:
		return strconv.FormatUint(uint64(n), 10)
	case uint16:
		return strconv.FormatUint(uint64(n), 10)
	case uint32:
		return strconv.FormatUint(uint64(n), 10)
	case uint64:
		return strconv.FormatUint(n, 10)
	case string:
		return n
	case json.Number:
		return n.String()
	default:
		return ""
	}
}

// Try 用此函数必然会打印堆栈信息以及错误信息,并且会加入统计
func Try(fun func(), handler func(stack string, err error)) {
	defer basal.ExceptionAndSend(func(err error) {
		stack := basal.Stack(err)
		xlog.Errorf(stack)
		//Statistics.AddPanic(stack)
		if handler == nil {
			return
		}
		handler(stack, err)
	})
	fun()
}

// 此函数捕捉异常并且加入统计
func Exception(catch func(err error)) {
	if e := recover(); e != nil {
		err := basal.ToError(e)
		xlog.Errorf(basal.Stack(basal.ToError(e)))
		if catch == nil {
			return
		}
		catch(err)
	}
}

func PbData(v proto.Message) []byte {
	data, err := proto.Marshal(v)
	if err != nil {
		xlog.Warnf("marshal proto failed err:%v", err)
		return nil
	}
	return data
}
