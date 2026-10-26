package raycast

import (
	"math"

	"github.com/kjkrol/aabbworld/geom"
)

// lattice is a HeightField as a scan reads it: the ground at a point, and the ground along a ray
// sunk by bend·d² with the distance d from the eye.
type lattice struct {
	f              *HeightField
	bend           float32
	cellsX, cellsY int64 // a corner fewer than the corners, as many where the axis wraps
	wrapX, wrapY   bool
}

// set has l read f, sunk by bend; a field of no cells is ground at 0 everywhere.
func (l *lattice) set(f *HeightField, bend float32) {
	*l = lattice{f: f, bend: bend, cellsX: int64(f.Cols), cellsY: int64(f.Rows), wrapX: f.WrapX,
		wrapY: f.WrapY}
	if !l.wrapX {
		l.cellsX--
	}
	if !l.wrapY {
		l.cellsY--
	}
	if l.cellsX <= 0 || l.cellsY <= 0 || !(f.Cell > 0) {
		l.cellsX, l.cellsY, l.wrapX, l.wrapY = 0, 0, false, false
	}
}

// Along is what walk records, handed over as a ProfileField: an elevation holds its lattice as
// its profile, so the whole cone is sampled as over any exact profile, and casts read it by walk.
func (l *lattice) Along(origin, dir geom.Vec, length float32, visit func(d, alt float32) bool) {
	w := profileWalk{length: float64(length)}
	l.walk(&w, vec{float64(origin.X), float64(origin.Y)}, vec{float64(dir.X), float64(dir.Y)})
	for k, d := range w.d {
		if !visit(float32(d), float32(w.alt[k])) {
			return
		}
	}
}

// walk records the ground along the ray from origin along the unit dir up to w.length, cell by
// cell: where the ray crosses a cell's fold, where it leaves the cell, again there at the height
// of the cell it enters where that differs (off the lattice the ground is 0), and the end.
func (l *lattice) walk(w *profileWalk, origin, dir vec) {
	if l.cellsX == 0 {
		l.end(w, origin, dir)
		return
	}
	// the walk sums t cell after cell, so in float64
	size, end := float64(l.f.Cell), w.length
	fx, fy := origin.x/size, origin.y/size
	cx, cy := math.Floor(fx), math.Floor(fy)
	bx, by := dir.x/size, dir.y/size // cells a unit along the ray
	stepX, nextX, deltaX := crossings(fx, bx)
	stepY, nextY, deltaY := crossings(fy, by)
	ix, iy := wrapCell(int64(cx), l.cellsX, l.wrapX), wrapCell(int64(cy), l.cellsY, l.wrapY)
	h0, h1, h2, h3 := l.corners(ix, iy)
	for t := 0.0; ; {
		ax, ay := fx-cx, fy-cy
		acrossX := nextX <= nextY
		exit := nextY
		if acrossX {
			exit = nextX
		}
		limit := exit
		if end < limit {
			limit = end
		}
		if fold, alt, ok := folded(h0, h1, h2, h3, ax, ay, bx, by, t, limit); ok &&
			!l.point(w, fold, alt) {
			return
		}
		if exit >= end {
			l.end(w, origin, dir)
			return
		}
		// how far along the edge it crosses the ray leaves the cell, the edge the other coordinate
		s := ax + bx*exit
		if acrossX {
			s = ay + by*exit
		}
		s = clamp01(s)
		alt := exited(h0, h1, h2, h3, acrossX, stepX, stepY, s)
		if !l.point(w, exit, alt) {
			return
		}
		if acrossX {
			cx += stepX
			nextX += deltaX
			ix = stepCell(ix, int64(stepX), l.cellsX, l.wrapX)
			h0, h1, h2, h3 = l.crossedX(h0, h1, h2, h3, ix, iy, stepX > 0)
		} else {
			cy += stepY
			nextY += deltaY
			iy = stepCell(iy, int64(stepY), l.cellsY, l.wrapY)
			h0, h1, h2, h3 = l.crossedY(h0, h1, h2, h3, ix, iy, stepY > 0)
		}
		if entry := entered(h0, h1, h2, h3, acrossX, stepX, stepY, s); entry != alt &&
			!l.point(w, exit, entry) {
			return
		}
		t = exit
	}
}

