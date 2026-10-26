package raycast

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/uid"
)

// filled is the slabs of box against n rays along dir, laid as a walk lays them.
func filled(origin, dir vec, box rect, n int) (near, far []float64) {
	var s slabs
	s.dirs = slices.Repeat([]vec{dir}, n)
	s.aim()
	s.near, s.far = make([]float64, n+3), make([]float64, n+3)
	s.fill(origin, box, run{from: 0, to: int32(n)})
	return s.near[:n], s.far[:n]
}

// sameSlabs checks every slab of box filled against n rays along dir is hitDistance's.
func sameSlabs(t *testing.T, origin, dir vec, box rect, n int) {
	t.Helper()
	nears, fars := filled(origin, dir, box, n)
	near, far, ok := hitDistance(origin, dir, box)
	for j := range n {
		gn, gf := nears[j], fars[j]
		if (gn <= gf) != ok || ok && (gn != near || gf != far || math.Signbit(gn)) {
			t.Fatalf("from %v along %v at %v, ray %d of %d: laid (%v, %v), hitDistance %v, %v, %v",
				origin, dir, box, j, n, gn, gf, near, far, ok)
		}
	}
}

func TestSlabs_FillTheEdgesAsHitDistanceMeetsThem(t *testing.T) {
	box := rect{10, 10, 20, 20}
	diag := rayDir(math.Pi / 4)
	cases := map[string]struct {
		origin, dir vec
		box         rect
	}{
		"along x, through it":            {vec{0, 15}, vec{1, 0}, box},
		"along x, on its top edge":       {vec{0, 10}, vec{1, 0}, box},
		"along x, on its bottom edge":    {vec{0, 20}, vec{1, 0}, box},
		"along x, above it":              {vec{0, 5}, vec{1, 0}, box},
		"along y, through it":            {vec{15, 0}, vec{0, 1}, box},
		"along y, on its left edge":      {vec{10, 0}, vec{0, 1}, box},
		"against y, on its right edge":   {vec{20, 30}, vec{0, -1}, box},
		"along y, beside it":             {vec{25, 0}, vec{0, 1}, box},
		"from inside":                    {vec{15, 15}, rayDir(1), box},
		"from inside, against x":         {vec{15, 15}, vec{-1, 0}, box},
		"from its corner, outward":       {vec{20, 20}, diag, box},
		"from its edge, along it":        {vec{10, 15}, vec{0, 1}, box},
		"from its edge, away":            {vec{10, 15}, vec{-1, 0}, box},
		"behind":                         {vec{30, 15}, vec{1, 0}, box},
		"behind, diagonally":             {vec{30, 30}, diag, box},
		"diagonally through its corners": {vec{0, 0}, diag, box},
		"grazing its corner":             {vec{0, 0}, diag, rect{10, -10, 20, 10}},
		"missing its corner":             {vec{0, 0}, diag, rect{10, -10, 20, 9.5}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// One ray alone is worked out as it is cast, more are laid at once.
			for _, n := range []int{1, 2, 5, 8} {
				sameSlabs(t, tc.origin, tc.dir, tc.box, n)
			}
		})
	}
}

func TestSlabs_FillRandomBoxesAsHitDistanceMeetsThem(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 7))
	axes := []vec{{1, 0}, {0, 1}, {-1, 0}, {0, -1}}
	for range 20_000 {
		origin := vec{r.Float64()*200 - 100, r.Float64()*200 - 100}.round32()
		x, y := float32(r.Float64()*400-200), float32(r.Float64()*400-200)
		if r.IntN(8) == 0 { // an edge on the origin's lines
			x, y = float32(origin.x), float32(origin.y)-float32(r.IntN(40))
		}
		w, h := float32(1+r.Float64()*60), float32(1+r.Float64()*60)
		box := rectOf(geom.NewAABBAt(geom.NewVec(x, y), w, h))
		dir := rayDir(r.Float64() * 2 * math.Pi)
		if r.IntN(8) == 0 {
			dir = axes[r.IntN(4)]
		}
		sameSlabs(t, origin, dir, box, 1+r.IntN(9))
	}
}

