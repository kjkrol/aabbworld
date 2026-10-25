package raycast_test

import (
	"maps"
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/internal/raycast"
	"github.com/kjkrol/uid"
)

// band is an entity's bottom and top.
type band [2]float32

// elevated gives the cone an eye height and every entity in bands its heights; the rest stand
// from 0 to 2, like a walker.
func elevated(c raycast.Cone, eye float32, bands map[uid.UID64]band) raycast.Cone {
	c.Eye = eye
	c.Elevation = func(id uid.UID64) (float32, float32) {
		if b, ok := bands[id]; ok {
			return b[0], b[1]
		}
		return 0, 2
	}
	return c
}

// ahead is the depth straight ahead of a scan with three samples.
func ahead(t *testing.T, s raycast.QueryableSpace, observer uid.UID64, c raycast.Cone) float64 {
	t.Helper()
	return float64(scan(t, s, observer, c).Depths(3, nil)[1])
}

const (
	wall   = uid.UID64(20)
	hawk   = uid.UID64(21)
	tower  = uid.UID64(22)
	walker = uid.UID64(23)
	coin   = uid.UID64(24)
)

// wallScene is an eye at x 100 looking east at a wall 200 ahead, 10 deep, and a walker 400 ahead,
// a little off the axis so that it shades no ground straight ahead.
func wallScene() *fakeSpace {
	s := newFake(2000, 2000, false)
	s.put(eye, 100, 100, 10, 10)
	s.put(wall, 305, 55, 10, 100)
	s.put(walker, 505, 140, 10, 10)
	return s
}

func TestElevation_AWallLowerThanTheEyeIsSeenAndCutsNothing(t *testing.T) {
	s := wallScene()
	cone := elevated(eastward(math.Pi/8, 500), 10, map[uid.UID64]band{wall: {0, 3}})

	assertVisible(t, visible(t, s, eye, cone), wall, walker)
	if d := ahead(t, s, eye, cone); d != 500 {
		t.Errorf("reach ahead = %.3f, want the full 500 over a low wall", d)
	}
}

func TestElevation_AWallHigherThanTheEyeCutsTheGroundAndHidesTheWalker(t *testing.T) {
	s := wallScene()
	s.put(tower, 505, 200, 10, 10) // beside the walker, 40 tall: its top clears the wall
	cone := elevated(eastward(math.Pi/8, 500), 1.5, map[uid.UID64]band{wall: {0, 10}, tower: {0, 40}})

	assertVisible(t, visible(t, s, eye, cone), wall, tower)
	if d := ahead(t, s, eye, cone); d != 200 {
		t.Errorf("reach ahead = %.3f, want 200, the foot of the wall", d)
	}
	pts := outline(t, s, eye, cone, 0)
	shortest := math.Inf(1)
	for i := 1; i < len(pts); i++ {
		shortest = math.Min(shortest, reach(pts, i))
	}
	if shortest > 200.05 { // the nearest sample is a shade off the axis
		t.Errorf("shortest outline reach %.3f, want it pulled in to the wall", shortest)
	}
}

// The line from an eye at 10 over a wall topping at 3, 210 out, meets the ground at 300.
func TestElevation_TheShadowOfALowWallEndsWhereTheSightlineMeetsTheGround(t *testing.T) {
	for _, tc := range []struct {
		name string
		at   float32
		seen bool
	}{
		{"just inside the shadow", 290, false},
		{"just past it", 310, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := wallScene()
			s.put(coin, 105+tc.at, 105, 1, 1)
			cone := elevated(eastward(math.Pi/8, 500), 10, map[uid.UID64]band{wall: {0, 3}, coin: {0, 0}})
			if got := visible(t, s, eye, cone)[coin]; got != tc.seen {
				t.Errorf("coin at %.0f seen = %v, want %v", tc.at, got, tc.seen)
			}
		})
	}
}

func TestElevation_AHawkAboveShadesNoGroundAndHidesNoWalker(t *testing.T) {
	s := wallScene()
	cone := elevated(eastward(math.Pi/8, 500), 1.5, map[uid.UID64]band{wall: {40, 42}}) // the "wall" flies

	assertVisible(t, visible(t, s, eye, cone), wall, walker)
	if d := ahead(t, s, eye, cone); d != 500 {
		t.Errorf("reach ahead = %.3f, want the full 500 under a hawk", d)
	}
}

