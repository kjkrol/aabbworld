//go:build goexperiment.simd && (amd64 || arm64 || wasm)

package spatial

import (
	"math/bits"
	"simd"

	"github.com/kjkrol/aabbworld/internal/simdx"
	"github.com/kjkrol/uid"
)

// walkConeLanes is walkConeAVX2 on simd's vectors, at whatever width they run.
func walkConeLanes(g *Grid, q *coneQuery, want Capability, fn func(uid.UID64)) int {
	c := &g.cols
	edges := [4][]float32{c.minX, c.minY, c.maxX, c.maxY}
	loX, loY := edges[q.edges[0].xi], edges[q.edges[0].yi]
	hiX, hiY := edges[q.edges[1].xi], edges[q.edges[1].yi]
	every := simd.BroadcastFloat32s
	ox, oy, r2, slack, zero := every(q.ox), every(q.oy), every(q.r2), every(q.slack), every(0)
	ax1, ay1 := every(q.area.TopLeft.X), every(q.area.TopLeft.Y)
	ax2, ay2 := every(q.area.BottomRight.X), every(q.area.BottomRight.Y)
	loCX, loCY := every(q.edges[0].cx), every(q.edges[0].cy)
	hiCX, hiCY := every(q.edges[1].cx), every(q.edges[1].cy)
	w := ox.Len()
	found := 0
	for y := q.y1; y <= q.y2; y++ {
		ownY := every(g.ownedFrom(y, q.y1))
		for x := q.x1; x <= q.x2; x++ {
			ownX := every(g.ownedFrom(x, q.x1))
			from, to := g.spans.entries(y*g.side + x)
			end := int(to)
			for j := int(from); j < end; j += w {
				minX, minY := simd.LoadFloat32s(c.minX[j:]), simd.LoadFloat32s(c.minY[j:])
				maxX, maxY := simd.LoadFloat32s(c.maxX[j:]), simd.LoadFloat32s(c.maxY[j:])
				in := minX.LessEqual(ax2).And(maxX.GreaterEqual(ax1)).
					And(minY.LessEqual(ay2)).And(maxY.GreaterEqual(ay1)).
					And(minX.GreaterEqual(ownX)).And(minY.GreaterEqual(ownY))
				dx := minX.Sub(ox).Max(ox.Sub(maxX)).Max(zero)
				dy := minY.Sub(oy).Max(oy.Sub(maxY)).Max(zero)
				lo := loCX.Mul(simd.LoadFloat32s(loX[j:]).Sub(ox)).
					Add(loCY.Mul(simd.LoadFloat32s(loY[j:]).Sub(oy)))
				hi := hiCX.Mul(simd.LoadFloat32s(hiX[j:]).Sub(ox)).
					Add(hiCY.Mul(simd.LoadFloat32s(hiY[j:]).Sub(oy)))
				cone := dx.Mul(dx).Add(dy.Mul(dy)).LessEqual(r2).
					And(lo.LessEqual(slack)).And(hi.LessEqual(slack))
				hits := simdx.Bits32(in.And(cone))
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
