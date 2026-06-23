package basal

import (
	"math"
	"testing"
)

// multiplyHash mirrors the old SimpleHash implementation (single multiply)
// to highlight its behavior when low bits stay constant and bucket count is a power of two.
func multiplyHash(x uint64) uint64 {
	return x * 0x9e3779b97f4a7c15
}

// splitMix64 is the stronger mixer we want to benchmark/compare against.
func splitMix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	x ^= x >> 31
	return x
}

func TestHashDistributionWithFixedLowBits(t *testing.T) {
	// Mimic fastid layout: [40 bits time][10 bits seq][13 bits machine].
	const (
		bucketCount = 128 // power-of-two buckets, typical for mod
		samples     = 200_000
		seqBits     = 10
		seqMask     = 1<<seqBits - 1
		machineID   = uint64(0x1A3) // fixed machine id (13 bits)
		shiftSeq    = 13
		shiftTime   = shiftSeq + seqBits
	)

	buildBuckets := func(hash func(uint64) uint64) []int {
		buckets := make([]int, bucketCount)
		var ts uint64 = 1000 // arbitrary start timestamp
		for i := range samples {
			seq := uint64(i) & seqMask
			if seq == 0 && i != 0 {
				ts++ // simulate time tick when seq wraps
			}
			id := (ts << shiftTime) | (seq << shiftSeq) | machineID
			buckets[hash(id)%bucketCount]++
		}
		return buckets
	}

	baseBuckets := buildBuckets(multiplyHash)
	mixedBuckets := buildBuckets(splitMix64)

	baseSpread := bucketStats(baseBuckets)
	mixedSpread := bucketStats(mixedBuckets)

	t.Logf("multiplyHash: unique=%d/%d min=%d max=%d std=%.2f",
		baseSpread.unique, bucketCount, baseSpread.min, baseSpread.max, baseSpread.stddev)
	t.Logf("splitMix64:   unique=%d/%d min=%d max=%d std=%.2f",
		mixedSpread.unique, bucketCount, mixedSpread.min, mixedSpread.max, mixedSpread.stddev)

	// Baseline shows the collision issue: everything falls into one bucket when low bits are fixed.
	if baseSpread.unique != 1 {
		t.Fatalf("expected multiply-only hash to collapse into 1 bucket, got %d", baseSpread.unique)
	}

	// SplitMix64 should spread broadly and keep variance reasonable.
	if mixedSpread.unique < bucketCount*3/4 {
		t.Fatalf("splitMix64 should cover most buckets: got %d/%d", mixedSpread.unique, bucketCount)
	}
	avg := float64(samples) / bucketCount
	if mixedSpread.stddev > avg*0.2 {
		t.Fatalf("splitMix64 bucket stddev too high: got %.2f (avg %.2f)", mixedSpread.stddev, avg)
	}
}

type bucketSpread struct {
	unique int
	min    int
	max    int
	stddev float64
}

func bucketStats(buckets []int) bucketSpread {
	const minSentinel = int(math.MaxInt64)
	var (
		unique int
		min    = minSentinel
		max    int
		sum    int
	)
	for _, c := range buckets {
		if c > 0 {
			unique++
			if c < min {
				min = c
			}
			if c > max {
				max = c
			}
		} else if min == minSentinel {
			min = 0
		}
		sum += c
	}

	avg := float64(sum) / float64(len(buckets))
	var variance float64
	for _, c := range buckets {
		delta := float64(c) - avg
		variance += delta * delta
	}
	variance /= float64(len(buckets))

	return bucketSpread{
		unique: unique,
		min:    min,
		max:    max,
		stddev: math.Sqrt(variance),
	}
}

func BenchmarkHashMultiply(b *testing.B) {
	var x uint64
	for i := 0; i < b.N; i++ {
		x = multiplyHash(x + 1)
	}
	_ = x
}

func BenchmarkHashSplitMix64(b *testing.B) {
	var x uint64
	for i := 0; i < b.N; i++ {
		x = splitMix64(x + 1)
	}
	_ = x
}

func BenchmarkSimpleHash(b *testing.B) {
	var x uint64
	for i := 0; i < b.N; i++ {
		x = SimpleHash(x + 1)
	}
	_ = x
}

func BenchmarkSimpleStrHash8Bytes(b *testing.B) {
	key := "abcdefgh"
	for i := 0; i < b.N; i++ {
		SimpleStrHash(key)
	}
}