// end records the ground at the end of the ray, read where the end lies in float32.
func (l *lattice) end(w *profileWalk, origin, dir vec) {
	length := float32(w.length)
	p := geom.NewVec(float32(origin.x)+float32(dir.x)*length,
		float32(origin.y)+float32(dir.y)*length)
	l.point(w, float64(length), float64(l.at(p)))
}

// point records the ground alt at d along the ray and says whether the walk goes on. Both are
// rounded to float32 first, as a ProfileField hands its points over, so that the field and a
// profile of it are read alike.
func (l *lattice) point(w *profileWalk, d, alt float64) bool {
	d32 := float32(d)
	return w.take(float64(d32), float64(l.sunk(float32(alt), d32)))
}

// sunk is the ground alt high at d from the eye, as it lies below the eye's level.
func (l *lattice) sunk(alt, d float32) float32 {
	if l.bend == 0 {
		return alt
	}
	return alt - l.bend*d*d
}

// at is the ground's height under p as the field's triangles lie, 0 off the lattice.
func (l *lattice) at(p geom.Vec) float32 {
	if l.cellsX == 0 {
		return 0
	}
	size := float64(l.f.Cell)
	fx, fy := float64(p.X)/size, float64(p.Y)/size
	x0, y0 := math.Floor(fx), math.Floor(fy)
	h0, h1, h2, h3 := l.corners(wrapCell(int64(x0), l.cellsX, l.wrapX),
		wrapCell(int64(y0), l.cellsY, l.wrapY))
	return float32(drawnAt(h0, h1, h2, h3, fx-x0, fy-y0))
}

// corners is the heights at cell (x, y)'s corners, wrapped onto the lattice where it wraps:
// top-left, top-right, bottom-left, bottom-right; 0 off the lattice.
func (l *lattice) corners(x, y int64) (h0, h1, h2, h3 float64) {
	if x < 0 || x >= l.cellsX || y < 0 || y >= l.cellsY {
		return 0, 0, 0, 0
	}
	cols := int64(l.f.Cols)
	x1, y1 := x+1, y+1
	if x1 == cols {
		x1 = 0
	}
	if y1 == int64(l.f.Rows) {
		y1 = 0
	}
	c, top, bottom := l.f.Corners, y*cols, y1*cols
	return float64(c[top+x]), float64(c[top+x1]), float64(c[bottom+x]), float64(c[bottom+x1])
}

// crossedX is the corners of cell (x, y), entered east or west from the cell of corners h0–h3
// beside it: the two corners they share are kept, the other two read.
func (l *lattice) crossedX(h0, h1, h2, h3 float64, x, y int64, east bool) (float64, float64,
	float64, float64) {
	switch {
	case x < 0 || x >= l.cellsX || y < 0 || y >= l.cellsY:
		return 0, 0, 0, 0
	case !l.wrapX && (east && x == 0 || !east && x == l.cellsX-1): // from off the lattice
		return l.corners(x, y)
	}
	cols, c := int64(l.f.Cols), l.f.Corners
	top, bottom := y*cols, (y+1)*cols
	if y+1 == int64(l.f.Rows) {
		bottom = 0
	}
	if east {
		x1 := x + 1
		if x1 == cols {
			x1 = 0
		}
		return h1, float64(c[top+x1]), h3, float64(c[bottom+x1])
	}
	return float64(c[top+x]), h0, float64(c[bottom+x]), h2
}