// slabScene is candidates round an eye at origin as a gather would hand them over, a few with an
// edge on the eye's own lines.
func slabScene(r *rand.Rand, origin vec, coneDir, half, radius float64) []candidate {
	var cands []candidate
	add := func(x, y, w, h float32, tau float64) {
		box := geom.NewAABBAt(geom.NewVec(x, y), w, h)
		span := subtendedArc(origin, box, coneDir, half)
		if tau > 0 && surrounds(box, origin) {
			span = arc{-half, half}
		}
		if span.empty() {
			return
		}
		cands = append(cands, candidate{
			id: uid.UID64(100 + len(cands)), span: span, box: rectOf(box), tau: tau,
			bottom: math.Inf(-1), top: math.Inf(1), standing: true, seenAt: math.Inf(1),
		})
	}
	ox, oy := float32(origin.x), float32(origin.y)
	for range 60 {
		a := coneDir + (r.Float64()*2-1)*half
		d := r.Float64() * radius
		x, y := ox+float32(d*math.Cos(a)), oy+float32(d*math.Sin(a))
		tau := 0.0
		if r.IntN(3) == 0 {
			tau = 0.25 + 0.5*r.Float64()
		}
		add(x, y, float32(2+r.IntN(40)), float32(2+r.IntN(40)), tau)
	}
	// Ahead along the cone's direction, the ray of angle 0 runs along one edge of each.
	add(ox+50, oy, 20, 20, 0)
	add(ox+80, oy-20, 20, 20, 0.5)
	add(ox-15, oy-15, 30, 30, 0.5) // round the eye
	return cands
}

// uniform is the k angles Depths casts along across a cone of half.
func uniform(k int, half float64) []float64 {
	out := make([]float64, k)
	for i := range k {
		out[i] = -half + float64(i)*2*half/float64(k-1)
	}
	return out
}

// checkLaid replays a walk along angles as walk has it and checks every slab laid is hitDistance's
// and the ray of every angle rayDir's.
func checkLaid(
	t *testing.T,
	s *slabs,
	origin vec,
	coneDir, minGap float64,
	angles []float64,
	events []event,
	cands []candidate,
) {
	t.Helper()
	var active []int
	next, met := 0, 0
	for j, a := range angles {
		if j > 0 && a-angles[j-1] < minGap {
			continue
		}
		for next < len(events) && events[next].angle <= a {
			if e := events[next]; e.enter {
				active = append(active, e.idx)
			} else if at := slices.Index(active, e.idx); at >= 0 {
				active = slices.Delete(active, at, at+1)
			}
			next++
		}
		if dir := rayDir(coneDir + a); s.dirs[j] != dir {
			t.Fatalf("angle %d: the ray laid is %v, rayDir gives %v", j, s.dirs[j], dir)
		}
		for i, r := range s.runs {
			if in := int(r.from) <= j && j < int(r.to); in != slices.Contains(active, i) {
				t.Fatalf("angle %d: candidate %d active %v, its run %v", j, i, !in, r)
			}
		}
		for _, i := range active {
			p := s.runs[i].at + int32(j)
			gn, gf := s.near[p], s.far[p]
			near, far, ok := hitDistance(origin, s.dirs[j], cands[i].box)
			if (gn <= gf) != ok || ok && (gn != near || gf != far) {
				t.Fatalf("angle %d, candidate %d at %v: laid (%v, %v), hitDistance (%v, %v, %v)",
					j, i, cands[i].box, gn, gf, near, far, ok)
			}
			met++
		}
	}
	if met == 0 {
		t.Fatal("no candidate was active at any angle")
	}
}

func TestSlabs_LayWhatEveryCastMeets(t *testing.T) {
	r := rand.New(rand.NewPCG(11, 13))
	for trial := range 40 {
		origin := vec{2000 + r.Float64()*10, 2000 + r.Float64()*10}.round32()
		coneDir, half, radius := 0.0, math.Pi/4, 300.0
		if trial%2 == 1 {
			coneDir, half = (r.Float64()*2-1)*math.Pi, 0.2+r.Float64()*1.2
		}
		cands := slabScene(r, origin, coneDir, half, radius)
		var sc scratch
		sweep(origin, coneDir, half, radius, cands, nil, &sc)
		minGap := math.Atan2(1, radius) / 8
		sc.slabs.lay(origin, coneDir, minGap, sc.angles, sc.events, cands)
		checkLaid(t, &sc.slabs, origin, coneDir, minGap, sc.angles, sc.events, cands)
		// Depths casts at angles of its own, angle 0 among them when the count less one is a power
		// of two: along the x axis when the cone looks along it.
		for _, k := range []int{2, 33, 63} {
			angles := uniform(k, half)
			sc.slabs.lay(origin, coneDir, 0, angles, sc.events, cands)
			checkLaid(t, &sc.slabs, origin, coneDir, 0, angles, sc.events, cands)
		}
	}
}

func TestSlabs_LayNothingButRaysWithoutCandidates(t *testing.T) {
	var s slabs
	angles := uniform(9, math.Pi/4)
	s.lay(vec{10, 10}, 1, 0, angles, nil, nil)
	for j, a := range angles {
		if s.dirs[j] != rayDir(1+a) {
			t.Errorf("angle %d: the ray laid is %v, rayDir gives %v", j, s.dirs[j], rayDir(1+a))
		}
	}
	if len(s.runs) != 0 || len(s.near) != 0 {
		t.Errorf("laid %d runs and %d slabs for no candidates", len(s.runs), len(s.near))
	}
}
