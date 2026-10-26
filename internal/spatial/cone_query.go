package spatial

import (
	"math"
	"unsafe"

	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/uid"
)

// QueryCone calls fn for every piece sharing want that may meet, inside area, the cone from origin
// along dir, halfAngle either side and radius long — within radius of origin and, for a cone
// narrower than a half-turn, not wholly beyond either of its edges — each once, and returns how
// many. A piece just outside may be reported, none meeting the cone in area is missed; a cone
// with no radius or no angle, or from an origin that is not a number, meets nothing.
func (g *Grid) QueryCone(
	area geom.AABB, origin, dir geom.Vec, halfAngle, radius float32, want Capability,
	fn func(uid.UID64),
) int {
	q, ok := g.newConeQuery(area, origin, dir, halfAngle, radius)
	if !ok {
		return 0
	}
	g.Ready()
	if !wide {
		return g.walkCone(&q, want, fn)
	}
	if g.colsBehind.Load() {
		g.colsUp()
	}
	return g.walkConeWide(&q, want, fn)
}

// colsUp builds the columns from the cells under the lock; every indexing after it builds them too.
func (g *Grid) colsUp() {
	g.mu.Lock()
	if g.colsBehind.Load() {
		g.cols.fill(g.cells)
		g.colsBehind.Store(false)
	}
	g.coned = true
	g.mu.Unlock()
}

// coneQuery is a cone as QueryCone meets the pieces: in float32, as they are, and wider every way
// by slack, so that no rounding loses a piece inside it.
type coneQuery struct {
	area           geom.AABB // the area asked, cut to the box round the cone
	x1, y1, x2, y2 uint32    // the area's cells
	ox, oy         float32
	r2             float32     // the radius and the slack, squared
	edges          [2]coneEdge // zero, so never passed, for a cone a half-turn wide or wider
	slack          float32
}

// coneEdge is how far past one edge of the cone a piece lies: cx·(x−ox) + cy·(y−oy), positive
// past it, taken at the corner where it is least, x and y being edges xi and yi of the piece
// (0 to 3: minX, minY, maxX, maxY).
type coneEdge struct {
	cx, cy float32
	xi, yi uint8
}

// newConeQuery readies the cone for the pieces of area; false when it can reach none.
func (g *Grid) newConeQuery(
	area geom.AABB, origin, dir geom.Vec, halfAngle, radius float32,
) (coneQuery, bool) {
	ox, oy, r := float64(origin.X), float64(origin.Y), float64(radius)
	if !(radius > 0) || !(halfAngle > 0) || math.IsNaN(ox) || math.IsNaN(oy) {
		return coneQuery{}, false
	}
	theta, alpha := math.Atan2(float64(dir.Y), float64(dir.X)), float64(halfAngle)
	// Rounding moves what the tests compare by a few steps of the largest number in play; the
	// slack is sixteen such steps.
	slack := (float64(g.maxCoord) + max(math.Abs(ox), math.Abs(oy)) + r) / (1 << 20)
	loX, loY := math.Cos(theta-alpha), math.Sin(theta-alpha)
	hiX, hiY := math.Cos(theta+alpha), math.Sin(theta+alpha)

	// The box round the sector holds its apex, the ends of its edges and the end of every axis
	// it spans.
	x1, x2 := min(ox, ox+r*loX, ox+r*hiX), max(ox, ox+r*loX, ox+r*hiX)
	y1, y2 := min(oy, oy+r*loY, oy+r*hiY), max(oy, oy+r*loY, oy+r*hiY)
	spans, ux, uy := math.Cos(alpha)-1e-9, math.Cos(theta), math.Sin(theta)
	if ux >= spans {
		x2 = ox + r
	}
	if -ux >= spans {
		x1 = ox - r
	}
	if uy >= spans {
		y2 = oy + r
	}
	if -uy >= spans {
		y1 = oy - r
	}
	cut := geom.NewAABB(
		geom.NewVec(max(area.TopLeft.X, float32(x1-slack)), max(area.TopLeft.Y, float32(y1-slack))),
		geom.NewVec(min(area.BottomRight.X, float32(x2+slack)),
			min(area.BottomRight.Y, float32(y2+slack))),
	)
	if cut.TopLeft.X > cut.BottomRight.X || cut.TopLeft.Y > cut.BottomRight.Y {
		return coneQuery{}, false
	}
	box, ok := g.clamp(cut)
	if !ok {
		return coneQuery{}, false
	}
	q := coneQuery{
		area:  box,
		x1:    g.cellOf(box.TopLeft.X),
		y1:    g.cellOf(box.TopLeft.Y),
		x2:    g.cellOf(box.BottomRight.X),
		y2:    g.cellOf(box.BottomRight.Y),
		ox:    origin.X,
		oy:    origin.Y,
		r2:    float32((r + slack) * (r + slack)),
		slack: float32(slack),
	}
	if alpha < math.Pi/2 {
		// Seen from the origin, x·lo.y − y·lo.x is positive past the low edge, y·hi.x − x·hi.y
		// past the high one.
		q.edges = [2]coneEdge{newConeEdge(loY, -loX), newConeEdge(-hiY, hiX)}
	}
	return q, true
}

