package raycast_test

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/internal/raycast"
	"github.com/kjkrol/uid"
)

// ridges is a ground of ridges running north to south: straight between a height every span along
// x. Along hands a ray the heights where it crosses them, the profile of a faceted ground.
type ridges struct {
	span    float64
	heights []float64
	reads   *int
}

func (g ridges) knot(k float64) float64 {
	n := len(g.heights)
	return g.heights[((int(k)%n)+n)%n]
}

func (g ridges) at(p geom.Vec) float64 {
	x := p.X / g.span
	k := math.Floor(x)
	f := x - k
	return g.knot(k)*(1-f) + g.knot(k+1)*f
}

func (g ridges) Along(origin, dir geom.Vec, length float64, visit func(d, alt float64) bool) {
	end := geom.NewVec(origin.X+dir.X*length, origin.Y+dir.Y*length)
	if dir.X != 0 {
		x := origin.X / g.span
		k, way := math.Floor(x)+1, 1.0
		if dir.X < 0 {
			k, way = math.Ceil(x)-1, -1
		}
		for ; ; k += way {
			d := (k*g.span - origin.X) / dir.X
			if d >= length {
				break
			}
			*g.reads++
			if d > 0 && !visit(d, g.knot(k)) {
				return
			}
		}
	}
	*g.reads++
	visit(length, g.at(end))
}

// ridgeScene is elevatedScene over ridges 41.3 apart (off the boxes' whole coordinates), the eye 6
// over the ground, the oracle reading their profile.
func ridgeScene(r *rand.Rand) (*fakeSpace, elevatedOracle, ridges) {
	s, o := elevatedScene(r)
	g := ridges{span: 41.3, reads: new(int)}
	for range 13 {
		g.heights = append(g.heights, 12*r.Float64())
	}
	o.ground, o.profile = g.at, g
	o.eye = g.at(geom.NewVec(2005, 2005)) + 6
	for id, b := range o.boxes {
		if bb := o.bands[id]; bb[0] < 30 { // the walkers and walls stand on the ridges
			foot := g.at(geom.NewVec((b[0]+b[2])/2, (b[1]+b[3])/2))
			o.bands[id] = band{foot, foot + bb[1] - bb[0]}
		}
	}
	return s, o, g
}

func (o elevatedOracle) profiled(dir, halfAngle, radius float64) raycast.Cone {
	c := o.cone(dir, halfAngle, radius)
	c.Profile = o.profile
	return c
}

// Over a faceted ground the profile decides what is seen and how far exactly: every depth and
// every shadow is what the oracle reading the same points finds.
func TestProfile_DepthsAndShadowsMatchTheOracle(t *testing.T) {
	r := rand.New(rand.NewPCG(71, 73))
	for trial := range 10 {
		s, o, _ := ridgeScene(r)
		coneDir := float64(trial)
		cone := o.profiled(coneDir, math.Pi/4, 500)
		origin := geom.NewVec(2005.0, 2005.0)
		const k = 33
		step := 2 * cone.HalfAngle / float64(k-1)
		v := scan(t, s, eye, cone)
		shadows := v.Shadows(k, nil)
		for i, d := range v.Depths(k, nil) {
			angle := coneDir - cone.HalfAngle + float64(i)*step
			want, _, ground := o.castAll(origin, angle, cone.Radius)
			if math.Abs(float64(d)-want) > 1e-3 {
				t.Errorf("trial %d: depth %d = %.4f, the oracle finds %.4f", trial, i, d, want)
			}
			for _, g := range ground {
				in := false
				for _, sh := range shadows {
					to := float64(sh.To)
					if sh.Sample == i && float64(sh.From) < g.dist &&
						(g.dist < to || g.dist == cone.Radius && to == cone.Radius) {
						in = true
					}
				}
				if in == g.visible {
					t.Errorf("trial %d, angle %d: ground %.2f away visible %v, in shadow %v",
						trial, i, g.dist, g.visible, in)
				}
			}
		}
	}
}

// lowered is g with every height drop lower.
func (g ridges) lowered(drop float64) ridges {
	low := ridges{span: g.span, reads: new(int)}
	for _, h := range g.heights {
		low.heights = append(low.heights, h-drop)
	}
	return low
}

// What the profile sees is what a ground sampled a hundred times as finely sees, for reading the
// ground a few times a ray. The fine step reads points of the same ground, never above it, so it
// sees all the profile sees; reading a crest up to a step short, it may see a little over it, and
// what it sees so the profile sees over the ground two of its steps lower (here no ridge rises,
// nor sightline falls, one in one).
func TestProfile_SeesWhatAFineStepSees(t *testing.T) {
	r := rand.New(rand.NewPCG(79, 83))
	for trial := range 10 {
		s, o, g := ridgeScene(r)
		profiled := o.profiled(float64(trial), math.Pi/3, 700)
		fine := o.cone(float64(trial), math.Pi/3, 700)
		fine.GroundStep = g.span / 100
		fineReads := 0
		fine.Ground = func(p geom.Vec) float64 {
			fineReads++
			return g.at(p)
		}
		lower := profiled
		low := g.lowered(2 * fine.GroundStep)
		lower.Profile, lower.Ground = low, low.at
		*g.reads = 0
		got := visible(t, s, eye, profiled)
		reads := *g.reads
		want := visible(t, s, eye, fine)
		over := visible(t, s, eye, lower)
		for id := range want {
			if !over[id] {
				t.Errorf("trial %d: a fine step sees %d, the profile not even over a lower ground",
					trial, id)
			}
		}
		for id := range got {
			if !want[id] {
				t.Errorf("trial %d: the profile sees %d, a fine step does not", trial, id)
			}
		}
		if reads == 0 || reads*20 > fineReads {
			t.Errorf("trial %d: the profile read the ground %d times, the fine step %d",
				trial, reads, fineReads)
		}
	}
}

func TestProfile_ScanDoesNotAllocateOnceWarm(t *testing.T) {
	s, o, _ := ridgeScene(rand.New(rand.NewPCG(89, 97)))
	cone := o.profiled(0, math.Pi/3, 700)
	v := &raycast.View{}
	v.Scan(s, eye, cone)
	depths := make([]float32, 0, 64)
	shadows := make([]raycast.Shadow, 0, 512)
	allocs := testing.AllocsPerRun(20, func() {
		v.Scan(s, eye, cone)
		v.Entities(func(uid.UID64, float64) {})
		depths = v.Depths(63, depths[:0])
		shadows = v.Shadows(63, shadows[:0])
	})
	if allocs > 0 {
		t.Errorf("a warm Scan over a profile allocates %.0f times per run, want none", allocs)
	}
}