// crossedY is the corners of cell (x, y), entered south or north from the cell of corners h0–h3
// beside it: the two corners they share are kept, the other two read.
func (l *lattice) crossedY(h0, h1, h2, h3 float64, x, y int64, south bool) (float64, float64,
	float64, float64) {
	switch {
	case x < 0 || x >= l.cellsX || y < 0 || y >= l.cellsY:
		return 0, 0, 0, 0
	case !l.wrapY && (south && y == 0 || !south && y == l.cellsY-1): // from off the lattice
		return l.corners(x, y)
	}
	cols, c := int64(l.f.Cols), l.f.Corners
	x1 := x + 1
	if x1 == cols {
		x1 = 0
	}
	if south {
		row := (y + 1) * cols
		if y+1 == int64(l.f.Rows) {
			row = 0
		}
		return h2, h3, float64(c[row+x]), float64(c[row+x1])
	}
	row := y * cols
	return float64(c[row+x]), float64(c[row+x1]), h0, h1
}

// wrapCell is cell v of an axis of n cells, wrapped onto them where the axis wraps.
func wrapCell(v, n int64, wraps bool) int64 {
	if wraps {
		return (v%n + n) % n
	}
	return v
}

// stepCell is wrapCell(v+s) for a step s of one cell either way.
func stepCell(v, s, n int64, wraps bool) int64 {
	v += s
	switch {
	case !wraps:
	case v == n:
		v = 0
	case v < 0:
		v = n - 1
	}
	return v
}

// crossings is how a ray starting f cells along an axis, b cells a unit along the ray, crosses its
// cells: which way, how far along the ray the first edge lies and how far apart the next ones.
func crossings(f, b float64) (step, next, delta float64) {
	switch {
	case b > 0:
		return 1, (math.Floor(f) + 1 - f) / b, 1 / b
	case b < 0:
		return -1, (math.Floor(f) - f) / b, -1 / b
	}
	return 0, math.Inf(1), math.Inf(1)
}

// folded is where along the ray, a in the cell of corners h0–h3 and b cells a unit along it, it
// crosses the cell's fold, and the ground there, when that lies after from and before to; false
// where it does not, or where the ray runs along the fold.
func folded(h0, h1, h2, h3, ax, ay, bx, by, from, to float64) (t, alt float64, ok bool) {
	if math.Abs(h0-h3) < math.Abs(h1-h2) { // folded along 0–3: u = v
		if bx == by {
			return 0, 0, false
		}
		if t = (ay - ax) / (bx - by); t > from && t < to {
			return t, h0 + clamp01(ax+bx*t)*(h3-h0), true
		}
		return 0, 0, false
	}
	if bx+by == 0 { // folded along 1–2: u + v = 1
		return 0, 0, false
	}
	if t = (1 - ax - ay) / (bx + by); t > from && t < to {
		return t, h1 + clamp01(ay+by*t)*(h2-h1), true
	}
	return 0, 0, false
}

// exited is the ground where the ray leaves the cell of corners h0–h3, s along the edge it
// crosses.
func exited(h0, h1, h2, h3 float64, acrossX bool, stepX, stepY, s float64) float64 {
	switch {
	case acrossX && stepX > 0:
		return h1 + s*(h3-h1)
	case acrossX:
		return h0 + s*(h2-h0)
	case stepY > 0:
		return h2 + s*(h3-h2)
	}
	return h0 + s*(h1-h0)
}

// entered is the ground where the ray enters the cell of corners h0–h3, s along the edge it
// crosses.
func entered(h0, h1, h2, h3 float64, acrossX bool, stepX, stepY, s float64) float64 {
	switch {
	case acrossX && stepX > 0:
		return h0 + s*(h2-h0)
	case acrossX:
		return h1 + s*(h3-h1)
	case stepY > 0:
		return h0 + s*(h1-h0)
	}
	return h2 + s*(h3-h2)
}

// clamp01 is min(max(x, 0), 1) but for the sign of a zero, which no height worked out from it
// shows, without the builtins' care for it.
func clamp01(x float64) float64 {
	switch {
	case x < 0:
		return 0
	case x > 1:
		return 1
	}
	return x
}

