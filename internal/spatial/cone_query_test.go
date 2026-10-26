package spatial

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/kjkrol/aabbworld/geom"
	iplane "github.com/kjkrol/aabbworld/internal/plane"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

// cone is a cone as the tests ask the grid for it.
type cone struct {
	origin      geom.Vec
	dir         geom.Vec
	half, reach float32
}

func (c cone) theta() float64 { return math.Atan2(float64(c.dir.Y), float64(c.dir.X)) }

// randomCone has its origin anywhere over the grid and a little past it.
func randomCone(rng *rand.Rand) cone {
	a := rng.Float64() * 2 * math.Pi
	return cone{
		origin: geom.NewVec(float32(rng.Float64()*1224-100), float32(rng.Float64()*1224-100)),
		dir:    geom.NewVec(float32(math.Cos(a)), float32(math.Sin(a))),
		half:   float32(0.02 + rng.Float64()*3.1),
		reach:  float32(5 + rng.Float64()*600),
	}
}

// square is the square round the cone's circle.
func (c cone) square() geom.AABB {
	return geom.NewAABB(geom.NewVec(c.origin.X-c.reach, c.origin.Y-c.reach),
		geom.NewVec(c.origin.X+c.reach, c.origin.Y+c.reach))
}

func askCone(g *Grid, area geom.AABB, c cone, want Capability) (ids []uid.UID64, n int) {
	n = g.QueryCone(area, c.origin, c.dir, c.half, c.reach, want,
		func(id uid.UID64) { ids = append(ids, id) })
	return ids, n
}

// reachesCone reports whether box b holds a point of cone c, worked out exactly in float64, the
// cone narrowed by shrink on every side.
func reachesCone(b geom.AABB, c cone, shrink float64) bool {
	ox, oy := float64(c.origin.X), float64(c.origin.Y)
	x1, y1 := float64(b.TopLeft.X)-ox, float64(b.TopLeft.Y)-oy
	x2, y2 := float64(b.BottomRight.X)-ox, float64(b.BottomRight.Y)-oy
	box := [][2]float64{{x1, y1}, {x2, y1}, {x2, y2}, {x1, y2}}
	r, alpha := float64(c.reach)-shrink, float64(c.half)-shrink/float64(c.reach)
	if r <= 0 || alpha <= 0 {
		return false
	}
	theta := c.theta()
	lo := [2]float64{math.Cos(theta - alpha), math.Sin(theta - alpha)}
	hi := [2]float64{math.Cos(theta + alpha), math.Sin(theta + alpha)}
	insideLo := func(p [2]float64) float64 { return lo[0]*p[1] - lo[1]*p[0] }
	insideHi := func(p [2]float64) float64 { return hi[1]*p[0] - hi[0]*p[1] }
	switch {
	case alpha >= math.Pi:
		return distanceTo(box) <= r
	case alpha < math.Pi/2:
		return distanceTo(clip(clip(box, insideLo), insideHi)) <= r
	}
	return distanceTo(clip(box, insideLo)) <= r || distanceTo(clip(box, insideHi)) <= r
}

// clip keeps the part of a convex polygon where f is not negative.
func clip(poly [][2]float64, f func([2]float64) float64) [][2]float64 {
	var out [][2]float64
	for i, p := range poly {
		q := poly[(i+1)%len(poly)]
		fp, fq := f(p), f(q)
		if fp >= 0 {
			out = append(out, p)
		}
		if (fp >= 0) != (fq >= 0) {
			t := fp / (fp - fq)
			out = append(out, [2]float64{p[0] + t*(q[0]-p[0]), p[1] + t*(q[1]-p[1])})
		}
	}
	return out
}

// distanceTo is how far the origin lies from a convex polygon wound counterclockwise: 0 inside
// it, +Inf when it is empty.
func distanceTo(poly [][2]float64) float64 {
	if len(poly) == 0 {
		return math.Inf(1)
	}
	inside, d := true, math.Inf(1)
	for i, p := range poly {
		q := poly[(i+1)%len(poly)]
		if (q[0]-p[0])*-p[1]-(q[1]-p[1])*-p[0] < 0 {
			inside = false
		}
		ex, ey := q[0]-p[0], q[1]-p[1]
		t := 0.0
		if l := ex*ex + ey*ey; l > 0 {
			t = min(max(-(p[0]*ex+p[1]*ey)/l, 0), 1)
		}
		d = min(d, math.Hypot(p[0]+t*ex, p[1]+t*ey))
	}
	if inside {
		return 0
	}
	return d
}

