package configdoc

import (
	"fmt"
	"game/deps/basal"
	"unsafe"
)

type extendCheckHandlers struct {
	has      map[int]struct{}
	handlers []func(*DocPbConfig, *DocExtendConfig) bool
}

func (m *extendCheckHandlers) SetHandler(f func(*DocPbConfig, *DocExtendConfig) bool) {
	ptr := *(*int)(unsafe.Pointer(&f))
	if _, ok := m.has[ptr]; ok {
		panic(fmt.Sprintf("extendCheckHandlers error repeat: %v", basal.GetFuncName(f)))
	}
	m.has[ptr] = struct{}{}
	m.handlers = append(m.handlers, f)
}

func newExtendCheckHandlers() *extendCheckHandlers {
	return &extendCheckHandlers{has: map[int]struct{}{}}
}
