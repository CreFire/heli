package basal

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSimpleStrHash(t *testing.T) {
	// 测试空字符串
	assert.Equal(t, uint64(14695981039346656037), SimpleStrHash(""))

	// 测试一些已知字符串
	assert.Equal(t, uint64(12638187200555641996), SimpleStrHash("a"))
	assert.Equal(t, uint64(11831194018420276491), SimpleStrHash("hello"))
	assert.Equal(t, uint64(5717881983045765875), SimpleStrHash("world"))
	assert.Equal(t, uint64(11486429397597581241), SimpleStrHash("game-server"))

	// 测试相同字符串产生相同哈希值
	h1 := SimpleStrHash("test-string")
	h2 := SimpleStrHash("test-string")
	assert.Equal(t, h1, h2)

	// 测试不同字符串产生不同哈希值（低概率冲突）
	h3 := SimpleStrHash("string-test")
	assert.NotEqual(t, h1, h3)
}

func BenchmarkSimpleStrHash(b *testing.B) {
	testStr := "benchmark-test-string"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SimpleStrHash(testStr)
	}
}

func BenchmarkSimpleStrHashLong(b *testing.B) {
	testStr := "this-is-a-much-longer-string-to-test-the-performance-of-the-hashing-function-with-larger-inputs"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SimpleStrHash(testStr)
	}
}

func TestIsKTimesPowerOfTwo(t *testing.T) {
	testCases := []struct {
		n, k int
		want bool
	}{
		{1024, 1024, true},  // 1024/1024 = 1, which is 2^0
		{2048, 1024, true},  // 2048/1024 = 2, which is 2^1
		{4096, 1024, true},  // 4096/1024 = 4, which is 2^2
		{3072, 1024, false}, // 3072/1024 = 3, not a power of two
		{100, 10, false},    // 100/10 = 10, not a power of two
		{100, 25, true},     // 100/25 = 4, which is 2^2
		{9, 3, false},       // 9/3 = 3, not a power of two
		{10, 3, false},      // 10 is not divisible by 3
		{0, 10, false},      // m < 1
		{10, 0, false},      // division by zero
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("n=%d,k=%d", tc.n, tc.k), func(t *testing.T) {
			if tc.k == 0 {
				defer func() {
					assert.NotNil(t, recover(), "Expected panic on division by zero")
				}()
			}
			got := IsKTimesPowerOfTwo(tc.n, tc.k)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestSafeRun(t *testing.T) {
	t.Run("NoPanic", func(t *testing.T) {
		executed := false
		SafeRun(func() {
			executed = true
		})
		assert.True(t, executed, "Function inside SafeRun should be executed")
	})

	t.Run("WithPanic", func(t *testing.T) {
		// This test ensures that SafeRun recovers from a panic and doesn't crash the test process.
		// We can't easily assert the log output without mocking xlog.
		assert.NotPanics(t, func() {
			SafeRun(func() {
				panic("test panic")
			})
		}, "SafeRun should recover from panics")
	})
}

func TestSafeGo(t *testing.T) {
	t.Run("NoPanic", func(t *testing.T) {
		var wg sync.WaitGroup
		executed := atomic.Bool{}
		wg.Add(1)
		SafeGo(func() {
			defer wg.Done()
			executed.Store(true)
		})
		wg.Wait()
		assert.True(t, executed.Load(), "Function inside SafeGo should be executed")
	})

	t.Run("WithPanic", func(t *testing.T) {
		// This test ensures that SafeGo recovers from a panic in a goroutine.
		// We just wait a bit to allow the goroutine to run and potentially panic.
		assert.NotPanics(t, func() {
			SafeGo(func() {
				panic("test panic in goroutine")
			})
			// Give the goroutine a moment to execute
			time.Sleep(50 * time.Millisecond)
		})
	})
}

func TestNewNoCacheLineData_Success(t *testing.T) {
	t.Run("Int", func(t *testing.T) {
		v := NewNoCacheLineData(123)
		assert.NotNil(t, v)
		assert.Equal(t, 123, v.Get())
	})

	t.Run("Pointer", func(t *testing.T) {
		p := &struct{}{}
		v := NewNoCacheLineData(p)
		assert.NotNil(t, v)
		assert.Equal(t, p, v.Get())
	})
}

func TestNewNoCacheLineData_Panic(t *testing.T) {
	// We can't easily mock xlog here, so we just check for the panic.
	defer func() {
		r := recover()
		require.NotNil(t, r, "Expected NewNoCacheLineData to panic for unsupported type")
		assert.Equal(t, "NoCacheLineData does not support this type", r)
	}()

	// This should panic because string is not in the supported list.
	NewNoCacheLineData("unsupported string")
}

func TestNoCacheLineData_UpdateMethods(t *testing.T) {
	t.Run("UpdateInt", func(t *testing.T) {
		d := NewNoCacheLineData(100)
		d.UpdateInt(200)
		assert.Equal(t, 200, d.Get(), "UpdateInt should update the value")
	})

	t.Run("UpdatePtr", func(t *testing.T) {
		p1 := &struct{}{}
		p2 := &struct{}{}
		d := NewNoCacheLineData(p1)
		d.UpdatePtr(p2)
		assert.Equal(t, p2, d.Get(), "UpdatePtr should update the value")
	})

	t.Run("Update", func(t *testing.T) {
		d := NewNoCacheLineData(100)
		d.Update(func(v int) int {
			return v + 50
		})
		assert.Equal(t, 150, d.Get(), "Update should apply the function to the value")
	})
}

type ISYE struct {
	v int
	X int
}

func TestNoCacheLineData_Update_Concurrency(t *testing.T) {
	Y := &ISYE{0, 0}
	d := NewNoCacheLineData(Y)
	var wg sync.WaitGroup
	numGoroutines := 1000
	incrementsPerGoroutine := 1003

	wg.Add(numGoroutines)
	for range numGoroutines {
		go func() {
			defer wg.Done()
			for j := range incrementsPerGoroutine {
				x := d.Get()

				assert.Equal(t, x.X, x.v)
				assert.Less(t, x.X, 100)

				XC := j % 100
				bd := new(ISYE)
				bd.X = XC
				bd.v = XC
				d.UpdatePtr(bd)
			}
		}()
	}
	wg.Wait()

	x := d.Get()

	assert.Equal(t, x.X, x.v)
	assert.Less(t, x.X, 100)
	fmt.Printf("x.X: %v\n", x)

	//expected := numGoroutines * incrementsPerGoroutine
	//assert.Equal(t, expected, d.Get(), "Update method should be thread-safe and produce the correct final count")
}