func TestElevation_TwoHawksAtOneHeightHideEachOther(t *testing.T) {
	s := wallScene()
	s.put(hawk, 405, 100, 10, 10)
	cone := elevated(eastward(math.Pi/8, 500), 41, map[uid.UID64]band{wall: {40, 42}, hawk: {40, 42}})

	assertVisible(t, visible(t, s, eye, cone), wall, walker) // the walker under both is seen, the far hawk not
}

// A hawk at 100 looks down over a forest 8 tall at τ = 0.5 at a coin 400 ahead: its sightline
// never enters the forest, so the coin costs it 400; a walker at 1.5 looks through the whole
// forest and pays 200 more.
func TestElevation_AForestUnderTheHawkDoesNotDimItsSight(t *testing.T) {
	s := newFake(2000, 2000, false)
	s.put(eye, 100, 100, 10, 10)
	s.put(forest, 355, 55, 100, 100)
	s.put(coin, 505, 105, 1, 1)
	bands := map[uid.UID64]band{forest: {0, 8}, coin: {0, 0.5}}
	taus := map[uid.UID64]float32{forest: 0.5}

	assertVisible(t, visible(t, s, eye, seeThrough(elevated(eastward(math.Pi/8, 401), 100, bands), taus)), forest, coin)
	assertVisible(t, visible(t, s, eye, seeThrough(elevated(eastward(math.Pi/8, 401), 1.5, bands), taus)), forest)
	assertVisible(t, visible(t, s, eye, seeThrough(elevated(eastward(math.Pi/8, 501), 1.5, bands), taus)), forest, coin)
}

// plateau is ground 20 high for x in [300, 400), 0 elsewhere.
func plateau(p geom.Vec) float32 {
	if p.X >= 300 && p.X < 400 {
		return 20
	}
	return 0
}

func TestElevation_AHillHidesWhatIsOnItAndBehindItFromTheLowland(t *testing.T) {
	s := newFake(2000, 2000, false)
	s.put(eye, 100, 100, 10, 10)
	s.put(tower, 345, 140, 10, 10)  // on the plateau, 245 ahead, off the axis
	s.put(walker, 445, 100, 10, 10) // behind it, 345 ahead
	bands := map[uid.UID64]band{tower: {20, 22}, walker: {0, 2}}
	cone := elevated(eastward(math.Pi/8, 500), 1.5, bands)
	cone.Ground, cone.GroundStep = plateau, 25

	assertVisible(t, visible(t, s, eye, cone))
	if d := ahead(t, s, eye, cone); d != 200 {
		t.Errorf("reach ahead = %.3f, want 200, the first ground sample on the crest", d)
	}

	cone.Eye = 30 // a hawk from the same spot sees onto the plateau, not behind it
	assertVisible(t, visible(t, s, eye, cone), tower)
	if d := ahead(t, s, eye, cone); d != 275 {
		t.Errorf("reach ahead from 30 up = %.3f, want 275, the last ground sample on the plateau", d)
	}
}

func TestElevation_FromTheHillTheLowlandIsInSight(t *testing.T) {
	s := newFake(2000, 2000, false)
	s.put(eye, 390, 100, 10, 10) // at the plateau's east edge, its centre 5 short of it
	s.put(walker, 505, 100, 10, 10)
	cone := elevated(eastward(math.Pi/8, 500), 21.5, map[uid.UID64]band{walker: {0, 2}})
	cone.Ground, cone.GroundStep = plateau, 25

	assertVisible(t, visible(t, s, eye, cone), walker)
	if d := ahead(t, s, eye, cone); d != 500 {
		t.Errorf("reach ahead = %.3f, want the full 500 down from the edge", d)
	}
}

func TestElevation_GroundIsSampledOnTheWrappedWorld(t *testing.T) {
	s := newFake(1000, 1000, true)
	s.put(eye, 900, 100, 10, 10)
	s.put(walker, 150, 100, 10, 10) // 250 ahead across the seam
	sampled := map[bool]int{}
	cone := elevated(eastward(math.Pi/8, 400), 1.5, nil)
	cone.Ground = func(p geom.Vec) float32 {
		sampled[p.X >= 0 && p.X < 1000] += 1
		return 0
	}

	assertVisible(t, visible(t, s, eye, cone), walker)
	if sampled[false] != 0 || sampled[true] == 0 {
		t.Errorf("ground sampled %d times inside the world and %d outside", sampled[true], sampled[false])
	}
}

