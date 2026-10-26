package raycast_test

import (
	"fmt"
	"maps"
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/internal/raycast"
	"github.com/kjkrol/uid"
)

// fakeSpace is a QueryableSpace over a plain slice — enough to pin the
// visibility rules without building a real index.
type fakeSpace struct {
	boxes    map[uid.UID64]geom.AABB
	w, h     float64
	toroidal bool
	// twice makes a query report every entity again; phantom is reported but has no indexed box.
	twice   bool
	phantom uid.UID64
	// loose makes QueryCone answer with the square round the cone's circle, as a scan once asked.
	loose bool
}

func newFake(w, h float64, toroidal bool) *fakeSpace {
	return &fakeSpace{boxes: map[uid.UID64]geom.AABB{}, w: w, h: h, toroidal: toroidal}
}

func (f *fakeSpace) put(id uid.UID64, x, y, w, h float32) {
	f.boxes[id] = geom.NewAABBAt(geom.NewVec(x, y), w, h)
}

func (f *fakeSpace) Bounds() (uint32, uint32, bool, bool) {
	return uint32(f.w), uint32(f.h), f.toroidal, f.toroidal
}

func (f *fakeSpace) EntryAABB(id uid.UID64) (geom.AABB, bool) {
	b, ok := f.boxes[id]
	return b, ok
}

func (f *fakeSpace) Query(area geom.AABB, fn func(uid.UID64)) int {
	n := 0
	for id, b := range f.boxes {
		if !area.Intersects(b) {
			continue
		}
		fn(id)
		n++
		if f.twice {
			fn(id)
			n++
		}
	}
	if f.phantom != 0 {
		fn(f.phantom)
		n++
	}
	return n
}

// QueryCone reports every box some copy of which, a world either way in a toroidal world, lies
// within radius of origin and, for a cone narrower than a half-turn, wholly beyond neither edge.
func (f *fakeSpace) QueryCone(
	origin, dir geom.Vec, halfAngle, radius float32, fn func(uid.UID64),
) int {
	ox, oy, r := float64(origin.X), float64(origin.Y), float64(radius)
	theta, half := math.Atan2(float64(dir.Y), float64(dir.X)), float64(halfAngle)
	lo := [2]float64{math.Cos(theta - half), math.Sin(theta - half)}
	hi := [2]float64{math.Cos(theta + half), math.Sin(theta + half)}
	reaches := func(b geom.AABB, sx, sy float64) bool {
		x1, y1 := float64(b.TopLeft.X)+sx-ox, float64(b.TopLeft.Y)+sy-oy
		x2, y2 := float64(b.BottomRight.X)+sx-ox, float64(b.BottomRight.Y)+sy-oy
		if f.loose {
			return x1 <= r && x2 >= -r && y1 <= r && y2 >= -r
		}
		if math.Hypot(max(x1, -x2, 0), max(y1, -y2, 0)) > r {
			return false
		}
		if half >= math.Pi/2 {
			return true
		}
		pastLo, pastHi := true, true
		for _, x := range [2]float64{x1, x2} {
			for _, y := range [2]float64{y1, y2} {
				pastLo = pastLo && lo[0]*y-lo[1]*x < 0
				pastHi = pastHi && hi[0]*y-hi[1]*x > 0
			}
		}
		return !pastLo && !pastHi
	}
	xs, ys := [3]float64{0, -f.w, f.w}, [3]float64{0, -f.h, f.h}
	n := 1
	if f.toroidal {
		n = 3
	}
	found := 0
	for id, b := range f.boxes {
		hit := false
		for _, sx := range xs[:n] {
			for _, sy := range ys[:n] {
				hit = hit || reaches(b, sx, sy)
			}
		}
		if !hit {
			continue
		}
		fn(id)
		found++
		if f.twice {
			fn(id)
			found++
		}
	}
	if f.phantom != 0 {
		fn(f.phantom)
		found++
	}
	return found
}

// eastward looks along +X with the given half-angle and reach.
func eastward(halfAngle, radius float32) raycast.Cone {
	return raycast.Cone{Direction: geom.NewVec(1.0, 0.0), HalfAngle: halfAngle, Radius: radius}
}

// scan answers through a fresh View, which is how a test looks at one query.
func scan(t *testing.T, s raycast.QueryableSpace, observer uid.UID64, c raycast.Cone) *raycast.View {
	t.Helper()
	v := &raycast.View{}
	v.Scan(s, observer, c)
	return v
}

func visible(t *testing.T, s raycast.QueryableSpace, observer uid.UID64, c raycast.Cone) map[uid.UID64]bool {
	t.Helper()
	got := map[uid.UID64]bool{}
	n := scan(t, s, observer, c).Entities(func(id uid.UID64, _ float32) { got[id] = true })
	if n != len(got) {
		t.Errorf("Entities returned %d but reported %d distinct ids", n, len(got))
	}
	return got
}

