package raycast_test

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/internal/raycast"
	"github.com/kjkrol/uid"
)

// fieldOracle is a HeightField's ground as the owner of a faceted ground hands it over, its rule
// spelled out plainly: a ProfileField along a ray and the ground at a point, both sunk by bend·d²
// with the distance d from the eye, the shortest way round a world of w by h where it wraps.
type fieldOracle struct {
	f    *raycast.HeightField
	bend float32
	eye  geom.Vec
	w, h float64
}

func (o fieldOracle) Along(origin, dir geom.Vec, length float32, visit func(d, alt float32) bool) {
	sunk := func(d, alt float32) bool { return visit(d, alt-o.bend*d*d) }
	size, end := float64(o.f.Cell), float64(length)
	fx, fy := float64(origin.X)/size, float64(origin.Y)/size
	cx, cy := math.Floor(fx), math.Floor(fy)
	bx, by := float64(dir.X)/size, float64(dir.Y)/size
	stepX, nextX, deltaX := oracleLattice(fx, bx)
	stepY, nextY, deltaY := oracleLattice(fy, by)
	h := o.cellCorners(int64(cx), int64(cy))
	for t := 0.0; ; {
		ax, ay := fx-cx, fy-cy
		exit := min(nextX, nextY)
		if fold, alt, ok := oracleFolded(h, ax, ay, bx, by); ok && fold > t &&
			fold < min(exit, end) {
			if !sunk(float32(fold), float32(alt)) {
				return
			}
		}
		if exit >= end {
			sunk(length, o.at(geom.NewVec(origin.X+dir.X*length, origin.Y+dir.Y*length)))
			return
		}
		u := min(max(ax+bx*exit, 0), 1)
		v := min(max(ay+by*exit, 0), 1)
		var alt float64
		switch {
		case nextX <= nextY && stepX > 0:
			alt = h[1] + v*(h[3]-h[1])
		case nextX <= nextY:
			alt = h[0] + v*(h[2]-h[0])
		case stepY > 0:
			alt = h[2] + u*(h[3]-h[2])
		default:
			alt = h[0] + u*(h[1]-h[0])
		}
		if !sunk(float32(exit), float32(alt)) {
			return
		}
		acrossX := nextX <= nextY
		if acrossX {
			cx += stepX
			nextX += deltaX
		} else {
			cy += stepY
			nextY += deltaY
		}
		h = o.cellCorners(int64(cx), int64(cy))
		var entry float64
		switch {
		case acrossX && stepX > 0:
			entry = h[0] + v*(h[2]-h[0])
		case acrossX:
			entry = h[1] + v*(h[3]-h[1])
		case stepY > 0:
			entry = h[0] + u*(h[1]-h[0])
		default:
			entry = h[2] + u*(h[3]-h[2])
		}
		if entry != alt && !sunk(float32(exit), float32(entry)) {
			return
		}
		t = exit
	}
}

func oracleLattice(f, b float64) (step, next, delta float64) {
	switch {
	case b > 0:
		return 1, (math.Floor(f) + 1 - f) / b, 1 / b
	case b < 0:
		return -1, (math.Floor(f) - f) / b, -1 / b
	}
	return 0, math.Inf(1), math.Inf(1)
}

func oracleFolded(h [4]float64, ax, ay, bx, by float64) (t, alt float64, ok bool) {
	if math.Abs(h[0]-h[3]) < math.Abs(h[1]-h[2]) {
		if bx == by {
			return 0, 0, false
		}
		t = (ay - ax) / (bx - by)
		u := min(max(ax+bx*t, 0), 1)
		return t, h[0] + u*(h[3]-h[0]), true
	}
	if bx+by == 0 {
		return 0, 0, false
	}
	t = (1 - ax - ay) / (bx + by)
	v := min(max(ay+by*t, 0), 1)
	return t, h[1] + v*(h[2]-h[1]), true
}

func cellsAlong(corners int, wraps bool) int64 {
	if wraps {
		return int64(corners)
	}
	return int64(corners) - 1
}

func foldAxis(v, n int64, wraps bool) (int64, bool) {
	if wraps {
		return (v%n + n) % n, true
	}
	return v, v >= 0 && v < n
}

