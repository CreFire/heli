package basal

import (
	"runtime"
	"strings"
	"sync"
	"testing"
)

// --- Test Helpers ---

func topLevelFuncForTest() {}

type testStructForFunc struct{}

func (ts testStructForFunc) ValueReceiverMethod() {}

func (ts *testStructForFunc) PtrReceiverMethod() {}

var (
	// pkg-level anonymous function
	anonFuncForTest = func() {}
)

// --- Unit Tests ---

func TestGetFuncNames(t *testing.T) {
	// Clear cache before running tests to ensure a clean state.
	funcNameCache = sync.Map{}

	ts := testStructForFunc{}
	pts := &ts

	// The full name of a function includes the package path.
	// We get the current package path to build the expected full names dynamically.
	pc, _, _, _ := runtime.Caller(0)
	myFullName := runtime.FuncForPC(pc).Name()
	pkgPath := myFullName[:strings.LastIndex(myFullName, ".")]

	// For anonymous functions, names are not stable across compilers/versions.
	// We will check for a reasonable prefix.
	inlineAnonFullNamePrefix := pkgPath + ".TestGetFuncNames.func"
	pkgAnonFullNamePrefix := pkgPath + ".init.func"

	testCases := []struct {
		name          string
		fn            any
		wantFullName  string
		wantShortName string
		isAnon        bool
	}{
		{
			name:          "Top Level Function",
			fn:            topLevelFuncForTest,
			wantFullName:  pkgPath + ".topLevelFuncForTest",
			wantShortName: "topLevelFuncForTest",
		},
		{
			name:          "Value Receiver Method",
			fn:            ts.ValueReceiverMethod,
			wantFullName:  pkgPath + ".testStructForFunc.ValueReceiverMethod",
			wantShortName: "ValueReceiverMethod",
		},
		{
			name:          "Pointer Receiver Method",
			fn:            pts.PtrReceiverMethod,
			wantFullName:  pkgPath + ".(*testStructForFunc).PtrReceiverMethod",
			wantShortName: "PtrReceiverMethod",
		},
		{
			name:          "Anonymous Function Var",
			fn:            anonFuncForTest,
			wantFullName:  pkgAnonFullNamePrefix, // e.g., game/deps/basal.func1
			wantShortName: "func",                // e.g., func1
			isAnon:        true,
		},
		{
			name:          "Inline Anonymous Function",
			fn:            func() {},
			wantFullName:  inlineAnonFullNamePrefix,
			wantShortName: "func", // e.g., func1
			isAnon:        true,
		},
		{
			name:          "Nil Input",
			fn:            nil,
			wantFullName:  "nil",
			wantShortName: "nil",
		},
		{
			name:          "Non-function Input",
			fn:            123,
			wantFullName:  "not a function",
			wantShortName: "not a function",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fullName := GetFuncFullName(tc.fn)
			shortName := GetFuncShortName(tc.fn)

			if tc.isAnon {
				if tc.fn != nil {
					if !strings.HasPrefix(fullName, tc.wantFullName) {
						t.Errorf("GetFuncFullName() for anon = %q, want prefix %q", fullName, tc.wantFullName)
					}
					if !strings.HasPrefix(shortName, tc.wantShortName) {
						t.Errorf("GetFuncShortName() for anon = %q, want prefix %q", shortName, tc.wantShortName)
					}
				}
			} else {
				if fullName != tc.wantFullName {
					t.Errorf("GetFuncFullName() = %q, want %q", fullName, tc.wantFullName)
				}
				if shortName != tc.wantShortName {
					t.Errorf("GetFuncShortName() = %q, want %q", shortName, tc.wantShortName)
				}
			}
		})
	}
}

func TestGetFuncName(t *testing.T) {
	fn := topLevelFuncForTest
	fullName := GetFuncFullName(fn) // e.g., "game/deps/basal.topLevelFuncForTest"

	testCases := []struct {
		name string
		seps []rune
		want string
	}{
		{
			name: "No Separators",
			seps: nil,
			want: fullName,
		},
		{
			name: "Single Separator '.' (optimized path)",
			seps: []rune{'.'},
			want: "topLevelFuncForTest",
		},
		{
			name: "Single Separator '/'",
			seps: []rune{'/'},
			want: "basal.topLevelFuncForTest",
		},
		{
			name: "Multiple Separators",
			seps: []rune{'/', '.'},
			want: "topLevelFuncForTest",
		},
		{
			name: "Separator not in name",
			seps: []rune{'-'},
			want: fullName,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := GetFuncName(fn, tc.seps...)
			if got != tc.want {
				t.Errorf("GetFuncName() with seps %v = %q, want %q", tc.seps, got, tc.want)
			}
		})
	}
}

// --- Benchmarks ---

func BenchmarkGetFuncFullName(b *testing.B) {
	fn := topLevelFuncForTest
	b.Run("NoCache", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			// Reset cache every time to simulate no-cache scenario
			funcNameCache = sync.Map{}
			_ = GetFuncFullName(fn)
		}
	})

	// Prime the cache
	funcNameCache = sync.Map{}
	_ = GetFuncFullName(fn)

	b.Run("WithCache", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = GetFuncFullName(fn)
		}
	})
}

func BenchmarkGetFuncShortName(b *testing.B) {
	fn := topLevelFuncForTest
	b.Run("NoCache", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			funcNameCache = sync.Map{}
			_ = GetFuncShortName(fn)
		}
	})

	// Prime the cache
	funcNameCache = sync.Map{}
	_ = GetFuncShortName(fn)

	b.Run("WithCache", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = GetFuncShortName(fn)
		}
	})
}
