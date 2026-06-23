package basal

import (
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"
)

// funcNameInfo stores both full and short names of a function.
type funcNameInfo struct {
	fullName  string
	shortName string
}

var (
	// funcNameCache 用于缓存函数名以提高性能。
	// It stores a *funcNameInfo for a given function PC.
	funcNameCache sync.Map // map[uintptr]*funcNameInfo
)

// getFuncNameInfo retrieves function name information from cache or through runtime lookup.
// It's an internal helper to be used by GetFuncFullName and GetFuncShortName.
func getFuncNameInfo(i any) *funcNameInfo {
	if i == nil {
		return nil
	}
	v := reflect.ValueOf(i)
	if v.Kind() != reflect.Func {
		return nil
	}

	pc := v.Pointer()
	if info, ok := funcNameCache.Load(pc); ok {
		return info.(*funcNameInfo)
	}

	// Look up the function name.
	fullName := runtime.FuncForPC(pc).Name()
	// The runtime adds a -fm suffix for method values (and expressions).
	// We trim this to get the canonical name.
	fullName = strings.TrimSuffix(fullName, "-fm")
	if fullName == "" {
		// This can happen for functions without symbols.
		// Cache the empty result to avoid repeated lookups.
		info := &funcNameInfo{fullName: "", shortName: ""}
		funcNameCache.Store(pc, info)
		return info
	}

	// Derive the short name.
	shortName := fullName
	if lastDot := strings.LastIndex(fullName, "."); lastDot >= 0 {
		shortName = fullName[lastDot+1:]
	}

	// Store the new info in the cache.
	info := &funcNameInfo{
		fullName:  fullName,
		shortName: shortName,
	}
	funcNameCache.Store(pc, info)
	return info
}

func GetFuncName(i any, seps ...rune) string {
	// 获取函数名称
	fn := GetFuncFullName(i)

	// If the only separator is '.', we can use the cached short name.
	if len(seps) == 1 && seps[0] == '.' {
		return GetFuncShortName(i)
	}

	if len(seps) == 0 {
		return fn
	}

	isSeparator := func(r rune) bool {
		return slices.Contains(seps, r)
	}

	trimmedFn := strings.TrimRightFunc(fn, isSeparator)
	if lastSepIndex := strings.LastIndexFunc(trimmedFn, isSeparator); lastSepIndex != -1 {
		_, size := utf8.DecodeRuneInString(trimmedFn[lastSepIndex:])
		return trimmedFn[lastSepIndex+size:]
	}
	return trimmedFn
}

// GetFuncFullName 获取函数的完整名称。
// 它使用缓存来避免重复的、开销大的运行时查找。
func GetFuncFullName(i any) string {
	info := getFuncNameInfo(i)
	if info == nil {
		if i == nil {
			return "nil"
		}
		return "not a function"
	}
	return info.fullName
}

func GetFuncShortName(i any) string {
	info := getFuncNameInfo(i)
	if info == nil {
		if i == nil {
			return "nil"
		}
		return "not a function"
	}
	return info.shortName
}
