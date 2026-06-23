package basal

import (
	"game/deps/xlog"
	"reflect"

	"github.com/sasha-s/go-deadlock"
)

// x64 架构下的 cache line 原子性，由mesi 协议保证
type NoCacheLineData[T any] struct {
	_ [64]byte // padding to avoid false sharing

	v T

	_  [64]byte // padding to avoid false sharing
	mu deadlock.Mutex
}

func NewNoCacheLineData[T any](v T) *NoCacheLineData[T] {
	switch reflect.ValueOf(v).Kind() {
	case reflect.Bool, reflect.Int, reflect.Int64, reflect.Uint64, reflect.Pointer:
		return &NoCacheLineData[T]{v: v}
	default:
		xlog.Errorf("NoCacheLineData does not support type %s", reflect.TypeOf(v).String())
		panic("NoCacheLineData does not support this type")
		//return nil // This line is unreachable but added to satisfy the compiler
	}
}

//go:norace
func (d *NoCacheLineData[T]) UpdateInt(v T) {
	d.v = v
}

//go:norace
func (d *NoCacheLineData[T]) UpdatePtr(v T) {
	d.v = v
}

//go:norace
func (d *NoCacheLineData[T]) Update(f func(v T) T) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.v = f(d.v)
}

//go:norace
func (d *NoCacheLineData[T]) Do(f func(v T)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	f(d.v)
}

//go:norace
func (d *NoCacheLineData[T]) Get() T {
	return d.v
}