func TestElevation_WithoutHeightsTheEyeChangesNothing(t *testing.T) {
	s := forestScene()
	taus := map[uid.UID64]float32{forest: 0.5}
	flat := seeThrough(eastward(math.Pi/8, 501), taus)
	raised := flat
	raised.Eye = 100

	if a, b := visible(t, s, eye, flat), visible(t, s, eye, raised); len(a) != len(b) {
		t.Errorf("Eye alone changed what is seen: %v vs %v", keys(a), keys(b))
	}
	if a, b := ahead(t, s, eye, flat), ahead(t, s, eye, raised); a != b {
		t.Errorf("Eye alone changed the reach: %v vs %v", a, b)
	}
}

// elevatedScene is translucentScene with heights, the same for the same r: every second box is a
// walker, every fifth a hawk, the rest walls or forests, over ground that rolls with a sine.
func elevatedScene(r *rand.Rand) (*fakeSpace, elevatedOracle) {
	s, boxes, taus := translucentScene(r)
	o := elevatedOracle{boxes: boxes, taus: taus, bands: map[uid.UID64]band{}, eye: 6, step: 33.3}
	for _, id := range slices.Sorted(maps.Keys(boxes)) {
		switch {
		case id%5 == 0:
			o.bands[id] = band{float32(30 + 4*r.Float64()), float32(34 + 4*r.Float64())}
		case id%2 == 0:
			o.bands[id] = band{0, 2}
		default:
			o.bands[id] = band{0, float32(4 + 8*r.Float64())}
		}
	}
	o.ground = func(p geom.Vec) float32 { return float32(5 + 5*math.Sin(float64(p.X)/90)*math.Cos(float64(p.Y)/70)) }
	for id, b := range boxes {
		bb := o.bands[id]
		if bb[0] == 0 {
			foot := o.ground(geom.NewVec(float32((b[0]+b[2])/2), float32((b[1]+b[3])/2)))
			o.bands[id] = band{foot, foot + bb[1]}
		}
	}
	return s, o
}

func (o elevatedOracle) cone(dir float64, halfAngle, radius float32) raycast.Cone {
	c := seeThrough(elevated(raycast.Cone{
		Direction: geom.NewVec(float32(math.Cos(dir)), float32(math.Sin(dir))), HalfAngle: halfAngle, Radius: radius,
	}, o.eye, o.bands), o.taus)
	c.Ground, c.GroundStep = o.ground, o.step
	return c
}

func TestElevation_DepthsMatchRaysCastDirectly(t *testing.T) {
	r := rand.New(rand.NewPCG(37, 41))
	for trial := range 10 {
		s, o := elevatedScene(r)
		coneDir := float64(trial)
		cone := o.cone(coneDir, math.Pi/4, 500)
		origin := geom.NewVec(2005.0, 2005.0)

		v := scan(t, s, eye, cone)
		const k = 33
		half := float64(cone.HalfAngle)
		step := 2 * half / float64(k-1)
		for i, d := range v.Depths(k, nil) {
			want, _ := o.cast(origin, coneDir-half+float64(i)*step, float64(cone.Radius))
			if math.Abs(float64(d)-want) > 1e-3 {
				t.Errorf("trial %d: depth %d = %.4f, casting that ray gives %.4f", trial, i, d, want)
			}
		}
	}
}

func TestElevation_MissesNothingADenseFanOfRaysReaches(t *testing.T) {
	r := rand.New(rand.NewPCG(43, 47))
	for trial := range 10 {
		s, o := elevatedScene(r)
		cone := o.cone(float64(trial), math.Pi/3, 700)
		origin := geom.NewVec(2005.0, 2005.0)

		got := visible(t, s, eye, cone)
		dir, half := float64(trial), float64(cone.HalfAngle)
		const steps = 4000
		arcs := map[uid.UID64]float64{}
		for i := 0; i <= steps; i++ {
			a := dir - half + 2*half*float64(i)/float64(steps)
			_, seen := o.cast(origin, a, float64(cone.Radius))
			for id := range seen {
				arcs[id] += 2 * half / steps
			}
		}
		for id, a := range arcs {
			if a > math.Pi/90 && !got[id] { // the sweep samples every two degrees
				t.Errorf("trial %d: rays see %d over %.1f°, but Entities does not report it", trial, id, a*180/math.Pi)
			}
		}
	}
}

func TestElevation_ScanDoesNotAllocateOnceWarm(t *testing.T) {
	s, o := elevatedScene(rand.New(rand.NewPCG(53, 59)))
	cone := o.cone(0, math.Pi/3, 700)
	v := &raycast.View{}
	v.Scan(s, eye, cone)
	depths := make([]float32, 0, 64)
	var pts []geom.Vec

	shadows := make([]raycast.Shadow, 0, 512)
	allocs := testing.AllocsPerRun(20, func() {
		v.Scan(s, eye, cone)
		v.Entities(func(uid.UID64, float32) {})
		depths = v.Depths(63, depths[:0])
		pts = v.Outline(0, pts[:0])
		shadows = v.Shadows(63, shadows[:0])
	})
	if allocs > 0 {
		t.Errorf("a warm Scan, Entities, Depths, Outline and Shadows allocate %.0f times per run, want none", allocs)
	}
}

