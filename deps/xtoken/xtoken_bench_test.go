package xtoken

import "testing"

func BenchmarkSimpleTokenDecode(b *testing.B) {
	token, err := UserTokenEncode(123456, "machine-abcdefg-xyz")
	if err != nil {
		b.Fatalf("encode token: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := UserTokenDecode(token, "machine-abcdefg-xyz"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSimpleTokenEncode(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := UserTokenEncode(123456, "machine-abcdefg-xyz"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkInternalTokenDecode(b *testing.B) {
	token, err := ServerTokenEncode("logic", "battle")
	if err != nil {
		b.Fatalf("encode token: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := ServerTokenDecode(token, "battle"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkInternalTokenEncode(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ServerTokenEncode("logic", "battle"); err != nil {
			b.Fatal(err)
		}
	}
}
