//go:build goexperiment.simd && amd64

package raycast

import "simd/archsimd"

// wide says the slabs of a walk are laid four angles at a time; tests turn it off to compare. The
// broadcasts take AVX2.
var wide = archsimd.X86.AVX2()

// fillWide lays the slabs of a box, relative to the origin, against the rays of run's angles, four
// at a time, storing up to three past the run. Each lane works out what hitDistance does.
func (s *slabs) fillWide(x1, y1, x2, y2 float64, r run) {
	bx1, bx2 := archsimd.BroadcastFloat64x4(x1), archsimd.BroadcastFloat64x4(x2)
	by1, by2 := archsimd.BroadcastFloat64x4(y1), archsimd.BroadcastFloat64x4(y2)
	var zero archsimd.Float64x4
	for j := r.from; j < r.to; j += 4 {
		ix := archsimd.LoadFloat64x4Array((*[4]float64)(s.ix[j : j+4]))
		iy := archsimd.LoadFloat64x4Array((*[4]float64)(s.iy[j : j+4]))
		// A ray along an axis has an infinite reciprocal across it, and so either crossing is
		// infinite: the whole line between the box's edges, none beside them.
		tx1, tx2 := bx1.Mul(ix), bx2.Mul(ix)
		ty1, ty2 := by1.Mul(iy), by2.Mul(iy)
		// Adding zero makes a zero near +0, as math.Max has it.
		near := tx1.Min(tx2).Max(ty1.Min(ty2)).Max(zero).Add(zero)
		far := tx1.Max(tx2).Min(ty1.Max(ty2))
		p := r.at + j
		near.StoreArray((*[4]float64)(s.near[p : p+4]))
		far.StoreArray((*[4]float64)(s.far[p : p+4]))
	}
	// The walk's scalar code is slowed while the upper halves of the vectors are dirty.
	archsimd.ClearAVXUpperBits()
}

// invertWide turns every value of v into its reciprocal, four at a time; len(v) is three past a
// whole number of vectors at least.
func invertWide(v []float64) {
	one := archsimd.BroadcastFloat64x4(1)
	for j := 0; j+4 <= len(v); j += 4 {
		x := archsimd.LoadFloat64x4Array((*[4]float64)(v[j : j+4]))
		one.Div(x).StoreArray((*[4]float64)(v[j : j+4]))
	}
	archsimd.ClearAVXUpperBits()
}
