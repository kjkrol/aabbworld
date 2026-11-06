//go:build goexperiment.simd && (amd64 || arm64 || wasm)

package spatial

import (
	"math"
	"slices"

	"github.com/kjkrol/aabbworld/internal/simdx"
	"github.com/kjkrol/uid"
)

// wide says crowded cells are swept, and cones walked, a vector at a time; tests turn it off to
// compare.
var wide = simdx.On() || simdx.ArchFirst()

// avx2 says the kernels written for AVX2 sweep and walk rather than simd's: on amd64 they are as
// fast or faster at every width simd gives. Tests turn it off to try simd's.
var avx2 = simdx.ArchFirst()

// visitWide tests every two entries of one cell a vector of the rest at a time, and takes the
// pairs this cell owns.
func (s *pairSweep) visitWide(entries []sweepEntry, cx, cy uint32, fn func(a, b int32)) {
	if avx2 {
		s.visitAVX2(entries, cx, cy, fn)
		return
	}
	visitLanes(s, entries, cx, cy, fn)
}

// walkConeWide tests the pieces of the cells under q a vector at a time, each as holds tests one.
func (g *Grid) walkConeWide(q *coneQuery, want Capability, fn func(uid.UID64)) int {
	if avx2 {
		return g.walkConeAVX2(q, want, fn)
	}
	return walkConeLanes(g, q, want, fn)
}

// sweepCols is a cell's boxes one edge to a slice, so a vector of neighbours loads at once. Each
// slice runs simdx.Pad32 past the cell, for loads that start near its end.
type sweepCols struct{ minX, minY, maxX, maxY []float32 }

func (c *sweepCols) fill(entries []sweepEntry) {
	n := len(entries) + simdx.Pad32
	c.minX, c.minY = slices.Grow(c.minX[:0], n)[:n], slices.Grow(c.minY[:0], n)[:n]
	c.maxX, c.maxY = slices.Grow(c.maxX[:0], n)[:n], slices.Grow(c.maxY[:0], n)[:n]
	for i := range entries {
		b := &entries[i].box
		c.minX[i], c.minY[i] = b.TopLeft.X, b.TopLeft.Y
		c.maxX[i], c.maxY[i] = b.BottomRight.X, b.BottomRight.Y
	}
}

// coneCols is the cells' boxes one edge to a slice, at the cells' positions, so a vector of pieces
// loads at once. Each slice runs simdx.Pad32 past the cells, for loads that start near their end.
type coneCols struct{ minX, minY, maxX, maxY []float32 }

func (c *coneCols) fill(cells []gridCell) {
	n := len(cells) + simdx.Pad32
	c.minX, c.minY = slices.Grow(c.minX[:0], n)[:n], slices.Grow(c.minY[:0], n)[:n]
	c.maxX, c.maxY = slices.Grow(c.maxX[:0], n)[:n], slices.Grow(c.maxY[:0], n)[:n]
	minX, minY := c.minX[:len(cells)], c.minY[:len(cells)]
	maxX, maxY := c.maxX[:len(cells)], c.maxY[:len(cells)]
	for i := range cells {
		b := &cells[i].box
		minX[i], minY[i] = b.TopLeft.X, b.TopLeft.Y
		maxX[i], maxY[i] = b.BottomRight.X, b.BottomRight.Y
	}
}

// edge is where column or row n begins as cellOf counts, the first running on from minus infinity.
func (s *pairSweep) edge(n uint32) float32 {
	if n == 0 {
		return float32(math.Inf(-1))
	}
	return float32(uint64(n) << s.shift)
}
