package basal

import (
	"errors"
	"fmt"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
)

func Sprintf(format string, a ...any) string {
	return fmt.Sprintf(format, a...)
}

func NewError(format string, a ...any) error {
	return fmt.Errorf(format, a...)
}

func ToError(e any) (err error) {
	switch v := e.(type) {
	case string:
		return errors.New(v)
	case error:
		return errors.New(v.Error())
	default:
		return fmt.Errorf("unknown error: %v", v)
	}
}

// 调用信息短文件名
func CallerShort(skip int) (file string, line int) {
	var ok bool
	_, file, line, ok = runtime.Caller(skip)
	if ok {
		short := file
		for i := len(file) - 1; i > 0; i-- {
			if file[i] == '/' {
				short = file[i+1:]
				break
			}
		}
		file = short
	} else {
		file = "???"
		line = 0
	}
	return
}

// 调用信息长文件名
func Caller(skip int) (file string, line int) {
	var ok bool
	_, file, line, ok = runtime.Caller(skip)
	if !ok {
		file = "???"
		line = 0
	}
	return
}

// 调用者信息 函数名
func CallerInFunc(skip int) (name string, file string, line int) {
	var pc uintptr
	var ok bool
	pc, file, line, ok = runtime.Caller(skip)
	if ok {
		inFunc := runtime.FuncForPC(pc)
		name = inFunc.Name()
	} else {
		file = "???"
		name = "???"
	}
	return
}

func StackLine(skip int) string {
	var name string
	pc, file, line, ok := runtime.Caller(skip)
	if ok {
		inFunc := runtime.FuncForPC(pc)
		name = inFunc.Name()
	} else {
		file = "???"
		name = "???"
	}
	var builder strings.Builder
	builder.Grow(256)
	builder.WriteString(name)
	builder.WriteByte('(')
	builder.WriteString(file)
	builder.WriteByte(':')
	builder.WriteString(strconv.Itoa(line))
	builder.WriteByte(')')
	return builder.String()
}

func PanicStackSimple(err error) string {
	pcs := make([]uintptr, 16)
	n := runtime.Callers(2, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	var builder strings.Builder
	builder.Grow(1024)
	builder.WriteString("exception: ")
	if err == nil {
		builder.WriteString("nil")
	} else {
		builder.WriteString(err.Error())
	}
	builder.WriteByte('\n')
	var found = false
	for {
		frame, more := frames.Next()
		if !found && frame.Func.Name() == "runtime.gopanic" {
			found = true
			builder.WriteString(frame.Function)
			builder.WriteString("")
			builder.WriteByte('\n')
			builder.WriteByte('\t')
			builder.WriteString(frame.File)
			builder.WriteByte(':')
			builder.WriteString(strconv.Itoa(frame.Line))
			builder.WriteByte('\n')
		} else if found {
			builder.WriteString(frame.Function)
			builder.WriteByte('\n')
			builder.WriteByte('\t')
			builder.WriteString(frame.File)
			builder.WriteByte(':')
			builder.WriteString(strconv.Itoa(frame.Line))
			builder.WriteByte('\n')
			break
		}
		if !more {
			break
		}
	}
	return builder.String()
}

func PanicStack(err error) string {
	pcs := make([]uintptr, 16)
	n := runtime.Callers(2, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	var builder strings.Builder
	builder.Grow(1024)
	builder.WriteString("exception: ")
	if err == nil {
		builder.WriteString("nil")
	} else {
		builder.WriteString(err.Error())
	}
	builder.WriteByte('\n')
	var found = false
	for {
		frame, more := frames.Next()
		if !found && frame.Func.Name() == "runtime.gopanic" {
			found = true
		}
		if found {
			builder.WriteString(frame.Function)
			builder.WriteByte('\n')
			builder.WriteByte('\t')
			builder.WriteString(frame.File)
			builder.WriteByte(':')
			builder.WriteString(strconv.Itoa(frame.Line))
			builder.WriteByte('\n')
		}
		if !more {
			break
		}
	}
	return builder.String()
}

func StackFast(err error) string {
	pcs := make([]uintptr, 16)
	n := runtime.Callers(2, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	var builder strings.Builder
	builder.Grow(1024)
	builder.WriteString("exception: ")
	if err == nil {
		builder.WriteString("nil")
	} else {
		builder.WriteString(err.Error())
	}
	builder.WriteByte('\n')
	for {
		frame, more := frames.Next()
		builder.WriteString(frame.Function)
		builder.WriteByte('\n')
		builder.WriteByte('\t')
		builder.WriteString(frame.File)
		builder.WriteByte(':')
		builder.WriteString(strconv.Itoa(frame.Line))
		builder.WriteByte('\n')
		if !more {
			break
		}
	}
	return builder.String()
}

func Stack(err error) string {
	var builder strings.Builder
	builder.Grow(1024)
	builder.WriteString("exception: ")
	if err == nil {
		builder.WriteString("nil")
	} else {
		builder.WriteString(err.Error())
	}
	builder.WriteByte('\n')
	builder.Write(debug.Stack())
	return builder.String()
}

func Exception(catch func(err error)) {
	if e := recover(); e != nil {
		if catch == nil {
			return
		}
		err := ToError(e)
		catch(err)
	}
}

// ExceptionAndSend 捕获异常并且发送到企业微信
func ExceptionAndSend(catch func(err error)) {
	if e := recover(); e != nil {
		err := ToError(e)
		if catch == nil {
			return
		}
		catch(err)
	}
}

func Try(f func(), catch func(err error)) {
	defer ExceptionAndSend(catch)
	f()
}
