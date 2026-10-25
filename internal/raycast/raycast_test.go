package raycast_test

import (
	"math"
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
	// twice makes Query report every entity again; phantom is reported but has no indexed box.
	twice   bool
	phantom uid.UID64
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