// mayReport says whether the grid may report box b for cone c: within the radius and, for a
// cone narrower than a half-turn, not wholly beyond either edge, both within tol.
func mayReport(b geom.AABB, c cone, tol float64) bool {
	ox, oy := float64(c.origin.X), float64(c.origin.Y)
	xs := [2]float64{float64(b.TopLeft.X) - ox, float64(b.BottomRight.X) - ox}
	ys := [2]float64{float64(b.TopLeft.Y) - oy, float64(b.BottomRight.Y) - oy}
	if math.Hypot(max(xs[0], -xs[1], 0), max(ys[0], -ys[1], 0)) > float64(c.reach)+tol {
		return false
	}
	alpha := float64(c.half)
	if alpha >= math.Pi/2 {
		return true
	}
	theta := c.theta()
	lo := [2]float64{math.Cos(theta - alpha), math.Sin(theta - alpha)}
	hi := [2]float64{math.Cos(theta + alpha), math.Sin(theta + alpha)}
	pastLo, pastHi := true, true
	for _, x := range xs {
		for _, y := range ys {
			pastLo = pastLo && lo[0]*y-lo[1]*x < -tol
			pastHi = pastHi && hi[0]*y-hi[1]*x > tol
		}
	}
	return !pastLo && !pastHi
}

// checkCone holds a cone query of sc.grid to the brute force: every piece meeting the cone inside
// the area reported, each reported piece in the area and allowed, none reported twice.
func checkCone(t *testing.T, sc *scene, area geom.AABB, c cone, want Capability) {
	t.Helper()
	got, n := askCone(sc.grid, area, c, want)
	if n != len(got) {
		t.Fatalf("%+v: QueryCone counted %d, called back %d times", c, n, len(got))
	}
	walked, ok := sc.grid.clamp(area)
	must, may := map[uid.UID64]bool{}, map[uid.UID64]int{}
	for _, it := range sc.items {
		if !ok || !it.Caps.matches(want) {
			continue
		}
		for _, img := range images(it.Box) {
			piece, in := sc.grid.clamp(img)
			if !in || !piece.Intersects(walked) {
				continue
			}
			inside := geom.NewAABB(
				geom.NewVec(max(piece.TopLeft.X, walked.TopLeft.X),
					max(piece.TopLeft.Y, walked.TopLeft.Y)),
				geom.NewVec(min(piece.BottomRight.X, walked.BottomRight.X),
					min(piece.BottomRight.Y, walked.BottomRight.Y)))
			if reachesCone(inside, c, 1e-3) {
				must[it.ID] = true
			}
			if mayReport(piece, c, 1e-2) {
				may[it.ID]++
			}
		}
	}
	times := map[uid.UID64]int{}
	for _, id := range got {
		times[id]++
	}
	for id := range must {
		if times[id] == 0 {
			t.Fatalf("%+v over %v: %v meets the cone inside the area and was not reported", c,
				area, id)
		}
	}
	for id, k := range times {
		if k > may[id] {
			t.Fatalf("%+v over %v: %v reported %d times, %d of its pieces may be", c, area, id, k,
				may[id])
		}
	}
}

func TestGrid_QueryConeAgreesWithBruteForce(t *testing.T) {
	for name, edges := range map[string]iplane.Edges{
		"closed": 0, "torus": iplane.Torus, "wrapX openY": iplane.WrapX | iplane.OpenY,
	} {
		for _, cells := range []Resolution{Size16x16, Size128x128} {
			t.Run(name+"/"+cells.String(), func(t *testing.T) {
				rng := rand.New(rand.NewPCG(19, 23))
				sc := newScene(t, edges, rng, 700, []float32{4, 5, 16, 60})
				sc.grid = NewGrid(sc.surface, Size1024x1024, cells)
				sc.grid.Rebuild(sc.items)
				reported, inSquares := 0, 0
				for range 8 {
					for q := range 40 {
						c := randomCone(rng)
						area := c.square()
						if q%5 == 0 {
							area = geom.NewAABBAt(geom.NewVec(float32(rng.Float64()*900),
								float32(rng.Float64()*900)), float32(rng.Float64()*300),
								float32(rng.Float64()*300))
						}
						want := AnyCapability
						if q%3 == 0 {
							want = collides
						}
						checkCone(t, sc, area, c, want)
						if q%5 != 0 && want == AnyCapability {
							_, n := askCone(sc.grid, area, c, want)
							reported += n
							inSquares += count(sc.grid, area, want)
						}
					}
					sc.step(rng)
				}
				if reported*3 > inSquares*2 {
					t.Errorf("cones kept %d of the %d pieces in the squares round them", reported,
						inSquares)
				}
			})
		}
	}
}

// plainGrid is a 1024 grid of 32-unit cells, closed at every edge, over items placed as given.
func plainGrid(boxes ...geom.AABB) *Grid {
	g := NewGrid(iplane.NewEuclidean2D(1024, 1024), Size1024x1024, Size32x32)
	items := make([]Item, len(boxes))
	for i, b := range boxes {
		items[i] = Item{ID: uid.UID64(i + 1), Box: plane.NewAABB(b.TopLeft,
			b.BottomRight.X-b.TopLeft.X, b.BottomRight.Y-b.TopLeft.Y)}
	}
	g.Rebuild(items)
	return g
}

