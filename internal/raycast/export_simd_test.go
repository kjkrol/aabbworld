//go:build goexperiment.simd && amd64

package raycast

import (
	"simd/archsimd"
	"slices"
	"testing"

	"github.com/kjkrol/aabbworld/geom"
)

// SetWide has walks lay their slabs and walk the ground along their rays four angles at a time, or
// work each out as it is cast, until t ends; it skips t on a CPU without AVX2.
func SetWide(t testing.TB, on bool) {
	if !archsimd.X86.AVX2() {
		t.Skip("the CPU has no AVX2")
	}
	was := wide
	wide = on
	t.Cleanup(func() { wide = was })
}

// SideBySide is the ground along the rays from origin along one to four dirs for length over f
// sunk by bend, walked side by side as a walk's block walks them; it skips t on a CPU without
// AVX2.
func SideBySide(t testing.TB, f *HeightField, bend float32, origin geom.Vec, dirs []geom.Vec,
	length float32) (d, alt [][]float64) {
	t.Helper()
	if !archsimd.X86.AVX2() {
		t.Skip("the CPU has no AVX2")
	}
	var e elevation
	e.over(f, bend)
	if e.lat.cellsX == 0 {
		t.Fatalf("a field of no cells is never walked side by side")
	}
	o := vec{float64(origin.X), float64(origin.Y)}
	rays := make([]vec, len(dirs))
	var b profileBlock
	for j, dir := range dirs {
		rays[j] = vec{float64(dir.X), float64(dir.Y)}
		b.at[j] = j
	}
	b.n = len(dirs)
	if !b.walkWide(&e.lat, o, rays, float64(length)) {
		t.Fatalf("rays along %v for %v were not walked side by side", dirs, length)
	}
	for j := range b.n {
		e.close(&b.walks[j], o, rays[j], float64(length))
		d, alt = append(d, slices.Clone(b.walks[j].d)), append(alt, slices.Clone(b.walks[j].alt))
	}
	return d, alt
}
