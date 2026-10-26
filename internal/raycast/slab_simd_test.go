//go:build goexperiment.simd && amd64

package raycast

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/kjkrol/aabbworld/geom"
)

func TestFillWide_EveryLaneIsHitDistance(t *testing.T) {
	SetWide(t, true)
	r := rand.New(rand.NewPCG(5, 9))
	const n = 61
	var s slabs
	s.dirs = make([]vec, n)
	for j := range s.dirs {
		s.dirs[j] = rayDir(r.Float64() * 2 * math.Pi)
		switch j % 13 {
		case 3:
			s.dirs[j] = vec{0, 1}
		case 7:
			s.dirs[j] = vec{-1, 0}
		}
	}
	s.aim()
	s.near, s.far = make([]float64, 2*n+3), make([]float64, 2*n+3)
	for range 20_000 {
		origin := vec{r.Float64()*20 - 10, r.Float64()*20 - 10}.round32()
		x, y := float32(r.Float64()*200-100), float32(r.Float64()*200-100)
		w, h := float32(r.Float64()*50), float32(r.Float64()*50)
		box := rectOf(geom.NewAABBAt(geom.NewVec(x, y), w, h))
		if box.x1 == origin.x || box.y1 == origin.y || box.x2 == origin.x || box.y2 == origin.y {
			continue // an edge through the origin is fill's to mend
		}
		from := int32(r.IntN(n))
		run := run{from: from, to: from + 1 + int32(r.IntN(n-int(from))), at: int32(r.IntN(n))}
		s.fillWide(box.x1-origin.x, box.y1-origin.y, box.x2-origin.x, box.y2-origin.y, run)
		for j := run.from; j < run.to; j++ {
			gn, gf := s.near[run.at+j], s.far[run.at+j]
			near, far, ok := hitDistance(origin, s.dirs[j], box)
			if (gn <= gf) != ok || ok && (gn != near || gf != far || math.Signbit(gn)) {
				t.Fatalf("from %v along %v at %v: lane (%v, %v), hitDistance (%v, %v, %v)",
					origin, s.dirs[j], box, gn, gf, near, far, ok)
			}
		}
	}
}

func TestInvertWide_IsOneOverEach(t *testing.T) {
	SetWide(t, true)
	v := []float64{1, -2, 3, 0.1, math.Inf(1), 0, math.Copysign(0, -1), 7, 1e-300, 3e300}
	v = append(v, make([]float64, 3)...)
	want := make([]float64, len(v))
	for i, x := range v {
		want[i] = 1 / x
	}
	invertWide(v)
	for i := range 10 {
		if v[i] != want[i] {
			t.Errorf("1/x at %d = %v, want %v", i, v[i], want[i])
		}
	}
}