func (o fieldOracle) cellCorners(x, y int64) (h [4]float64) {
	cx, okX := foldAxis(x, cellsAlong(o.f.Cols, o.f.WrapX), o.f.WrapX)
	cy, okY := foldAxis(y, cellsAlong(o.f.Rows, o.f.WrapY), o.f.WrapY)
	if !okX || !okY {
		return h
	}
	h[0] = o.cornerAt(cx, cy)
	h[1] = o.cornerAt(cx+1, cy)
	h[2] = o.cornerAt(cx, cy+1)
	h[3] = o.cornerAt(cx+1, cy+1)
	return h
}

func (o fieldOracle) cornerAt(x, y int64) float64 {
	fx, _ := foldAxis(x, int64(o.f.Cols), o.f.WrapX)
	fy, _ := foldAxis(y, int64(o.f.Rows), o.f.WrapY)
	return float64(o.f.Corners[int(fy)*o.f.Cols+int(fx)])
}

// at is the field under p: two flat triangles a cell, folded along the diagonal whose corners
// differ less in height, along 1–2 where they differ alike; 0 off the field.
func (o fieldOracle) at(p geom.Vec) float32 {
	size := float64(o.f.Cell)
	fx, fy := float64(p.X)/size, float64(p.Y)/size
	x0, y0 := math.Floor(fx), math.Floor(fy)
	h := o.cellCorners(int64(x0), int64(y0))
	h0, h1, h2, h3, u, v := h[0], h[1], h[2], h[3], fx-x0, fy-y0
	if math.Abs(h0-h3) < math.Abs(h1-h2) {
		if u >= v {
			return float32(h0 + u*(h1-h0) + v*(h3-h1))
		}
		return float32(h0 + v*(h2-h0) + u*(h3-h2))
	}
	if u+v <= 1 {
		return float32(h0 + u*(h1-h0) + v*(h2-h0))
	}
	return float32(h3 + (1-u)*(h2-h3) + (1-v)*(h1-h3))
}

// ground is Cone.Ground over the field: at p, sunk as far as p lies from the eye.
func (o fieldOracle) ground(p geom.Vec) float32 {
	dx := float32(shortestWay(float64(p.X)-float64(o.eye.X), o.w))
	dy := float32(shortestWay(float64(p.Y)-float64(o.eye.Y), o.h))
	return o.at(p) - o.bend*(dx*dx+dy*dy)
}

func shortestWay(d, size float64) float64 {
	switch {
	case size <= 0:
		return d
	case d > size/2:
		return d - size
	case d < -size/2:
		return d + size
	}
	return d
}

// randomField is cols by rows corners cell apart of rolling ground, cliffs and level stretches —
// cells whose fold is a ridge or a trough, and level ones whose diagonals differ alike.
func randomField(r *rand.Rand, cols, rows int, cell float32,
	wrapX, wrapY bool) *raycast.HeightField {
	f := &raycast.HeightField{Corners: make([]float32, cols*rows), Cols: cols, Rows: rows,
		Cell: cell, WrapX: wrapX, WrapY: wrapY}
	ax, ay := 0.2+r.Float64(), 0.2+r.Float64()
	for y := range rows {
		for x := range cols {
			h := 8 + 6*math.Sin(ax*float64(x))*math.Cos(ay*float64(y))
			switch r.IntN(6) {
			case 0:
				h = math.Round(h/4) * 4 // a level stretch among its neighbours
			case 1:
				h += 20 * r.Float64() // a spike: the folds about it ridges
			case 2:
				h = 0
			}
			f.Corners[y*cols+x] = float32(h)
		}
	}
	return f
}

// unitDir is a direction the way a scan rounds its rays, now and then along an axis or a diagonal,
// where the ray runs along a fold.
func unitDir(r *rand.Rand) geom.Vec {
	switch r.IntN(8) {
	case 0:
		return geom.NewVec(1, 0)
	case 1:
		return geom.NewVec(0, -1)
	case 2:
		return geom.NewVec(float32(math.Sqrt(0.5)), float32(math.Sqrt(0.5)))
	case 3:
		return geom.NewVec(-float32(math.Sqrt(0.5)), float32(math.Sqrt(0.5)))
	}
	a := 2 * math.Pi * r.Float64()
	return geom.NewVec(float32(math.Cos(a)), float32(math.Sin(a)))
}