func outline(t *testing.T, s raycast.QueryableSpace, observer uid.UID64, c raycast.Cone, step float32) []geom.Vec {
	t.Helper()
	return scan(t, s, observer, c).Outline(step, nil)
}

func assertVisible(t *testing.T, got map[uid.UID64]bool, want ...uid.UID64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("visible = %v, want exactly %v", keys(got), want)
	}
	for _, id := range want {
		if !got[id] {
			t.Errorf("entity %v missing from %v", id, keys(got))
		}
	}
}

func keys(m map[uid.UID64]bool) []uid.UID64 {
	out := make([]uid.UID64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

const (
	eye  = uid.UID64(1)
	near = uid.UID64(2)
	far  = uid.UID64(3)
	tail = uid.UID64(4)
)

func TestVisible_NearestOfAColumnHidesTheRest(t *testing.T) {
	s := newFake(1000, 1000, false)
	s.put(eye, 100, 100, 10, 10)
	s.put(near, 200, 100, 10, 10)
	s.put(far, 300, 100, 10, 10)
	s.put(tail, 400, 100, 10, 10)

	assertVisible(t, visible(t, s, eye, eastward(math.Pi/4, 500)), near)
}

func TestVisible_TargetPeekingPastTheBlockerIsSeen(t *testing.T) {
	s := newFake(1000, 1000, false)
	s.put(eye, 100, 100, 10, 10)
	s.put(near, 200, 100, 10, 10)
	s.put(far, 300, 60, 10, 100)

	assertVisible(t, visible(t, s, eye, eastward(math.Pi/4, 500)), near, far)
}

func TestVisible_SideBySideEntitiesDoNotHideEachOther(t *testing.T) {
	s := newFake(1000, 1000, false)
	s.put(eye, 100, 100, 10, 10)
	s.put(near, 300, 60, 10, 10)
	s.put(far, 300, 140, 10, 10)

	assertVisible(t, visible(t, s, eye, eastward(math.Pi/3, 500)), near, far)
}

func TestVisible_ExcludesOutOfRangeOutOfConeAndSelf(t *testing.T) {
	cases := map[string]struct {
		x, y float32
		want []uid.UID64
	}{
		"in cone and in range":               {700, 500, []uid.UID64{near}},
		"inside range, outside the cone":     {500, 300, nil},
		"inside the cone, beyond the radius": {796, 608, nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s := newFake(2000, 2000, false)
			s.put(eye, 500, 500, 10, 10)
			s.put(near, tc.x, tc.y, 10, 10)

			got := visible(t, s, eye, eastward(math.Pi/8, 300))
			assertVisible(t, got, tc.want...)
		})
	}
}

func TestScan_RefusesAnUnknownObserver(t *testing.T) {
	s := newFake(1000, 1000, false)
	s.put(near, 200, 100, 10, 10)

	if (&raycast.View{}).Scan(s, eye, eastward(math.Pi/4, 500)) {
		t.Error("Scan succeeded for an observer that is not indexed")
	}
}

func TestScan_RefusesDegenerateCones(t *testing.T) {
	s := newFake(1000, 1000, false)
	s.put(eye, 100, 100, 10, 10)
	s.put(near, 200, 100, 10, 10)

	cases := map[string]raycast.Cone{
		"zero half-angle": eastward(0, 500),
		"half-angle pi":   eastward(math.Pi, 500),
		"zero radius":     eastward(math.Pi/4, 0),
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if (&raycast.View{}).Scan(s, eye, c) {
				t.Error("Scan succeeded, want a refusal")
			}
		})
	}
}

func TestVisible_ReportsNearestFirst(t *testing.T) {
	s := newFake(1000, 1000, false)
	s.put(eye, 100, 100, 10, 10)
	s.put(near, 300, 60, 10, 10)
	s.put(far, 500, 140, 10, 10)

	var order []float32
	scan(t, s, eye, eastward(math.Pi/3, 900)).Entities(func(_ uid.UID64, d float32) {
		order = append(order, d)
	})
	if len(order) != 2 {
		t.Fatalf("saw %d entities, want 2", len(order))
	}
	if order[0] > order[1] {
		t.Errorf("distances %v are not ascending", order)
	}
}

func TestVisible_SeesAcrossTheSeamInAToroidalWorld(t *testing.T) {
	s := newFake(1000, 1000, true)
	s.put(eye, 970, 500, 10, 10)
	s.put(near, 20, 500, 10, 10)

	got := visible(t, s, eye, eastward(math.Pi/4, 200))
	assertVisible(t, got, near)
}