func newConeEdge(cx, cy float64) coneEdge {
	e := coneEdge{cx: float32(cx), cy: float32(cy), xi: 2, yi: 3}
	if e.cx >= 0 {
		e.xi = 0
	}
	if e.cy >= 0 {
		e.yi = 1
	}
	return e
}

// holds reports whether piece b meets the area, lies within the radius and is past neither edge,
// by the steps and the comparisons the wide walk makes eight at a time.
func (q *coneQuery) holds(b *geom.AABB) bool {
	if !(b.TopLeft.X <= q.area.BottomRight.X && b.BottomRight.X >= q.area.TopLeft.X &&
		b.TopLeft.Y <= q.area.BottomRight.Y && b.BottomRight.Y >= q.area.TopLeft.Y) {
		return false
	}
	dx := b.TopLeft.X - q.ox
	if c := q.ox - b.BottomRight.X; c > dx {
		dx = c
	}
	if dx < 0 {
		dx = 0
	}
	dy := b.TopLeft.Y - q.oy
	if c := q.oy - b.BottomRight.Y; c > dy {
		dy = c
	}
	if dy < 0 {
		dy = 0
	}
	// Each product is rounded on its own, as the wide walk rounds it, never fused into the sum.
	if !(float32(dx*dx)+float32(dy*dy) <= q.r2) {
		return false
	}
	at := (*[4]float32)(unsafe.Pointer(b)) // minX, minY, maxX, maxY
	for _, e := range q.edges {
		if !(float32(e.cx*(at[e.xi]-q.ox))+float32(e.cy*(at[e.yi]-q.oy)) <= q.slack) {
			return false
		}
	}
	return true
}

// walkCone tests the pieces of the cells under q one at a time.
func (g *Grid) walkCone(q *coneQuery, want Capability, fn func(uid.UID64)) int {
	found := 0
	for y := q.y1; y <= q.y2; y++ {
		ownY := g.ownedFrom(y, q.y1)
		for x := q.x1; x <= q.x2; x++ {
			ownX := g.ownedFrom(x, q.x1)
			cell := y*g.side + x
			for k := g.starts[cell]; k < g.starts[cell+1]; k++ {
				c := &g.cells[k]
				if !c.caps.matches(want) || !(c.box.TopLeft.X >= ownX && c.box.TopLeft.Y >= ownY) ||
					!q.holds(&c.box) {
					continue
				}
				fn(c.id)
				found++
			}
		}
	}
	return found
}

// ownedFrom is where a piece in column or row n of the walk must begin for that cell to own it, as
// Query's cellOf test says: a piece in a cell begins in it or before it, so anywhere on the first
// column or row walked and past the cell's first edge on the rest.
func (g *Grid) ownedFrom(n, first uint32) float32 {
	if n == first {
		return float32(math.Inf(-1))
	}
	return float32(uint64(n) << g.cellRes)
}