// The profile a scan reads off a field is, point for point, the one the field's owner hands over:
// every fold crossed, every edge left, the step down off the field, the end.
func TestHeights_AFieldIsReadAsItsOwnerHandsItsProfileOver(t *testing.T) {
	r := rand.New(rand.NewPCG(101, 103))
	for trial := range 2000 {
		wrapX, wrapY := r.IntN(3) == 0, r.IntN(3) == 0
		cell := float32(5+r.IntN(40)) + float32(r.IntN(4))/3
		f := randomField(r, 2+r.IntN(24), 2+r.IntN(24), cell, wrapX, wrapY)
		span := float32(max(f.Cols, f.Rows)) * cell
		origin := geom.NewVec(-span/4+1.5*span*float32(r.Float64()),
			-span/4+1.5*span*float32(r.Float64()))
		dir, length := unitDir(r), float32(1+r.IntN(int(2*span)))
		bend := float32(0)
		if r.IntN(2) == 0 {
			bend = float32(r.Float64() / float64(span))
		}
		o := fieldOracle{f: f, bend: bend, eye: origin}
		gotD, gotAlt := raycast.ProfileOf(f, bend, nil, nil, origin, dir, length)
		wantD, wantAlt := raycast.ProfileOf(nil, 0, o, o.ground, origin, dir, length)
		handedD, handedAlt := raycast.ProfileOf(nil, 0, raycast.FieldProfile(f, bend), nil, origin,
			dir, length)
		if !slices.Equal(handedD, gotD) || !slices.Equal(handedAlt, gotAlt) {
			t.Fatalf("trial %d: the field handed over as a profile is %v %v, read off it %v %v",
				trial, handedD, handedAlt, gotD, gotAlt)
		}
		if !slices.Equal(gotD, wantD) || !slices.Equal(gotAlt, wantAlt) {
			t.Fatalf("trial %d, a ray from %v along %v for %v over %d×%d corners %v apart "+
				"(wrap %v, %v), bend %v:\nread off the field %v\n                   %v\n"+
				"handed over        %v\n                   %v", trial, origin, dir, length,
				f.Cols, f.Rows, cell, wrapX, wrapY, bend, gotD, gotAlt, wantD, wantAlt)
		}
	}
}

// fan is n rays the way a scan rounds them: spread at random, so that they part into different
// cells, two degrees apart as a sweep casts them, a hair apart, or along the axes and diagonals.
func fan(r *rand.Rand, n int) []geom.Vec {
	base, step := 2*math.Pi*r.Float64(), 0.0
	switch r.IntN(4) {
	case 0:
		step = 2 * math.Pi * r.Float64()
	case 1:
		step = math.Pi / 90
	case 2:
		step = 1e-3 * r.Float64()
	}
	dirs := make([]geom.Vec, n)
	for j := range dirs {
		if step == 0 {
			dirs[j] = unitDir(r)
			continue
		}
		a := base + float64(j)*step
		dirs[j] = geom.NewVec(float32(math.Cos(a)), float32(math.Sin(a)))
	}
	return dirs
}

func sameBits(a, b []float64) bool {
	return slices.EqualFunc(a, b, func(x, y float64) bool {
		return math.Float64bits(x) == math.Float64bits(y)
	})
}

