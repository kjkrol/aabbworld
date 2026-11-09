//go:build goexperiment.simd && (amd64 || arm64 || wasm)

package raycast

import (
	"testing"

	"github.com/kjkrol/aabbworld/internal/simdx"
)

// SetWide has walks lay their slabs a vector of angles at a time, and with AVX2 walk the ground
// along their rays four at a time, or work each out as it is cast, until t ends; it skips t where
// no wide kernel runs.
func SetWide(t testing.TB, on bool) {
	if !simdx.On() && !simdx.ArchFirst() {
		t.Skip("no wide kernel runs here")
	}
	was := wide
	wide = on
	t.Cleanup(func() { wide = was })
}

// EachKernel calls f, naming it, with walks laid wide by every kernel this CPU runs in turn, the
// AVX2 one and simd's, until t ends; it skips t where none runs.
func EachKernel(t testing.TB, f func(kernel string)) {
	if !simdx.On() && !simdx.ArchFirst() {
		t.Skip("no wide kernel runs here")
	}
	wasWide, wasAVX2 := wide, avx2
	t.Cleanup(func() { wide, avx2 = wasWide, wasAVX2 })
	for _, k := range []struct {
		name       string
		avx2, runs bool
	}{{"avx2", true, simdx.ArchFirst()}, {"simd", false, simdx.On()}} {
		if k.runs {
			wide, avx2 = true, k.avx2
			f(k.name)
		}
	}
	wide, avx2 = wasWide, wasAVX2
}
