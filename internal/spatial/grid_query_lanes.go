//go:build goexperiment.simd && (amd64 || arm64 || wasm)

package spatial

import (
	"math/bits"
	"simd"

	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/internal/simdx"
	"github.com/kjkrol/uid"
)

// queryLanes is Query over cells x1, y1 to x2, y2 of area, testing a vector of a cell's pieces at
// a time from the columns; a piece is the cell's to report if it begins in it, or the cell is on
// the area's first column or row, as cellOf has it.
func queryLanes(g *Grid, area geom.AABB, x1, y1, x2, y2 uint32, want Capability,
	fn func(uid.UID64)) int {
	c := &g.cols
	every := simd.BroadcastFloat32s
	ax1, ay1 := every(area.TopLeft.X), every(area.TopLeft.Y)
	ax2, ay2 := every(area.BottomRight.X), every(area.BottomRight.Y)
	w := ax1.Len()
	found := 0
	for y := y1; y <= y2; y++ {
		oy := g.ownedFrom(y, y1)
		ownY := every(oy)
		for x := x1; x <= x2; x++ {
			ox := g.ownedFrom(x, x1)
			cell := y*g.side + x
			start, end := int(g.starts[cell]), int(g.starts[cell+1])
			// A cell of fewer pieces than a vector holds is tested a piece at a time, as the
			// lanes test it: loading a vector for one or two costs more than it saves.
			if end-start < w {
				for k := start; k < end; k++ {
					p := &g.cells[k]
					b := &p.box
					if b.TopLeft.X <= area.BottomRight.X && b.BottomRight.X >= area.TopLeft.X &&
						b.TopLeft.Y <= area.BottomRight.Y && b.BottomRight.Y >= area.TopLeft.Y &&
						b.TopLeft.X >= ox && b.TopLeft.Y >= oy && p.caps.matches(want) {
						fn(p.id)
						found++
					}
				}
				continue
			}
			ownX := every(ox)
			for j := start; j < end; j += w {
				minX, minY := simd.LoadFloat32s(c.minX[j:]), simd.LoadFloat32s(c.minY[j:])
				maxX, maxY := simd.LoadFloat32s(c.maxX[j:]), simd.LoadFloat32s(c.maxY[j:])
				in := minX.LessEqual(ax2).And(maxX.GreaterEqual(ax1)).
					And(minY.LessEqual(ay2)).And(maxY.GreaterEqual(ay1)).
					And(minX.GreaterEqual(ownX)).And(minY.GreaterEqual(ownY))
				hits := simdx.Bits32(in)
				if left := end - j; left < w {
					hits &= 1<<left - 1
				}
				if hits == 0 {
					continue
				}
				// fn may run SSE code, slowed while the upper halves of the vectors are dirty.
				simdx.Settle()
				for hits != 0 {
					p := &g.cells[j+bits.TrailingZeros64(hits)]
					hits &= hits - 1
					if p.caps.matches(want) {
						fn(p.id)
						found++
					}
				}
			}
		}
	}
	simdx.Settle()
	return found
}