func TestVisible_ReportsAFragmentedEntityOnce(t *testing.T) {
	s := newFake(1000, 1000, false)
	s.twice = true
	s.put(eye, 100, 100, 10, 10)
	s.put(near, 300, 100, 10, 10)

	assertVisible(t, visible(t, s, eye, eastward(math.Pi/4, 500)), near)
}

func TestVisible_SkipsCandidatesWithoutAnIndexedBox(t *testing.T) {
	s := newFake(1000, 1000, false)
	s.phantom = uid.UID64(99)
	s.put(eye, 100, 100, 10, 10)
	s.put(near, 300, 100, 10, 10)

	assertVisible(t, visible(t, s, eye, eastward(math.Pi/4, 500)), near)
}

func TestVisible_SeesAcrossTheSeamWhenTheObserverSitsAtTheOrigin(t *testing.T) {
	s := newFake(1000, 1000, true)
	s.put(eye, 20, 500, 10, 10)
	s.put(near, 960, 500, 10, 10)

	cone := raycast.Cone{Direction: geom.NewVec(-1.0, 0.0), HalfAngle: math.Pi / 4, Radius: 200}
	assertVisible(t, visible(t, s, eye, cone), near)
}

func TestVisible_SeesATargetInFrontOfAReceedingWall(t *testing.T) {
	s := newFake(2000, 2000, false)
	s.put(eye, 0, 0, 10, 10)
	s.put(near, 20, 20, 980, 10)
	s.put(far, 500, 14, 10, 4)

	assertVisible(t, visible(t, s, eye, eastward(math.Pi/3, 1200)), near, far)
}

func TestVisible_BoxesOnTheSameSightLines(t *testing.T) {
	s := newFake(2000, 2000, false)
	s.put(eye, 100, 100, 10, 10)
	s.put(near, 200, 100, 10, 10)
	s.put(far, 295, 95, 20, 20)

	assertVisible(t, visible(t, s, eye, eastward(math.Pi/4, 900)), near)
}

func TestVisible_WideConeStillExcludesWhatIsBehind(t *testing.T) {
	s := newFake(2000, 2000, false)
	s.put(eye, 1000, 1000, 10, 10)
	s.put(near, 1300, 995, 20, 20)
	s.put(tail, 700, 995, 20, 20)

	var got []uid.UID64
	scan(t, s, eye, eastward(2*math.Pi/3, 600)).Entities(func(id uid.UID64, _ float32) {
		got = append(got, id)
	})

	if len(got) != 1 || got[0] != near {
		t.Errorf("visible = %v, want only the entity ahead (%d)", got, near)
	}
}

// seamScene scatters boxes over a toroidal world with the eye near its corner, so cones cross
// both seams; some reach farther than half the world.
func seamScene(r *rand.Rand) (*fakeSpace, raycast.Cone) {
	s := newFake(1000, 1000, true)
	s.put(eye, 990, 4, 10, 10)
	for i := range 150 {
		s.put(uid.UID64(100+i), float32(r.Float64()*980), float32(r.Float64()*980),
			float32(4+r.Float64()*16), float32(4+r.Float64()*16))
	}
	a := r.Float64() * 2 * math.Pi
	return s, raycast.Cone{
		Direction: geom.NewVec(float32(math.Cos(a)), float32(math.Sin(a))),
		HalfAngle: float32(0.1 + 2.9*r.Float64()),
		Radius:    float32(50 + 650*r.Float64()),
	}
}

