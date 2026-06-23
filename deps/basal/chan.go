package basal

import (
	"sync/atomic"
	"time"
)

var ChanPushTimeout = NewError("chan push timeout")
var ChanPushFull = NewError("chan push full")

type Chan[T any] struct {
	ch     chan T
	C      <-chan T
	closed int32
}

func NewChan[T any](size int) *Chan[T] {
	ch := make(chan T, size)
	return &Chan[T]{ch: ch, C: ch}
}

func (m *Chan[T]) Close() {
	if atomic.CompareAndSwapInt32(&m.closed, 0, 1) {
		close(m.ch)
	}
}

func (m *Chan[T]) Len() int {
	return len(m.ch)
}

// timeout: 0 一直等待,除非错误返回, >0 超时返回或错误返回, -1, push失败立即返回
func (m *Chan[T]) Push(v T, timeout time.Duration) (err error) {
	defer Exception(func(e error) {
		err = e
	})
	if timeout == 0 {
		m.ch <- v
		return nil
	} else if timeout > 0 {
		ticker := time.NewTimer(timeout)
		defer ticker.Stop()
		select {
		case m.ch <- v:
			return nil
		case <-ticker.C:
			return ChanPushTimeout
		}
	} else {
		select {
		case m.ch <- v:
			return nil
		default:
			return ChanPushFull
		}
	}
}
