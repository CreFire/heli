package basal

import (
	"bytes"
	"math/rand"
	"testing"
	"time"
)

// go test -run=none -benchmem -bench .
var lorem []byte
var loremLZ4 []byte
var loremSnappy []byte

func init() {
	rand.Seed(time.Now().Unix())
	for len(lorem) < 1024 {
		lorem = append(lorem, []byte("kashdahdakscnlsandkajhskashdkasdnkasnxaskndkas")...)
	}
	loremLZ4, _ = LZ4Compress(lorem)
	loremSnappy = SnappyCompress(lorem)
}

func BenchmarkLZ4Compress(b *testing.B) {
	for i := 0; i < b.N; i++ {
		LZ4Compress(lorem)
	}
}

func BenchmarkLZ4Decompress(b *testing.B) {
	for i := 0; i < b.N; i++ {
		LZ4Decompress(loremLZ4)
	}
}

func TestLZ4RoundTripHighlyCompressibleData(t *testing.T) {
	src := bytes.Repeat([]byte{7}, 1<<20)

	compressed, err := LZ4Compress(src)
	if err != nil {
		t.Fatalf("compress failed: %v", err)
	}

	got, err := LZ4Decompress(compressed)
	if err != nil {
		t.Fatalf("decompress failed: %v", err)
	}
	if !bytes.Equal(got, src) {
		t.Fatal("round trip mismatch")
	}
}

func BenchmarkSnappyCompress(b *testing.B) {
	for i := 0; i < b.N; i++ {
		SnappyCompress(lorem)
	}
}

func BenchmarkSnappyDecompress(b *testing.B) {
	for i := 0; i < b.N; i++ {
		SnappyDecompress(loremSnappy)
	}
}
