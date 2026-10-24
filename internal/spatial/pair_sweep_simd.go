//go:build goexperiment.simd && amd64

package spatial

import (
	"math"
	"math/bits"
	"simd/archsimd"
	"slices"
)

// wide says crowded cells are swept four entries at a time; tests turn it off to compare. The
// masks are joined with VPAND on 256 bits, which takes AVX2.
var wide = archsimd.X86.AVX2()

// sweepCols is a cell's boxes one edge to a slice, so four neighbours load at once. Each slice
// runs three past the cell, for loads that start near its end.
type sweepCols struct{ minX, minY, maxX, maxY []float64 }

func (c *sweepCols) fill(entries []sweepEntry) {
	n := len(entries) + 3
	c.minX, c.minY = slices.Grow(c.minX[:0], n)[:n], slices.Grow(c.minY[:0], n)[:n]
	c.maxX, c.maxY = slices.Grow(c.maxX[:0], n)[:n], slices.Grow(c.maxY[:0], n)[:n]
	for i := range entries {
		b := &entries[i].box
		c.minX[i], c.minY[i] = b.TopLeft.X, b.TopLeft.Y
		c.maxX[i], c.maxY[i] = b.BottomRight.X, b.BottomRight.Y
	}
}

// visitWide tests every two entries of one cell, entry i against four of the rest at once, and
// takes the pairs this cell owns.
func (s *pairSweep) visitWide(entries []sweepEntry, cx, cy uint32, fn func(a, b int32)) {
	c := &s.cols
	c.fill(entries)
	n := len(entries)
	// Both entries reach this cell, so the overlap's top-left corner lies in it or before it.
	ownX, ownY := archsimd.BroadcastFloat64x4(s.edge(cx)), archsimd.BroadcastFloat64x4(s.edge(cy))
	for i := range n - 1 {
		ax1, ay1 := archsimd.BroadcastFloat64x4(c.minX[i]), archsimd.BroadcastFloat64x4(c.minY[i])
		ax2, ay2 := archsimd.BroadcastFloat64x4(c.maxX[i]), archsimd.BroadcastFloat64x4(c.maxY[i])
		for j := i + 1; j < n; j += 4 {
			bx1 := archsimd.LoadFloat64x4Array((*[4]float64)(c.minX[j : j+4]))
			by1 := archsimd.LoadFloat64x4Array((*[4]float64)(c.minY[j : j+4]))
			bx2 := archsimd.LoadFloat64x4Array((*[4]float64)(c.maxX[j : j+4]))
			by2 := archsimd.LoadFloat64x4Array((*[4]float64)(c.maxY[j : j+4]))
			touch := ax1.LessEqual(bx2).And(ax2.GreaterEqual(bx1)).
				And(ay1.LessEqual(by2)).And(ay2.GreaterEqual(by1))
			own := ax1.Max(bx1).GreaterEqual(ownX).And(ay1.Max(by1).GreaterEqual(ownY))
			hits := touch.And(own).ToBits()
			if left := n - j; left < 4 {
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

// edge is where column or row n begins as cellOf counts, the first running on from minus infinity.
func (s *pairSweep) edge(n uint32) float64 {
	if n == 0 {
		return math.Inf(-1)
	}
	return float64(uint64(n) << s.shift)
}
