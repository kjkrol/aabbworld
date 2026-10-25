package raycast_test

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/internal/raycast"
	"github.com/kjkrol/uid"
)

const forest = uid.UID64(10)

// seeThrough is a cone whose entities are as see-through as taus says; the rest block.
func seeThrough(c raycast.Cone, taus map[uid.UID64]float32) raycast.Cone {
	c.Transparency = func(id uid.UID64) float32 { return taus[id] }
	return c
}

// forestScene is an eye at x 100, a forest 100 thick starting 100 ahead, and a target 400 ahead:
// looked through a τ = 0.5 forest the target costs 100 + 200 + 200 = 500 of reach. The forest
// itself, looked into, is always seen.
func forestScene() *fakeSpace {
	s := newFake(2000, 2000, false)
	s.put(eye, 100, 100, 10, 10)
	s.put(forest, 205, 55, 100, 100)
	s.put(far, 505, 100, 10, 10)
	return s
}

func TestTransparency_ForestChargesItsThicknessAgainstTheReach(t *testing.T) {
	s := forestScene()
	taus := map[uid.UID64]float32{forest: 0.5}

	assertVisible(t, visible(t, s, eye, seeThrough(eastward(math.Pi/8, 501), taus)), forest, far)
	assertVisible(t, visible(t, s, eye, seeThrough(eastward(math.Pi/8, 499), taus)), forest)
}

func TestTransparency_FullyTransparentIsAsGoodAsEmpty(t *testing.T) {
	s := forestScene()
	for name, tau := range map[string]float32{"tau one": 1, "tau above one is capped": 5} {
		t.Run(name, func(t *testing.T) {
			taus := map[uid.UID64]float32{forest: tau}
			assertVisible(t, visible(t, s, eye, seeThrough(eastward(math.Pi/8, 401), taus)), forest, far)
			assertVisible(t, visible(t, s, eye, seeThrough(eastward(math.Pi/8, 399), taus)), forest)
		})
	}
}

func TestTransparency_ZeroBlocksLikeAWall(t *testing.T) {
	s := forestScene()
	for name, taus := range map[string]map[uid.UID64]float32{
		"tau zero":     {forest: 0},
		"tau negative": {forest: -1},
		"not listed":   {},
	} {
		t.Run(name, func(t *testing.T) {
			assertVisible(t, visible(t, s, eye, seeThrough(eastward(math.Pi/8, 600), taus)), forest)
		})
	}
}

func TestTransparency_NilFunctionKeepsEverythingBlocking(t *testing.T) {
	assertVisible(t, visible(t, forestScene(), eye, eastward(math.Pi/8, 600)), forest)
}

func TestTransparency_ObserverInsideAForestPaysFromWhereItStands(t *testing.T) {
	s := newFake(2000, 2000, false)
	s.put(eye, 100, 100, 10, 10)
	s.put(forest, 50, 55, 300, 100) // the eye's centre 105 sits inside; 245 left to the far edge
	s.put(far, 505, 100, 10, 10)    // 400 ahead: 245·2 + 155 = 645 of reach
	taus := map[uid.UID64]float32{forest: 0.5}

	assertVisible(t, visible(t, s, eye, seeThrough(eastward(math.Pi/8, 646), taus)), forest, far)
	assertVisible(t, visible(t, s, eye, seeThrough(eastward(math.Pi/8, 644), taus)), forest)

	v := scan(t, s, eye, seeThrough(eastward(math.Pi/8, 500), taus))
	if d := v.Depths(5, nil)[2]; math.Abs(float64(d)-255) > 1e-3 { // 490 of the 500 spent inside, 10 left
		t.Errorf("depth straight ahead = %.3f, want 255", d)
	}
}

func TestTransparency_TwoForestsAddUp(t *testing.T) {
	s := newFake(2000, 2000, false)
	s.put(eye, 100, 100, 10, 10)
	s.put(forest, 205, 55, 50, 100)   // 100 ahead, 50 thick: costs 100
	s.put(forest+1, 305, 55, 50, 100) // 200 ahead, 50 thick: costs 100
	s.put(far, 505, 100, 10, 10)      // 400 ahead: 100 + 100 + 50 + 100 + 150 = 500
	taus := map[uid.UID64]float32{forest: 0.5, forest + 1: 0.5}

	assertVisible(t, visible(t, s, eye, seeThrough(eastward(math.Pi/8, 501), taus)), forest, forest+1, far)
	assertVisible(t, visible(t, s, eye, seeThrough(eastward(math.Pi/8, 499), taus)), forest, forest+1)
}

func TestTransparency_OverlappingForestsChargeTheStretchOnce(t *testing.T) {
	s := newFake(2000, 2000, false)
	s.put(eye, 100, 100, 10, 10)
	s.put(forest, 205, 55, 100, 100)   // 100 ahead, 100 thick
	s.put(forest+1, 255, 55, 100, 100) // overlaps its second half, runs 50 further: 150 thick in all
	s.put(far, 505, 100, 10, 10)       // 400 ahead: 100 + 300 + 150 = 550
	taus := map[uid.UID64]float32{forest: 0.5, forest + 1: 0.5}

	assertVisible(t, visible(t, s, eye, seeThrough(eastward(math.Pi/8, 551), taus)), forest, forest+1, far)
	assertVisible(t, visible(t, s, eye, seeThrough(eastward(math.Pi/8, 549), taus)), forest, forest+1)
}