func TestGrid_QueryConeKeepsToTheCone(t *testing.T) {
	at := func(x, y, w, h float32) geom.AABB { return geom.NewAABBAt(geom.NewVec(x, y), w, h) }
	g := plainGrid(
		at(495, 495, 10, 10),  // 1: round the origin
		at(560, 495, 10, 10),  // 2: straight ahead
		at(420, 495, 10, 10),  // 3: straight behind
		at(500, 440, 10, 10),  // 4: beside, within the radius
		at(595, 490, 10, 20),  // 5: ahead, reaching just into the radius
		at(610, 490, 10, 20),  // 6: ahead, past the radius
		at(560, 475, 10, 10),  // 7: across the edge of the cone
		at(550, 470, 10, 9.9), // 8: wholly past the edge, a hair away
		at(550, 470, 10, 10),  // 9: touching the edge with a corner
	)
	// The low edge runs from the origin through (560, 480), box 9's corner; box 8's is 0.1 past.
	c := cone{origin: geom.NewVec(500, 500), dir: geom.NewVec(1, 0),
		half: float32(math.Atan2(20, 60)), reach: 100}
	got, _ := askCone(g, c.square(), c, AnyCapability)
	slices.Sort(got)
	if want := []uid.UID64{1, 2, 5, 7, 9}; !slices.Equal(got, want) {
		t.Errorf("a narrow cone found %v, want %v", got, want)
	}

	c.half = 3 * math.Pi / 4
	got, _ = askCone(g, c.square(), c, AnyCapability)
	slices.Sort(got)
	if want := []uid.UID64{1, 2, 3, 4, 5, 7, 8, 9}; !slices.Equal(got, want) {
		t.Errorf("a cone wider than a half-turn found %v, want everything within the radius %v",
			got, want)
	}
}

func TestGrid_QueryConeReportsAPieceAcrossFourCellsOnce(t *testing.T) {
	g := plainGrid(geom.NewAABBAt(geom.NewVec(60, 60), 8, 8))
	c := cone{origin: geom.NewVec(20, 20), dir: geom.NewVec(1, 1), half: 0.3, reach: 200}
	if got, n := askCone(g, c.square(), c, AnyCapability); n != 1 || len(got) != 1 {
		t.Errorf("a box across four cells was reported %d times, want once", n)
	}
}

func TestGrid_QueryConeRefusesAConeWithNoRadiusAngleOrOrigin(t *testing.T) {
	g := plainGrid(geom.NewAABBAt(geom.NewVec(495, 495), 10, 10))
	for name, c := range map[string]cone{
		"no radius": {origin: geom.NewVec(500, 500), dir: geom.NewVec(1, 0), half: 1, reach: 0},
		"no angle":  {origin: geom.NewVec(500, 500), dir: geom.NewVec(1, 0), half: 0, reach: 50},
		"NaN angle": {origin: geom.NewVec(500, 500), dir: geom.NewVec(1, 0),
			half: float32(math.NaN()), reach: 50},
		"NaN origin": {origin: geom.NewVec(float32(math.NaN()), 500), dir: geom.NewVec(1, 0),
			half: 1, reach: 50},
	} {
		if _, n := askCone(g, geom.NewAABBAt(geom.Vec{}, 1024, 1024), c, AnyCapability); n != 0 {
			t.Errorf("%s: a degenerate cone found %d pieces", name, n)
		}
	}
}

func TestGrid_QueryConeFollowsABoxMovedSinceRebuild(t *testing.T) {
	surface := iplane.NewEuclidean2D(1024, 1024)
	g := NewGrid(surface, Size1024x1024, Size32x32)
	items := []Item{{ID: 1, Box: plane.NewAABB(geom.NewVec(560, 495), 10, 10)}}
	g.Rebuild(items)
	c := cone{origin: geom.NewVec(500, 500), dir: geom.NewVec(1, 0), half: 0.5, reach: 100}
	if _, n := askCone(g, c.square(), c, AnyCapability); n != 1 {
		t.Fatalf("the cone finds %d pieces ahead of it, want 1", n)
	}
	for _, step := range []geom.Vec{geom.NewVec(-120, 0), geom.NewVec(120, 0)} {
		surface.Translate(&items[0].Box, step)
		g.Invalidate()
		want := 0
		if items[0].Box.TopLeft.X > 500 {
			want = 1
		}
		if _, n := askCone(g, c.square(), c, AnyCapability); n != want {
			t.Errorf("box moved to %v: the cone finds %d pieces, want %d", items[0].Box, n, want)
		}
	}
}
