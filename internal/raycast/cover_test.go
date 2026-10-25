package raycast_test

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/internal/raycast"
	"github.com/kjkrol/uid"
)

// coverCell is what stands on one cell: how see-through it is and the band it spans.
type coverCell struct {
	tau         float32
	bottom, top float32
}

// gridCover is ground cover on a grid of square cells, walked exactly cell by cell.
type gridCover struct {
	cell  float32
	cells map[[2]int]coverCell
}

func newCover(cell float32) *gridCover { return &gridCover{cell: cell, cells: map[[2]int]coverCell{}} }

func (g *gridCover) set(x, y int, c coverCell) { g.cells[[2]int{x, y}] = c }

func (g *gridCover) Walk(origin, dir geom.Vec, length float32, visit func(near, far, bottom, top, tau float32) bool) {
	ox, oy, cell, end := float64(origin.X), float64(origin.Y), float64(g.cell), float64(length)
	cx, cy := int(math.Floor(ox/cell)), int(math.Floor(oy/cell))
	axis := func(o, d float64, c int) (step int, next, delta float64) {
		switch {
		case d > 0:
			return 1, (float64(c+1)*cell - o) / d, cell / d
		case d < 0:
			return -1, (float64(c)*cell - o) / d, -cell / d
		}
		return 0, math.Inf(1), math.Inf(1)
	}
	sx, nx, dx := axis(ox, float64(dir.X), cx)
	sy, ny, dy := axis(oy, float64(dir.Y), cy)
	for t := 0.0; t < end; {
		exit := math.Min(math.Min(nx, ny), end)
		if c, ok := g.cells[[2]int{cx, cy}]; ok && !visit(float32(t), float32(exit), c.bottom, c.top, c.tau) {
			return
		}
		if nx < ny {
			cx, t, nx = cx+sx, nx, nx+dx
		} else {
			cy, t, ny = cy+sy, ny, ny+dy
		}
	}
}

// asEntities puts every cell of cover into a copy of s as an entity of its own, with the same
// transparency and band, for the oracle: the scan over entities is exact.
func (g *gridCover) asEntities(s *fakeSpace) (*fakeSpace, map[uid.UID64]float32, map[uid.UID64]band) {
	out := newFake(s.w, s.h, s.toroidal)
	for id, b := range s.boxes {
		out.boxes[id] = b
	}
	taus, bands := map[uid.UID64]float32{}, map[uid.UID64]band{}
	id := uid.UID64(10000)
	for k, c := range g.cells {
		out.put(id, float32(k[0])*g.cell, float32(k[1])*g.cell, g.cell, g.cell)
		taus[id], bands[id] = c.tau, band{c.bottom, c.top}
		id++
	}
	return out, taus, bands
}

// withBands is elevated with the bands of cells and the walkers standing 0 to 2.
func withBands(c raycast.Cone, eye float32, cells, walkers map[uid.UID64]band) raycast.Cone {
	all := map[uid.UID64]band{}
	for id, b := range cells {
		all[id] = b
	}
	for id, b := range walkers {
		all[id] = b
	}
	return elevated(c, eye, all)
}

func depthsOf(t *testing.T, s raycast.QueryableSpace, c raycast.Cone, k int) []float32 {
	t.Helper()
	return scan(t, s, eye, c).Depths(k, nil)
}

func sameDepths(t *testing.T, what string, got, want []float32) {
	t.Helper()
	for i := range want {
		if math.Abs(float64(got[i]-want[i])) > 1e-3 {
			t.Errorf("%s: depth %d = %.4f through cover, %.4f through the same cells as entities", what, i, got[i], want[i])
		}
	}
}

func TestCover_AWallOfCellsHidesWhatIsBehindIt(t *testing.T) {
	s := newFake(2000, 2000, false)
	s.put(eye, 100, 100, 10, 10)
	s.put(far, 505, 100, 10, 10)
	cover := newCover(10)
	for y := 5; y < 16; y++ {
		cover.set(30, y, coverCell{tau: 0, bottom: float32(math.Inf(-1)), top: float32(math.Inf(1))})
	}
	c := eastward(math.Pi/8, 600)
	c.Cover = cover
	assertVisible(t, visible(t, s, eye, c))
	if d := ahead(t, s, eye, c); math.Abs(d-195) > 1e-6 {
		t.Errorf("reach ahead = %.3f, want 195, the face of the wall", d)
	}
}

