package fastid_test

import (
	"game/deps/fastid"
	"testing"
	"time"
)

func TestConfig_GetTimeMillFromFastID(t *testing.T) {
	fastid.InitWithMachineID(111)
	before := time.Now().UnixMilli()
	id := fastid.GenInt64ID()
	after := time.Now().UnixMilli()
	got := fastid.CommonConfig.GetTimeMillFromFastID(id)
	// The ID timestamp is stored in 2^20ns buckets and decoded back with millisecond truncation,
	// so the decoded wall time can be up to 1ms earlier than the caller's sampled clock.
	if got < before-1 || got > after {
		t.Fatalf("decoded millis out of range: got=%d before=%d after=%d", got, before, after)
	}
}

func BenchmarkGetTimeMillFromFastID(b *testing.B) {
	fastid.InitWithMachineID(111)
	id := fastid.GenInt64ID()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			id += 1
			_ = fastid.GetTimeMillFromFastID(id)
		}
	})
}
