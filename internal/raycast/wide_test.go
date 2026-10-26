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

// sight is what a test compares of one scan: the entities seen, the samples of the sweep, the
// reach at 33 and at 63 angles, and the shadows at 33.
type sight struct {
	seen     map[uid.UID64]float32
	sweep    []raycast.Reach
	depths33 []raycast.Reach
	depths63 []raycast.Reach
	shadows  []raycast.Shadow
}

func sightOf(t *testing.T, s raycast.QueryableSpace, c raycast.Cone) sight {
	t.Helper()
	v := scan(t, s, eye, c)
	got := sight{seen: map[uid.UID64]float32{}, sweep: v.Reaches()}
	v.Entities(func(id uid.UID64, d float32) { got.seen[id] = d })
	v.Depths(33, nil)
	got.depths33 = v.DepthReaches()
	v.Depths(63, nil)
	got.depths63 = v.DepthReaches()
	got.shadows = v.Shadows(33, nil)
	return got
}

// sameReaches checks two runs of samples meet the same at the same angles, as far.
func sameReaches(t *testing.T, what string, narrow, wide []raycast.Reach) {
	t.Helper()
	if len(wide) != len(narrow) {
		t.Fatalf("%s: %d samples wide, %d narrow", what, len(wide), len(narrow))
	}
	for i, n := range narrow {
		if wide[i] != n {
			t.Errorf("%s: sample %d wide %+v, narrow %+v", what, i, wide[i], n)
		}
	}
}

func sameSight(t *testing.T, what string, narrow, wide sight) {
	t.Helper()
	if !maps.Equal(wide.seen, narrow.seen) {
		t.Errorf("%s: seen wide %v, narrow %v", what, wide.seen, narrow.seen)
	}
	sameReaches(t, what+", sweep", narrow.sweep, wide.sweep)
	sameReaches(t, what+", 33 depths", narrow.depths33, wide.depths33)
	sameReaches(t, what+", 63 depths", narrow.depths63, wide.depths63)
	if !slices.Equal(wide.shadows, narrow.shadows) {
		t.Errorf("%s: shadows wide %v, narrow %v", what, wide.shadows, narrow.shadows)
	}
}

// compareWide scans s through c with the slabs worked out as each ray is cast and laid four angles
// at a time, and checks both see the same.
func compareWide(t *testing.T, what string, s raycast.QueryableSpace, c raycast.Cone) {
	t.Helper()
	raycast.SetWide(t, false)
	narrow := sightOf(t, s, c)
	raycast.SetWide(t, true)
	sameSight(t, what, narrow, sightOf(t, s, c))
}

func TestScan_WideSeesWhatNarrowSees(t *testing.T) {
	r := rand.New(rand.NewPCG(61, 67))
	look := func(trial int) geom.Vec {
		if trial == 0 {
			return geom.NewVec(1, 0) // along x, and so the middle ray of 33
		}
		a := r.Float64() * 2 * math.Pi
		return geom.NewVec(float32(math.Cos(a)), float32(math.Sin(a)))
	}
	for trial := range 12 {
		half := float32(0.3 + r.Float64()*1.2)
		radius := float32(300 + r.IntN(500))

		s, _ := scene(uint64(trial))
		c := raycast.Cone{Direction: look(trial), HalfAngle: half, Radius: radius}
		compareWide(t, "opaque", s, c)

		ts, _, taus := translucentScene(r)
		c = raycast.Cone{Direction: look(trial), HalfAngle: half, Radius: radius}
		compareWide(t, "see-through", ts, seeThrough(c, taus))

		es, o := elevatedScene(r)
		dir := float64(trial)
		compareWide(t, "heights", es, o.cone(dir, half, radius))

		rs, ro, _ := ridgeScene(r)
		compareWide(t, "profile", rs, ro.profiled(dir, half, radius))

		cs, cover, _ := coverScene(r, false)
		c = raycast.Cone{Direction: look(trial), HalfAngle: half, Radius: 300}
		c.Cover = cover
		compareWide(t, "cover", cs, c)

		hs, hcover, walkers := coverScene(r, true)
		hc := raycast.Cone{Direction: look(trial), HalfAngle: half, Radius: 300, GroundStep: 10}
		hc = withBands(hc, 6, nil, walkers)
		hc.Cover = hcover
		compareWide(t, "cover with heights", hs, hc)
	}
	ahead := seeThrough(eastward(math.Pi/4, 400), map[uid.UID64]float32{103: 0.5})
	compareWide(t, "edges along x", edgeScene(), ahead)
	ahead.Direction = geom.NewVec(1, 1)
	compareWide(t, "edges along the diagonal", edgeScene(), ahead)
}