func TestCover_AForestOfCellsChargesItsDepthAgainstTheReach(t *testing.T) {
	s := newFake(2000, 2000, false)
	s.put(eye, 100, 100, 10, 10)
	s.put(far, 505, 100, 10, 10) // 400 ahead: 100 + 200 + 200 of the budget through a τ 0.5 forest
	cover := newCover(10)
	for x := 21; x < 31; x++ { // 205 to 305 ahead of the eye's centre... cells 210 to 310
		for y := 5; y < 16; y++ {
			cover.set(x, y, coverCell{tau: 0.5, bottom: float32(math.Inf(-1)), top: float32(math.Inf(1))})
		}
	}
	for _, radius := range []float32{450, 550} {
		c := eastward(math.Pi/8, radius)
		c.Cover = cover
		entities, taus, _ := cover.asEntities(s)
		want := visible(t, entities, eye, seeThrough(eastward(math.Pi/8, radius), taus))
		got := visible(t, s, eye, c)
		if got[far] != want[far] {
			t.Errorf("radius %v: the target seen %v through cover, %v through the same cells as entities", radius, got[far], want[far])
		}
	}
}

func TestCover_TheEyeInsideAForestPaysFromWhereItStands(t *testing.T) {
	s := newFake(2000, 2000, false)
	s.put(eye, 100, 100, 10, 10)
	cover := newCover(10)
	for x := 5; x < 35; x++ {
		for y := 5; y < 16; y++ {
			cover.set(x, y, coverCell{tau: 0.5, bottom: float32(math.Inf(-1)), top: float32(math.Inf(1))})
		}
	}
	c := eastward(math.Pi/8, 500)
	c.Cover = cover
	entities, taus, _ := cover.asEntities(s)
	sameDepths(t, "eye in a forest", depthsOf(t, s, c, 9), depthsOf(t, entities, seeThrough(eastward(math.Pi/8, 500), taus), 9))
}

// coverScene scatters cells of cover — walls and forests — and a few walkers round an eye.
func coverScene(r *rand.Rand, heights bool) (*fakeSpace, *gridCover, map[uid.UID64]band) {
	s := newFake(4000, 4000, false)
	s.put(eye, 2000, 2000, 10, 10)
	cover := newCover(20)
	for x := 80; x < 120; x++ {
		for y := 80; y < 120; y++ {
			if (x == 100 || x == 99) && (y == 100 || y == 99) || r.Float64() > 0.3 {
				continue
			}
			c := coverCell{bottom: float32(math.Inf(-1)), top: float32(math.Inf(1))}
			if r.IntN(2) == 0 {
				c.tau = float32(0.25 + 0.5*r.Float64())
			}
			if heights {
				c.bottom, c.top = 0, float32(2+10*r.Float64())
			}
			cover.set(x, y, c)
		}
	}
	walkers := map[uid.UID64]band{}
	for i := range 12 {
		id := uid.UID64(100 + i)
		s.put(id, 1700+float32(r.IntN(600)), 1700+float32(r.IntN(600)), 8, 8)
		walkers[id] = band{0, 2}
	}
	return s, cover, walkers
}

func TestCover_DepthsMatchTheSameCellsAsEntities(t *testing.T) {
	r := rand.New(rand.NewPCG(31, 37))
	for trial := range 8 {
		s, cover, _ := coverScene(r, false)
		dir := float64(trial)
		base := raycast.Cone{Direction: geom.NewVec(float32(math.Cos(dir)), float32(math.Sin(dir))), HalfAngle: math.Pi / 4, Radius: 300}
		c := base
		c.Cover = cover
		entities, taus, _ := cover.asEntities(s)
		sameDepths(t, "flat", depthsOf(t, s, c, 33), depthsOf(t, entities, seeThrough(base, taus), 33))
	}
}

func TestCover_SeesTheWalkersTheSameCellsAsEntitiesLetBeSeen(t *testing.T) {
	r := rand.New(rand.NewPCG(41, 43))
	for trial := range 8 {
		s, cover, _ := coverScene(r, false)
		dir := float64(trial)
		base := raycast.Cone{Direction: geom.NewVec(float32(math.Cos(dir)), float32(math.Sin(dir))), HalfAngle: math.Pi / 3, Radius: 300}
		c := base
		c.Cover = cover
		entities, taus, _ := cover.asEntities(s)
		exact := visible(t, entities, eye, seeThrough(base, taus))
		got := visible(t, s, eye, c)
		for id := range got {
			if !exact[id] {
				t.Errorf("trial %d: walker %d seen through cover, hidden by the same cells as entities", trial, id)
			}
		}
		// What the entities' scan sees, a dense fan of casts through the cover must reach too.
		boxes := map[uid.UID64][4]float64{}
		for id, b := range entities.boxes {
			boxes[id] = [4]float64{float64(b.TopLeft.X), float64(b.TopLeft.Y), float64(b.BottomRight.X), float64(b.BottomRight.Y)}
		}
		fan := bruteForce(boxes, taus, geom.NewVec(2005, 2005), c, 4000, math.Pi/90)
		for id := range fan {
			if id >= 100 && id < 200 && !got[id] {
				t.Errorf("trial %d: rays reach walker %d, but the scan through cover does not report it", trial, id)
			}
		}
	}
}