// drawnAt is the ground at (u, v) in a cell of corners h0–h3: two flat triangles folded along the
// diagonal whose corners differ less in height, along 1–2 where they differ alike.
func drawnAt(h0, h1, h2, h3, u, v float64) float64 {
	if math.Abs(h0-h3) < math.Abs(h1-h2) {
		if u >= v {
			return h0 + u*(h1-h0) + v*(h3-h1)
		}
		return h0 + v*(h2-h0) + u*(h3-h2)
	}
	if u+v <= 1 {
		return h0 + u*(h1-h0) + v*(h2-h0)
	}
	return h3 + (1-u)*(h2-h3) + (1-v)*(h1-h3)
}

// footOn is the ground under the middle of box as a cone's Ground would stand it on f: sunk by
// bend·d² with its distance d from origin, the nearest way round a world of w by h that wraps.
func footOn(f *HeightField, bend float32, box geom.AABB, origin vec, w, h float64) float64 {
	var l lattice
	l.set(f, bend)
	c := centerOf(box).vec32()
	alt := l.at(c)
	if bend == 0 {
		return float64(alt)
	}
	dx := float32(shortestDelta(float64(c.X)-origin.x, w))
	dy := float32(shortestDelta(float64(c.Y)-origin.y, h))
	return float64(alt - bend*(dx*dx+dy*dy))
}

// minLanes is the fewest rays a block walks side by side; fewer are walked as each is cast.
const minLanes = 2

// profileBlock is the ground along the rays of a few of a walk's angles, walked side by side
// before they are cast: walks[j] is the profile of angle at[j] for the first n lanes. from and to
// are the angles it was laid for, those left to each cast among them.
type profileBlock struct {
	from, to int
	n        int
	at       [4]int
	walks    [4]profileWalk
	lanes    lanes
}

// lanes is a block's walk in progress, a ray to a lane: the rays and what walk keeps of each, the
// measures of the lattice in every lane, the cells the rays are in and step into, where the
// corners of the latter lie in the field, and a log of what each round records.
type lanes struct {
	origin                                   vec
	dir                                      [4]vec
	bx, by, sx, sy, nx, ny, dx, dy, t        [4]float64
	cx, cy, ix, iy                           [4]float64 // walk's: the cells, the latter wrapped
	fx, fy, end, one                         [4]float64
	cols, rows, cellsX, cellsY, lastX, lastY [4]float64
	bend                                     [4]float32
	bent                                     bool
	index                                    [4][4]int32 // [corner][lane]
	cells                                    [2]cells
	s, alt, exit                             [4]float64 // along the edge left, how high, how far
	rounds                                   []round
	kept                                     []uint16 // a round's points the lanes keep
	entries                                  [4][]float64
}

// cells is a cell of each lane: its corners as float32 bits, [corner][lane], and whether it lies
// on the lattice, all bits set where it does.
type cells struct {
	corners [4][4]uint32
	on      [4]int64
}

// round is a cell of each lane: where it crosses the fold and how high, then where it is left and
// how high, four lanes each.
type round [16]float64

// reset readies b for a walk: no angle is laid.
func (b *profileBlock) reset() { b.from, b.to, b.n = -1, -1, 0 }

// lay walks the ground along the rays of the angles a walk along angles casts from i on — those
// not within minGap of the angle before — up to four side by side, unless the block was laid for
// angle i already; fewer than minLanes are left to each cast. dirs are the angles' rays.
func (b *profileBlock) lay(e *elevation, origin vec, length, minGap float64, angles []float64,
	dirs []vec, i int) {
	if i >= b.from && i <= b.to {
		return
	}
	b.from, b.n = i, 0
	for j := i; j < len(angles) && b.n < len(b.at); j++ {
		if j > i && angles[j]-angles[j-1] < minGap {
			continue
		}
		b.at[b.n], b.to = j, j
		b.n++
	}
	if b.n < minLanes || !b.walkWide(&e.lat, origin, dirs, length) {
		b.n = 0
		return
	}
	for j := range b.n {
		e.close(&b.walks[j], origin, dirs[b.at[j]], length)
	}
}

// of is the profile of angle at, nil where the block holds none.
func (b *profileBlock) of(at int) *profileWalk {
	for j := range b.n {
		if b.at[j] == at {
			return &b.walks[j]
		}
	}
	return nil
}
