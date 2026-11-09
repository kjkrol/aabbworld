//go:build goexperiment.simd && (amd64 || arm64 || wasm)

package raycast

import "github.com/kjkrol/aabbworld/internal/simdx"

// wide says the slabs of a walk are laid a vector of angles at a time, and with AVX2 the ground is
// walked in lockstep; tests turn it off to compare.
var wide = simdx.On() || simdx.ArchFirst()

// avx2 says the kernels written for AVX2 lay the slabs and walk the ground rather than simd's: on
// amd64 they are as fast or faster at every width simd gives, and the lockstep has no other.
// Tests turn it off to try simd's.
var avx2 = simdx.ArchFirst()

// fillWide lays the slabs of a box, relative to the origin, against the rays of run's angles, a
// vector at a time, storing up to simdx.Pad64 past the run.
func (s *slabs) fillWide(x1, y1, x2, y2 float64, r run) {
	if avx2 {
		s.fillAVX2(x1, y1, x2, y2, r)
		return
	}
	fillLanes(s, x1, y1, x2, y2, r)
}

// invertWide turns every value of v into its reciprocal, a vector at a time; len(v) is
// simdx.Pad64 past a whole number of vectors at least.
func invertWide(v []float64) {
	if avx2 {
		invertAVX2(v)
		return
	}
	invertLanes(v)
}