func TestCover_WithHeightsDepthsAndShadowsMatchTheSameCellsAsEntities(t *testing.T) {
	r := rand.New(rand.NewPCG(47, 53))
	for trial := range 6 {
		s, cover, walkers := coverScene(r, true)
		dir := float64(trial)
		for _, eyeAt := range []float32{1.5, 30} {
			base := raycast.Cone{Direction: geom.NewVec(float32(math.Cos(dir)), float32(math.Sin(dir))), HalfAngle: math.Pi / 4, Radius: 300, GroundStep: 10}
			entities, taus, bands := cover.asEntities(s)
			want := seeThrough(withBands(base, eyeAt, bands, walkers), taus)
			c := withBands(base, eyeAt, nil, walkers)
			c.Cover = cover
			sameDepths(t, "with heights", depthsOf(t, s, c, 33), depthsOf(t, entities, want, 33))
			got, exp := scan(t, s, eye, c).Shadows(33, nil), scan(t, entities, eye, want).Shadows(33, nil)
			if len(got) != len(exp) {
				t.Fatalf("trial %d eye %v: %d shadows through cover, %d through the same cells as entities", trial, eyeAt, len(got), len(exp))
			}
			for i := range exp {
				if got[i].Sample != exp[i].Sample || !near32(got[i].From, exp[i].From) || !near32(got[i].To, exp[i].To) {
					t.Errorf("trial %d eye %v: shadow %d = %+v, want %+v", trial, eyeAt, i, got[i], exp[i])
				}
			}
		}
	}
}

// From the edge of a cliff the lowland below is in sight, but not what a wall down there hides.
func TestCover_FromTheCliffAWallBelowStillCastsItsShadow(t *testing.T) {
	s := newFake(2000, 2000, false)
	s.put(eye, 390, 100, 10, 10) // at the plateau's east edge
	s.put(walker, 605, 100, 10, 10)
	cover := newCover(10)
	for y := 5; y < 16; y++ {
		cover.set(50, y, coverCell{tau: 0, bottom: 0, top: 15}) // a wall 100 past the edge, 15 tall
	}
	walkers := map[uid.UID64]band{walker: {0, 2}}
	base := eastward(math.Pi/8, 500)
	base.Ground, base.GroundStep = plateau, 25
	entities, taus, bands := cover.asEntities(s)
	want := seeThrough(withBands(base, 21.5, bands, walkers), taus)
	c := withBands(base, 21.5, nil, walkers)
	c.Cover = cover

	if got, exp := visible(t, s, eye, c), visible(t, entities, eye, want); got[walker] != exp[walker] {
		t.Errorf("the walker behind the wall seen %v from the cliff through cover, %v through the wall as entities", got[walker], exp[walker])
	}
	got, exp := scan(t, s, eye, c).Shadows(9, nil), scan(t, entities, eye, want).Shadows(9, nil)
	if len(got) == 0 || len(got) != len(exp) {
		t.Fatalf("shadows %+v through cover, %+v as entities; want the wall's shadow in both", got, exp)
	}
	for i := range exp {
		if got[i] != exp[i] && (!near32(got[i].From, exp[i].From) || !near32(got[i].To, exp[i].To)) {
			t.Errorf("shadow %d = %+v, want %+v", i, got[i], exp[i])
		}
	}
}

func TestCover_ScanDoesNotAllocateOnceWarm(t *testing.T) {
	for name, heights := range map[string]bool{"flat": false, "with heights": true} {
		t.Run(name, func(t *testing.T) {
			s, cover, walkers := coverScene(rand.New(rand.NewPCG(59, 61)), heights)
			c := raycast.Cone{Direction: geom.NewVec(1, 0), HalfAngle: math.Pi / 3, Radius: 300, Cover: cover}
			if heights {
				c = withBands(c, 1.5, nil, walkers)
				c.Cover = cover
			}
			v := &raycast.View{}
			v.Scan(s, eye, c)
			depths := make([]float32, 0, 64)
			allocs := testing.AllocsPerRun(20, func() {
				v.Scan(s, eye, c)
				depths = v.Depths(33, depths[:0])
			})
			if allocs > 0 {
				t.Errorf("a warm Scan and Depths through cover allocate %.0f times per run, want none", allocs)
			}
		})
	}
}