func TestTransparency_AWallInsideAForestStillCuts(t *testing.T) {
	s := forestScene()
	s.put(near, 255, 100, 10, 10) // 150 ahead, inside the forest, covering the target
	taus := map[uid.UID64]float32{forest: 0.5}

	assertVisible(t, visible(t, s, eye, seeThrough(eastward(math.Pi/8, 600), taus)), forest, near)
}

func TestTransparency_OutlineIsPulledInWithoutAHit(t *testing.T) {
	s := newFake(2000, 2000, false)
	s.put(eye, 1000, 1000, 10, 10)
	s.put(forest, 1105, 990, 100, 30) // straight ahead: 100 + 200 of the 600, so reach 500 there
	taus := map[uid.UID64]float32{forest: 0.5}
	cone := seeThrough(eastward(math.Pi/4, 600), taus)

	assertVisible(t, visible(t, s, eye, cone), forest)
	pts := outline(t, s, eye, cone, 0)
	shortest, longest := math.Inf(1), 0.0
	for i := 1; i < len(pts); i++ {
		shortest, longest = math.Min(shortest, reach(pts, i)), math.Max(longest, reach(pts, i))
	}
	if math.Abs(shortest-500) > 1 { // sampled every two degrees, so not exactly straight ahead
		t.Errorf("shortest reach %.3f, want about 500 behind the forest", shortest)
	}
	if longest < 599 {
		t.Errorf("longest reach %.3f, want the full 600 beside the forest", longest)
	}
}

// translucentScene scatters boxes ahead of the eye and makes every third one see-through.
func translucentScene(r *rand.Rand) (*fakeSpace, map[uid.UID64][4]float64, map[uid.UID64]float32) {
	s := newFake(4000, 4000, false)
	s.put(eye, 2000, 2000, 10, 10)
	boxes := map[uid.UID64][4]float64{}
	taus := map[uid.UID64]float32{}
	id := uid.UID64(100)
	for cx := range 8 {
		for cy := range 8 {
			if cx == 4 && cy == 4 {
				continue
			}
			x := float32(1600 + cx*100 + r.IntN(20))
			y := float32(1600 + cy*100 + r.IntN(20))
			w, h := float32(20+r.IntN(60)), float32(20+r.IntN(60))
			s.put(id, x, y, w, h)
			boxes[id] = [4]float64{float64(x), float64(y), float64(x + w), float64(y + h)}
			if id%3 == 0 {
				taus[id] = float32(0.25 + 0.5*r.Float64())
			}
			id++
		}
	}
	return s, boxes, taus
}

func TestTransparency_DepthsMatchRaysCastDirectly(t *testing.T) {
	r := rand.New(rand.NewPCG(13, 17))
	for trial := range 10 {
		s, boxes, taus := translucentScene(r)
		coneDir := float64(trial)
		cone := seeThrough(raycast.Cone{
			Direction: geom.NewVec(float32(math.Cos(coneDir)), float32(math.Sin(coneDir))),
			HalfAngle: math.Pi / 4,
			Radius:    500,
		}, taus)
		origin := geom.NewVec(2005.0, 2005.0)

		v := scan(t, s, eye, cone)
		const k = 33
		half := float64(cone.HalfAngle)
		step := 2 * half / float64(k-1)
		for i, d := range v.Depths(k, nil) {
			want, _, _ := castRay(boxes, taus, origin, coneDir-half+float64(i)*step, float64(cone.Radius))
			if math.Abs(float64(d)-want) > 1e-3 {
				t.Errorf("trial %d: depth %d = %.4f, casting that ray gives %.4f", trial, i, d, want)
			}
		}
	}
}

func TestTransparency_MissesNothingADenseFanOfRaysReaches(t *testing.T) {
	r := rand.New(rand.NewPCG(19, 23))
	for trial := range 10 {
		s, boxes, taus := translucentScene(r)
		cone := seeThrough(raycast.Cone{
			Direction: geom.NewVec(float32(math.Cos(float64(trial))), float32(math.Sin(float64(trial)))),
			HalfAngle: math.Pi / 3,
			Radius:    700,
		}, taus)
		origin := geom.NewVec(2005.0, 2005.0)

		got := visible(t, s, eye, cone)
		minArc := math.Pi / 90 // the sweep samples the reach behind a see-through box every two degrees
		for id := range bruteForce(boxes, taus, origin, cone, 4000, minArc) {
			if !got[id] {
				t.Errorf("trial %d: rays reach %d, but Entities does not report it", trial, id)
			}
		}
	}
}

func TestTransparency_ScanDoesNotAllocateOnceWarm(t *testing.T) {
	s, _, taus := translucentScene(rand.New(rand.NewPCG(29, 31)))
	cone := seeThrough(eastward(math.Pi/3, 700), taus)
	v := &raycast.View{}
	v.Scan(s, eye, cone)
	depths := make([]float32, 0, 64)
	var pts []geom.Vec

	allocs := testing.AllocsPerRun(20, func() {
		v.Scan(s, eye, cone)
		depths = v.Depths(63, depths[:0])
		pts = v.Outline(0, pts[:0])
	})
	if allocs > 0 {
		t.Errorf("a warm Scan, Depths and Outline allocate %.0f times per run, want none", allocs)
	}
}