// Rays walked side by side record, ray by ray and to the bit, the points each walks alone: one to
// four rays parting or close, from on and off the field and from the corners of its cells, onto it
// and off it, to the end or to a point rounded onto the end, over fields that end, wrap along one
// axis or both, flat or bent.
func TestHeights_RaysWalkedSideBySideRecordWhatEachWalksAlone(t *testing.T) {
	r := rand.New(rand.NewPCG(139, 149))
	for trial := range 4000 {
		wrapX, wrapY := r.IntN(3) == 0, r.IntN(3) == 0
		cell := float32(5 + r.IntN(40))
		if r.IntN(2) == 0 {
			cell += float32(r.IntN(4)) / 3
		}
		f := randomField(r, 2+r.IntN(24), 2+r.IntN(24), cell, wrapX, wrapY)
		if r.IntN(4) == 0 { // corners at -0, where the sign of a zero in the arithmetic shows
			for i, h := range f.Corners {
				if h == 0 {
					f.Corners[i] = float32(math.Copysign(0, -1))
				}
			}
		}
		span := float32(max(f.Cols, f.Rows)) * cell
		origin := geom.NewVec(-span/4+1.5*span*float32(r.Float64()),
			-span/4+1.5*span*float32(r.Float64()))
		if r.IntN(4) == 0 {
			origin = geom.NewVec(float32(r.IntN(f.Cols))*cell, float32(r.IntN(f.Rows))*cell)
		}
		bend := float32(0)
		if r.IntN(2) == 0 {
			bend = float32(r.Float64() / float64(span))
		}
		dirs := fan(r, 1+r.IntN(4))
		length := float32(1 + r.IntN(int(2*span)))
		if r.IntN(4) == 0 {
			if d, _ := raycast.ProfileOf(f, bend, nil, nil, origin, dirs[0], length); len(d) > 1 {
				length = float32(d[r.IntN(len(d)-1)])
			}
		}
		gotD, gotAlt := raycast.SideBySide(t, f, bend, origin, dirs, length)
		for j, dir := range dirs {
			wantD, wantAlt := raycast.ProfileOf(f, bend, nil, nil, origin, dir, length)
			if !sameBits(gotD[j], wantD) || !sameBits(gotAlt[j], wantAlt) {
				t.Fatalf("trial %d, ray %d of %v from %v for %v over %d×%d corners %v apart "+
					"(wrap %v, %v), bend %v:\nside by side %v\n             %v\n"+
					"alone        %v\n             %v", trial, j, dirs, origin, length, f.Cols,
					f.Rows, cell, wrapX, wrapY, bend, gotD[j], gotAlt[j], wantD, wantAlt)
			}
		}
	}
}

func TestHeights_TheGroundAtAPointIsTheFieldsTriangles(t *testing.T) {
	r := rand.New(rand.NewPCG(107, 109))
	for trial := range 200 {
		f := randomField(r, 2+r.IntN(12), 2+r.IntN(12), 10, r.IntN(2) == 0, r.IntN(2) == 0)
		o := fieldOracle{f: f}
		for range 50 {
			p := geom.NewVec(float32(-60+300*r.Float64()), float32(-60+300*r.Float64()))
			if got, want := raycast.HeightAt(f, p), o.at(p); got != want {
				t.Fatalf("trial %d: the ground at %v is %v, the field's triangles say %v",
					trial, p, got, want)
			}
		}
	}
}

// stripSpace is a fakeSpace wrapping along x alone, or y alone.
type stripSpace struct {
	*fakeSpace
	wrapX, wrapY bool
}

func (s stripSpace) Bounds() (uint32, uint32, bool, bool) {
	return uint32(s.w), uint32(s.h), s.wrapX, s.wrapY
}

// fieldScene is an eye somewhere over a field among walkers, walls and forests standing on it and
// hawks over it, in a world of 1000 by 1000 wrapping as the field does; a field that does not wrap
// ends short of the world's far edges, so that rays step off it.
func fieldScene(r *rand.Rand, wrapX, wrapY bool, bend float32) (raycast.QueryableSpace,
	fieldOracle, map[uid.UID64]band, map[uid.UID64]float32) {
	const side, cell = 1000, 25
	cols, rows := 30, 34
	if wrapX {
		cols = side / cell
	}
	if wrapY {
		rows = side / cell
	}
	f := randomField(r, cols, rows, cell, wrapX, wrapY)
	s := newFake(side, side, false)
	var space raycast.QueryableSpace = stripSpace{s, wrapX, wrapY}
	ex, ey := float32(40+r.IntN(900)), float32(40+r.IntN(900))
	s.put(eye, ex, ey, 10, 10)
	o := fieldOracle{f: f, bend: bend, eye: geom.NewVec(ex+5, ey+5)}
	if wrapX {
		o.w = side
	}
	if wrapY {
		o.h = side
	}
	bands, taus := map[uid.UID64]band{}, map[uid.UID64]float32{}
	for id := uid.UID64(100); id < 180; id++ {
		x, y := float32(r.IntN(side)), float32(r.IntN(side))
		w, h := float32(2+r.IntN(30)), float32(2+r.IntN(30))
		s.put(id, x, y, w, h)
		foot := o.ground(geom.NewVec(x+w/2, y+h/2))
		switch id % 5 {
		case 0:
			bands[id] = band{foot + 30, foot + 34} // a hawk
		case 1:
			bands[id] = band{foot, foot + 12} // a wall
		case 2:
			bands[id] = band{foot, foot + 8} // a forest
			taus[id] = 0.5
		default:
			bands[id] = band{foot, foot + 2} // a walker
		}
	}
	eyeAt := o.ground(o.eye) + 1.5
	if r.IntN(3) == 0 {
		eyeAt += 40
	}
	bands[eye] = band{eyeAt, eyeAt}
	return space, o, bands, taus
}

