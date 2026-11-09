//go:build goexperiment.simd && amd64

package spatial

import (
	"math/bits"
	"simd/archsimd"
)

// visitAVX2 tests every two entries of one cell, entry i against eight of the rest at once, and
// takes the pairs this cell owns.
func (s *pairSweep) visitAVX2(entries []sweepEntry, cx, cy uint32, fn func(a, b int32)) {
	c := &s.cols
	c.fill(entries)
	n := len(entries)
	// Both entries reach this cell, so the overlap's top-left corner lies in it or before it.
	ownX, ownY := archsimd.BroadcastFloat32x8(s.edge(cx)), archsimd.BroadcastFloat32x8(s.edge(cy))
	for i := range n - 1 {
		ax1, ay1 := archsimd.BroadcastFloat32x8(c.minX[i]), archsimd.BroadcastFloat32x8(c.minY[i])
		ax2, ay2 := archsimd.BroadcastFloat32x8(c.maxX[i]), archsimd.BroadcastFloat32x8(c.maxY[i])
		for j := i + 1; j < n; j += 8 {
			bx1 := archsimd.LoadFloat32x8Array((*[8]float32)(c.minX[j : j+8]))
			by1 := archsimd.LoadFloat32x8Array((*[8]float32)(c.minY[j : j+8]))
			bx2 := archsimd.LoadFloat32x8Array((*[8]float32)(c.maxX[j : j+8]))
			by2 := archsimd.LoadFloat32x8Array((*[8]float32)(c.maxY[j : j+8]))
			touch := ax1.LessEqual(bx2).And(ax2.GreaterEqual(bx1)).
				And(ay1.LessEqual(by2)).And(ay2.GreaterEqual(by1))
			own := ax1.Max(bx1).GreaterEqual(ownX).And(ay1.Max(by1).GreaterEqual(ownY))
			hits := touch.And(own).ToBits()
			if left := n - j; left < 8 {
				hits &= 1<<left - 1
			}
			if hits == 0 {
				continue
			}
			// fn may run SSE code, slowed while the upper halves of the vectors are dirty.
			archsimd.ClearAVXUpperBits()
			a := &entries[i]
			for hits != 0 {
				b := &entries[j+bits.TrailingZeros8(hits)]
				hits &= hits - 1
				if a.at != b.at {
					s.meet(a, b, fn)
				}
			}
		}
	}
	archsimd.ClearAVXUpperBits()
}