// ahead3 is the shadows straight ahead of a scan read at three angles.
func ahead3(t *testing.T, s raycast.QueryableSpace, c raycast.Cone) []raycast.Shadow {
	t.Helper()
	var out []raycast.Shadow
	for _, sh := range scan(t, s, eye, c).Shadows(3, nil) {
		if sh.Sample == 1 {
			out = append(out, sh)
		}
	}
	return out
}

func near32(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-3 }

func TestShadows_AHillHidesTheLowlandBehindIt(t *testing.T) {
	s := newFake(2000, 2000, false)
	s.put(eye, 100, 100, 10, 10)
	cone := elevated(eastward(math.Pi/8, 500), 1.5, nil)
	cone.Ground, cone.GroundStep = plateau, 25

	// The plateau's face is lit at 200; its top from 225 on is above the line no more, nor is anything past it.
	got := ahead3(t, s, cone)
	if len(got) != 1 || !near32(got[0].From, 212.5) || !near32(got[0].To, 500) {
		t.Errorf("shadows ahead = %+v, want one from 212.5, between the lit face and the hidden top, to the radius", got)
	}
}

// valley is the plateau of 20 on [300, 340), a dip back to 0, and a steep far slope from 480 on.
func valley(p geom.Vec) float32 {
	switch {
	case p.X >= 300 && p.X < 340:
		return 20
	case p.X >= 480:
		return (p.X - 480) * 3
	}
	return 0
}

func TestShadows_EndWhereTheGroundIsSeenAgain(t *testing.T) {
	s := newFake(2000, 2000, false)
	s.put(eye, 100, 100, 10, 10)
	cone := elevated(eastward(math.Pi/8, 500), 1.5, nil)
	cone.Ground, cone.GroundStep = valley, 25

	got := ahead3(t, s, cone)
	if len(got) != 1 || !near32(got[0].From, 212.5) || !near32(got[0].To, 387.5) {
		t.Errorf("shadows ahead = %+v, want one over the dip, from 212.5 to 387.5, the far slope lit again", got)
	}
	if d := ahead(t, s, eye, cone); d != 500 {
		t.Errorf("reach ahead = %v, want the far slope lit to the radius", d)
	}
}

func TestShadows_OnAPlaneRunFromTheReachToTheRadius(t *testing.T) {
	s := newFake(2000, 2000, false)
	s.put(eye, 100, 100, 10, 10)
	s.put(near, 305, 100, 10, 10) // 200 ahead, covering straight ahead only
	got := ahead3(t, s, eastward(math.Pi/4, 500))
	if len(got) != 1 || !near32(got[0].From, 200) || !near32(got[0].To, 500) {
		t.Errorf("shadows ahead = %+v, want the wall's, from 200 to the radius", got)
	}
	for _, sh := range scan(t, s, eye, eastward(math.Pi/4, 500)).Shadows(3, nil) {
		if sh.Sample != 1 {
			t.Errorf("shadow %+v off the wall's line, want none at the cone's edges", sh)
		}
	}
}

func TestShadows_MatchWhatTheOracleSees(t *testing.T) {
	r := rand.New(rand.NewPCG(61, 67))
	for trial := range 10 {
		s, o := elevatedScene(r)
		coneDir := float64(trial)
		cone := o.cone(coneDir, math.Pi/4, 500)
		origin := geom.NewVec(2005.0, 2005.0)
		const k = 17
		half, radius := float64(cone.HalfAngle), float64(cone.Radius)
		step := 2 * half / float64(k-1)
		shadows := scan(t, s, eye, cone).Shadows(k, nil)
		for i := range k {
			_, _, ground := o.castAll(origin, coneDir-half+float64(i)*step, radius)
			for _, g := range ground {
				inShadow := false
				for _, sh := range shadows {
					to := float64(sh.To)
					if sh.Sample == i && float64(sh.From) < g.dist && (g.dist < to || g.dist == radius && to == radius) {
						inShadow = true
					}
				}
				if inShadow == g.visible {
					t.Errorf("trial %d, angle %d: ground %.2f away is visible %v to the oracle but in shadow %v", trial, i, g.dist, g.visible, inShadow)
				}
			}
		}
	}
}