// scanned is what a scan through c finds: the entities nearest first, the reach and the hidden
// ground at k angles.
type scanned struct {
	seen    []seenPair
	depths  []float32
	shadows []raycast.Shadow
}

func scanOf(t *testing.T, s raycast.QueryableSpace, c raycast.Cone, k int) scanned {
	t.Helper()
	v := scan(t, s, eye, c)
	var out scanned
	v.Entities(func(id uid.UID64, d float32) { out.seen = append(out.seen, seenPair{id, d}) })
	out.depths = v.Depths(k, nil)
	out.shadows = v.Shadows(k, nil)
	return out
}

// A scan over a field sees what one over the field's profile sees, to the last bit: the same
// entities at the same distances, the same reach and the same hidden ground, over a field that
// ends, wraps along one axis or both, flat or bent.
func TestHeights_AScanOverAFieldSeesWhatOneOverItsProfileSees(t *testing.T) {
	r := rand.New(rand.NewPCG(113, 127))
	compared := 0
	for trial := range 60 {
		wrapX, wrapY := trial%3 == 1, trial%6 == 2
		bend := float32(0)
		if trial%2 == 0 {
			bend = 1.0 / 16000 // the ground 10 below the eye's level 400 off
		}
		s, o, bands, taus := fieldScene(r, wrapX, wrapY, bend)
		dir := 2 * math.Pi * r.Float64()
		base := seeThrough(raycast.Cone{
			Direction: geom.NewVec(float32(math.Cos(dir)), float32(math.Sin(dir))),
			HalfAngle: float32(math.Pi/8 + r.Float64()*math.Pi/3),
			Radius:    float32(200 + r.IntN(400)),
		}, taus)
		base = elevated(base, bands[eye][0], bands)
		field := base
		field.Heights, field.Bend = o.f, bend
		profiled := base
		profiled.Profile, profiled.Ground = o, o.ground
		got, want := scanOf(t, s, field, 33), scanOf(t, s, profiled, 33)
		if !slices.Equal(got.seen, want.seen) {
			t.Errorf("trial %d (wrap %v, %v, bend %v): over the field %v seen, over its profile %v",
				trial, wrapX, wrapY, bend, got.seen, want.seen)
		}
		if !slices.Equal(got.depths, want.depths) {
			t.Errorf("trial %d: over the field the reach is %v, over its profile %v",
				trial, got.depths, want.depths)
		}
		if !slices.Equal(got.shadows, want.shadows) {
			t.Errorf("trial %d: over the field the ground hidden is %v, over its profile %v",
				trial, got.shadows, want.shadows)
		}
		compared += len(got.seen)
	}
	if compared < 300 {
		t.Errorf("%d entities seen in all, too few to compare", compared)
	}
}

// level is a field cols by rows corners 10 apart, every corner alt high.
func level(cols, rows int, alt float32) *raycast.HeightField {
	f := &raycast.HeightField{Corners: make([]float32, cols*rows), Cols: cols, Rows: rows, Cell: 10}
	for i := range f.Corners {
		f.Corners[i] = alt
	}
	return f
}

