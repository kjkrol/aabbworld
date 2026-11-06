//go:build goexperiment.simd && (amd64 || arm64 || wasm)

package spatial

import (
	"math/bits"
	"simd"

	"github.com/kjkrol/aabbworld/internal/simdx"
)

// visitLanes is visitAVX2 on simd's vectors, at whatever width they run: entry i against a vector
// of the rest at once. Kernels over simd's vectors are functions: see simdx.
func visitLanes(s *pairSweep, entries []sweepEntry, cx, cy uint32, fn func(a, b int32)) {
	c := &s.cols
	c.fill(entries)
	n := len(entries)
	every := simd.BroadcastFloat32s
	// Both entries reach this cell, so the overlap's top-left corner lies in it or before it.
	ownX, ownY := every(s.edge(cx)), every(s.edge(cy))
	w := ownX.Len()
	for i := range n - 1 {
		ax1, ay1 := every(c.minX[i]), every(c.minY[i])
		ax2, ay2 := every(c.maxX[i]), every(c.maxY[i])
		for j := i + 1; j < n; j += w {
			bx1, by1 := simd.LoadFloat32s(c.minX[j:]), simd.LoadFloat32s(c.minY[j:])
			bx2, by2 := simd.LoadFloat32s(c.maxX[j:]), simd.LoadFloat32s(c.maxY[j:])
			touch := ax1.LessEqual(bx2).And(ax2.GreaterEqual(bx1)).
				And(ay1.LessEqual(by2)).And(ay2.GreaterEqual(by1))
			own := ax1.Max(bx1).GreaterEqual(ownX).And(ay1.Max(by1).GreaterEqual(ownY))
			hits := simdx.Bits32(touch.And(own))
			if left := n - j; left < w {
				hits &= 1<<left - 1
			}
			if hits == 0 {
				continue
			}
			// fn may run SSE code, slowed while the upper halves of the vectors are dirty.
			simdx.Settle()
			a := &entries[i]
			for hits != 0 {
				b := &entries[j+bits.TrailingZeros64(hits)]
				hits &= hits - 1
				if a.at != b.at {
					s.meet(a, b, fn)
				}
			}
		}
	}
	simdx.Settle()
}