// The scan reads the same through the cone's answer as through the square round its circle, as it
// read before the index was asked for the cone: the reach and the hidden ground at every angle,
// and every entity but one seen over less than the sweep resolves. The sweep casts along the
// edges of every box it is handed, so a box out of reach that the square adds can catch a sliver.
func TestScan_ReadsTheSameThroughTheConeAsThroughItsSquare(t *testing.T) {
	type trial struct {
		name string
		s    *fakeSpace
		c    raycast.Cone
		// robust is what a dense fan of rays sees over more than the sweep resolves; nil leaves
		// the entities uncompared.
		robust func() map[uid.UID64]bool
	}
	flat := func(
		s *fakeSpace, c raycast.Cone, taus map[uid.UID64]float32,
	) func() map[uid.UID64]bool {
		return func() map[uid.UID64]bool {
			o, boxes := nearestBoxes(s)
			return bruteForce(boxes, taus, o, c, 20000, 2*math.Atan2(1, float64(c.Radius)))
		}
	}
	var trials []trial
	for seed := range uint64(8) {
		s, c := scene(seed)
		trials = append(trials, trial{fmt.Sprintf("scene %d", seed), s, c, flat(s, c, nil)})
	}
	r := rand.New(rand.NewPCG(61, 67))
	turned := func(halfAngle, radius float32) raycast.Cone {
		dir := r.Float64() * 2 * math.Pi
		return raycast.Cone{
			Direction: geom.NewVec(float32(math.Cos(dir)), float32(math.Sin(dir))),
			HalfAngle: halfAngle,
			Radius:    radius,
		}
	}
	for i := range 12 {
		s, _, taus := translucentScene(r)
		c := turned(float32(0.2+2.8*r.Float64()), float32(200+600*r.Float64()))
		trials = append(trials, trial{fmt.Sprintf("opaque %d", i), s, c, flat(s, c, nil)},
			trial{fmt.Sprintf("translucent %d", i), s, seeThrough(c, taus), flat(s, c, taus)})
	}
	for i := range 8 {
		s, o := elevatedScene(r)
		c := o.cone(r.Float64()*2*math.Pi, float32(0.2+2.8*r.Float64()), 700)
		trials = append(trials, trial{fmt.Sprintf("elevated %d", i), s, c,
			func() map[uid.UID64]bool { return o.robust(c) }})
	}
	for i := range 4 {
		s, cover, walkers := coverScene(r, i%2 == 1)
		c := turned(math.Pi/3, 300)
		c.Cover = cover
		var robust func() map[uid.UID64]bool
		if i%2 == 1 {
			c = withBands(c, 30, nil, walkers)
		} else {
			entities, taus, _ := cover.asEntities(s)
			robust = flat(entities, seeThrough(c, taus), taus)
		}
		trials = append(trials, trial{fmt.Sprintf("cover %d", i), s, c, robust})
	}
	for i := range 16 {
		s, c := seamScene(r)
		trials = append(trials, trial{fmt.Sprintf("seam %d", i), s, c, flat(s, c, nil)})
	}
	for _, tr := range trials {
		tight := scan(t, tr.s, eye, tr.c)
		tr.s.loose = true
		loose := scan(t, tr.s, eye, tr.c)
		tr.s.loose = false
		if got, want := tight.Depths(33, nil), loose.Depths(33, nil); !slices.Equal(got, want) {
			t.Errorf("%s: depths %v through the cone, %v through its square", tr.name, got, want)
		}
		if got, want := tight.Shadows(33, nil), loose.Shadows(33, nil); !slices.Equal(got, want) {
			t.Errorf("%s: shadows %v through the cone, %v through its square", tr.name, got, want)
		}
		got, want := visible(t, tr.s, eye, tr.c), map[uid.UID64]bool{}
		loose.Entities(func(id uid.UID64, _ float32) { want[id] = true })
		if tr.robust == nil || maps.Equal(got, want) {
			continue
		}
		robust := tr.robust()
		for id := range robust {
			if got[id] != want[id] {
				t.Errorf("%s: %v seen through only one of the cone (%v) and its square (%v)",
					tr.name, id, got[id], want[id])
			}
		}
	}
}

// nearestBoxes is the eye of s and every other box at its copy nearest it, as a scan takes them.
func nearestBoxes(s *fakeSpace) (geom.Vec, map[uid.UID64][4]float64) {
	e := s.boxes[eye]
	ox := (float64(e.TopLeft.X) + float64(e.BottomRight.X)) / 2
	oy := (float64(e.TopLeft.Y) + float64(e.BottomRight.Y)) / 2
	shift := func(lo, hi, o, size float64) float64 {
		d := (lo+hi)/2 - o
		if !s.toroidal {
			return 0
		}
		return math.Remainder(d, size) - d
	}
	boxes := map[uid.UID64][4]float64{}
	for id, b := range s.boxes {
		if id == eye {
			continue
		}
		x1, x2 := float64(b.TopLeft.X), float64(b.BottomRight.X)
		y1, y2 := float64(b.TopLeft.Y), float64(b.BottomRight.Y)
		dx, dy := shift(x1, x2, ox, s.w), shift(y1, y2, oy, s.h)
		boxes[id] = [4]float64{x1 + dx, y1 + dy, x2 + dx, y2 + dy}
	}
	return geom.NewVec(float32(ox), float32(oy)), boxes
}

// robust is what a dense fan of rays sees through c over more than the arc the sweep samples by.
func (o elevatedOracle) robust(c raycast.Cone) map[uid.UID64]bool {
	dir := math.Atan2(float64(c.Direction.Y), float64(c.Direction.X))
	half, origin := float64(c.HalfAngle), geom.NewVec(2005, 2005)
	const steps = 4000
	arcs := map[uid.UID64]float64{}
	for i := 0; i <= steps; i++ {
		_, seen := o.cast(origin, dir-half+2*half*float64(i)/steps, float64(c.Radius))
		for id := range seen {
			arcs[id] += 2 * half / steps
		}
	}
	robust := map[uid.UID64]bool{}
	for id, a := range arcs {
		robust[id] = a > math.Pi/90
	}
	return robust
}