// A ridge along a cell's fold hides what lies behind it where the cell's edges, half as high,
// would not.
func TestHeights_AFoldRisingToARidgeHidesWhatLiesBehindIt(t *testing.T) {
	s := newFake(1000, 1000, false)
	s.put(eye, 100, 100, 10, 10)
	s.put(walker, 300, 100, 10, 10)
	f := level(60, 60, 0)
	// the cell (20, 10) from x 200: corners 1 and 2 high, 0 and 3 level, so it folds along 1–2
	f.Corners[10*60+21], f.Corners[11*60+20] = 9, 9
	cone := elevated(eastward(math.Pi/8, 400), 6, map[uid.UID64]band{walker: {0, 5}})
	cone.Heights = f
	assertVisible(t, visible(t, s, eye, cone))
	f.Corners[10*60+21], f.Corners[11*60+20] = 0.5, 0.5
	assertVisible(t, visible(t, s, eye, cone), walker)
}

// Off the field the ground is 0: a walker past its edge stands below it, out of sight of an eye
// over the field, and so does the ground there.
func TestHeights_OffTheFieldTheGroundStepsDownOutOfSight(t *testing.T) {
	s := newFake(2000, 2000, false)
	s.put(eye, 395, 95, 10, 10)
	s.put(walker, 695, 95, 10, 10)
	f := level(51, 51, 10) // to x 500
	cone := elevated(eastward(math.Pi/8, 400), 11.5, map[uid.UID64]band{walker: {0, 2}})
	cone.Heights = f
	assertVisible(t, visible(t, s, eye, cone))
	got := ahead3(t, s, cone)
	if len(got) != 1 || got[0].From > 100 || got[0].To != 400 {
		t.Errorf("shadows ahead = %+v, want one from the edge 100 off to the radius", got)
	}
	f.Cols = 81 // to x 800: the walker stands on it
	f.Corners = append(f.Corners, make([]float32, 81*51-len(f.Corners))...)
	for i := range f.Corners {
		f.Corners[i] = 10
	}
	cone.Elevation = elevated(cone, 11.5, map[uid.UID64]band{walker: {10, 12}}).Elevation
	assertVisible(t, visible(t, s, eye, cone), walker)
}

// at 4 km a unit, an eye 1.5 over a level field sees it sink out of sight 74 off
func TestHeights_ABentFieldSinksOutOfSightPastTheHorizon(t *testing.T) {
	s := newFake(1000, 1000, false)
	s.put(eye, 100, 100, 10, 10)
	s.put(walker, 340, 100, 10, 10)
	cone := elevated(eastward(math.Pi/8, 400), 1.5, map[uid.UID64]band{walker: {0, 2}})
	cone.Heights = level(80, 80, 0)
	assertVisible(t, visible(t, s, eye, cone), walker)
	cone.Bend = 0.87 / (2 * 6371000.0 / 4000)
	sunk := cone.Elevation
	cone.Elevation = func(id uid.UID64) (float32, float32) {
		b, t := sunk(id)
		return b - cone.Bend*240*240, t - cone.Bend*240*240
	}
	assertVisible(t, visible(t, s, eye, cone))
	if d := ahead(t, s, eye, cone); d > 80 {
		t.Errorf("reach ahead = %.1f, want the horizon about 74 off", d)
	}
}

func TestHeights_ScanDoesNotAllocateOnceWarm(t *testing.T) {
	r := rand.New(rand.NewPCG(131, 137))
	s, o, bands, taus := fieldScene(r, true, false, 1.0/16000)
	cone := elevated(seeThrough(eastward(math.Pi/3, 500), taus), bands[eye][0], bands)
	cone.Heights, cone.Bend = o.f, o.bend
	v := &raycast.View{}
	v.Scan(s, eye, cone)
	depths := make([]float32, 0, 64)
	shadows := make([]raycast.Shadow, 0, 512)
	allocs := testing.AllocsPerRun(20, func() {
		v.Scan(s, eye, cone)
		v.Entities(func(uid.UID64, float32) {})
		depths = v.Depths(63, depths[:0])
		shadows = v.Shadows(63, shadows[:0])
	})
	if allocs > 0 {
		t.Errorf("a warm Scan over a field allocates %.0f times per run, want none", allocs)
	}
}