// Over a field the wide scan walks the ground of its rays side by side and sees what the narrow
// one sees: over a field that ends, wraps along one axis or both, flat or bent, the last block of
// a walk one to four rays long.
func TestScan_WideSeesWhatNarrowSeesOverAField(t *testing.T) {
	r := rand.New(rand.NewPCG(151, 157))
	for trial := range 24 {
		wrapX, wrapY := trial%3 == 1, trial%4 == 2
		bend := float32(0)
		if trial%2 == 0 {
			bend = 1.0 / 16000
		}
		s, o, bands, taus := fieldScene(r, wrapX, wrapY, bend)
		dir := 2 * math.Pi * r.Float64()
		cone := seeThrough(raycast.Cone{
			Direction: geom.NewVec(float32(math.Cos(dir)), float32(math.Sin(dir))),
			HalfAngle: float32(math.Pi/8 + r.Float64()*math.Pi/3),
			Radius:    float32(200 + r.IntN(400)),
		}, taus)
		cone = elevated(cone, bands[eye][0], bands)
		cone.Heights, cone.Bend = o.f, bend
		what := fmt.Sprintf("trial %d (wrap %v, %v, bend %v)", trial, wrapX, wrapY, bend)
		compareWide(t, what, s, cone)
		for k := 5; k <= 8; k++ {
			raycast.SetWide(t, false)
			narrow := scan(t, s, eye, cone).Depths(k, nil)
			raycast.SetWide(t, true)
			if wide := scan(t, s, eye, cone).Depths(k, nil); !slices.Equal(wide, narrow) {
				t.Errorf("%s: the reach at %d angles wide %v, narrow %v", what, k, wide, narrow)
			}
		}
	}
}

// edgeScene is an eye at (2005, 2005) among boxes whose edges and corners lie on its own lines:
// on the line along x, touching the diagonal at a corner, and one round the eye.
func edgeScene() *fakeSpace {
	s := newFake(4000, 4000, false)
	s.put(eye, 2000, 2000, 10, 10)
	s.put(100, 2100, 2005, 20, 25) // its top edge on the line along x
	s.put(101, 2200, 1980, 20, 25) // its bottom edge on it
	s.put(102, 2105, 2085, 20, 20) // its corner on the diagonal
	s.put(103, 1995, 1995, 30, 30) // round the eye
	return s
}

func TestScan_MeetsTheEdgesOnTheEyesLines(t *testing.T) {
	taus := map[uid.UID64]float32{103: 0.5}
	ahead := seeThrough(eastward(math.Pi/4, 400), taus)
	// The middle ray of 33 runs from the eye along x and along the top edge of the first box, which
	// it meets at 95, behind the box round the eye that costs it 40 of its 400.
	depths := scan(t, edgeScene(), eye, ahead).Depths(33, nil)
	if got := depths[16]; math.Abs(float64(got)-95) > 1e-3 {
		t.Errorf("along x the reach is %v, want 95, the box whose edge the ray runs along", got)
	}
	diagonal := ahead
	diagonal.Direction = geom.NewVec(1, 1)
	// Along the diagonal the ray touches the third box at its corner, 100·√2 away.
	depths = scan(t, edgeScene(), eye, diagonal).Depths(33, nil)
	if got, want := float64(depths[16]), 100*math.Sqrt2; math.Abs(got-want) > 1e-3 {
		t.Errorf("along the diagonal the reach is %v, want %v at the corner of the box", got, want)
	}
}
