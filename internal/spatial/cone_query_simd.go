//go:build goexperiment.simd && amd64

package spatial

import (
	"math/bits"
	"simd/archsimd"

	"github.com/kjkrol/uid"
)

// walkConeAVX2 tests the pieces of the cells under q eight at a time, each as holds tests one.
func (g *Grid) walkConeAVX2(q *coneQuery, want Capability, fn func(uid.UID64)) int {
	c := &g.cols
	edges := [4][]float32{c.minX, c.minY, c.maxX, c.maxY}
	loX, loY := edges[q.edges[0].xi], edges[q.edges[0].yi]
	hiX, hiY := edges[q.edges[1].xi], edges[q.edges[1].yi]
	every := archsimd.BroadcastFloat32x8
	ox, oy, r2, slack, zero := every(q.ox), every(q.oy), every(q.r2), every(q.slack), every(0)
	ax1, ay1 := every(q.area.TopLeft.X), every(q.area.TopLeft.Y)
	ax2, ay2 := every(q.area.BottomRight.X), every(q.area.BottomRight.Y)
	loCX, loCY := every(q.edges[0].cx), every(q.edges[0].cy)
	hiCX, hiCY := every(q.edges[1].cx), every(q.edges[1].cy)
	found := 0
	for y := q.y1; y <= q.y2; y++ {
		ownY := every(g.ownedFrom(y, q.y1))
		for x := q.x1; x <= q.x2; x++ {
			ownX := every(g.ownedFrom(x, q.x1))
			from, to := g.spans.entries(y*g.side + x)
			end := int(to)
			for j := int(from); j < end; j += 8 {
				minX, minY := load8(c.minX, j), load8(c.minY, j)
				maxX, maxY := load8(c.maxX, j), load8(c.maxY, j)
				in := minX.LessEqual(ax2).And(maxX.GreaterEqual(ax1)).
					And(minY.LessEqual(ay2)).And(maxY.GreaterEqual(ay1)).
					And(minX.GreaterEqual(ownX)).And(minY.GreaterEqual(ownY))
				dx := minX.Sub(ox).Max(ox.Sub(maxX)).Max(zero)
				dy := minY.Sub(oy).Max(oy.Sub(maxY)).Max(zero)
				lo := loCX.Mul(load8(loX, j).Sub(ox)).Add(loCY.Mul(load8(loY, j).Sub(oy)))
				hi := hiCX.Mul(load8(hiX, j).Sub(ox)).Add(hiCY.Mul(load8(hiY, j).Sub(oy)))
				cone := dx.Mul(dx).Add(dy.Mul(dy)).LessEqual(r2).
					And(lo.LessEqual(slack)).And(hi.LessEqual(slack))
				hits := in.And(cone).ToBits()
				if left := end - j; left < 8 {
					hits &= 1<<left - 1
				}
				if hits == 0 {
					continue
				}
				// fn may run SSE code, slowed while the upper halves of the vectors are dirty.
				archsimd.ClearAVXUpperBits()
				for hits != 0 {
					p := &g.cells[j+bits.TrailingZeros8(hits)]
					hits &= hits - 1
					if p.caps.matches(want) {
						fn(p.id)
						found++
					}
				}
			}
		}
	}
	archsimd.ClearAVXUpperBits()
	return found
}

func load8(s []float32, at int) archsimd.Float32x8 {
	return archsimd.LoadFloat32x8Array((*[8]float32)(s[at : at+8]))
}
