//go:build goexperiment.simd && (amd64 || arm64 || wasm)

package raycast

import (
	"simd"

	"github.com/kjkrol/aabbworld/internal/simdx"
)

// fillLanes is fillAVX2 on simd's vectors, at whatever width they run: each lane works out what
// hitDistance does. Kernels over simd's vectors are functions: see simdx.
func fillLanes(s *slabs, x1, y1, x2, y2 float64, r run) {
	bx1, bx2 := simd.BroadcastFloat64s(x1), simd.BroadcastFloat64s(x2)
	by1, by2 := simd.BroadcastFloat64s(y1), simd.BroadcastFloat64s(y2)
	zero := simd.BroadcastFloat64s(0)
	w := int32(zero.Len())
	for j := r.from; j < r.to; j += w {
		ix, iy := simd.LoadFloat64s(s.ix[j:]), simd.LoadFloat64s(s.iy[j:])
		// A ray along an axis has an infinite reciprocal across it, and so either crossing is
		// infinite: the whole line between the box's edges, none beside them.
		tx1, tx2 := bx1.Mul(ix), bx2.Mul(ix)
		ty1, ty2 := by1.Mul(iy), by2.Mul(iy)
		// Adding zero makes a zero near +0, as math.Max has it.
		near := tx1.Min(tx2).Max(ty1.Min(ty2)).Max(zero).Add(zero)
		far := tx1.Max(tx2).Min(ty1.Max(ty2))
		p := r.at + j
		near.Store(s.near[p:])
		far.Store(s.far[p:])
	}
	simdx.Settle()
}

// invertLanes is invertAVX2 on simd's vectors, at whatever width they run.
func invertLanes(v []float64) {
	one := simd.BroadcastFloat64s(1)
	w := one.Len()
	for j := 0; j+w <= len(v); j += w {
		one.Div(simd.LoadFloat64s(v[j:])).Store(v[j:])
	}
	simdx.Settle()
}
