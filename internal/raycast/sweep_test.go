package raycast_test

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/internal/raycast"
	"github.com/kjkrol/uid"
)

// bruteForce is what a dense fan of rays meets across an arc wider than minArc.
// bruteForce casts a dense fan of rays and reports every box hit over an arc wider than minArc.
func bruteForce(boxes map[uid.UID64][4]float64, taus map[uid.UID64]float32, origin geom.Vec, cone raycast.Cone, steps int, minArc float64) map[uid.UID64]bool {
	dir := math.Atan2(float64(cone.Direction.Y), float64(cone.Direction.X))
	half, radius := float64(cone.HalfAngle), float64(cone.Radius)
	step := 2 * half / float64(steps)
	arcs := map[uid.UID64]float64{}
	for i := 0; i <= steps; i++ {
		a := dir - half + 2*half*float64(i)/float64(steps)
		_, id, hit, through := castRaySeeing(boxes, taus, origin, a, radius)
		if hit {
			arcs[id] += step
		}
		for _, id := range through {
			arcs[id] += step
		}
	}

	seen := map[uid.UID64]bool{}
	for id, a := range arcs {
		if a > minArc {
			seen[id] = true
		}
	}
	return seen
}

func TestVisible_MissesNothingADenseFanOfRaysReaches(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 11))
	for trial := range 20 {
		s := newFake(4000, 4000, false)
		s.put(eye, 2000, 2000, 10, 10)
		boxes := map[uid.UID64][4]float64{}
		id := uid.UID64(100)
		for cx := range 8 {
			for cy := range 8 {
				if cx == 4 && cy == 4 {
					continue
				}
				x := float32(1600 + cx*100 + r.IntN(20))
				y := float32(1600 + cy*100 + r.IntN(20))
				w, h := float32(20+r.IntN(50)), float32(20+r.IntN(50))
				s.put(id, x, y, w, h)
				boxes[id] = [4]float64{float64(x), float64(y), float64(x + w), float64(y + h)}
				id++
			}
		}

		cone := raycast.Cone{
			Direction: geom.NewVec(float32(math.Cos(float64(trial))), float32(math.Sin(float64(trial)))),
			HalfAngle: math.Pi / 3,
			Radius:    700,
		}
		origin := geom.NewVec(2005.0, 2005.0)

		got := map[uid.UID64]bool{}
		scan(t, s, eye, cone).Entities(func(id uid.UID64, _ float32) { got[id] = true })

		minArc := 2 * math.Atan2(1, float64(cone.Radius))
		for id := range bruteForce(boxes, nil, origin, cone, 4000, minArc) {
			if !got[id] {
				t.Errorf("trial %d: rays reach %d, but Visible does not report it", trial, id)
			}
		}
	}
}
