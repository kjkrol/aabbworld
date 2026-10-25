package aabbworld_test

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/kjkrol/aabbworld"
	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/uid"
)

// ticked runs n ticks over s and counts the contacts each reports
func ticked(s *scene, n int) []int {
	counts := make([]int, n)
	for k := range counts {
		s.tick(nil, func(uid.UID64, uid.UID64, geom.Vec) { counts[k]++ })
	}
	return counts
}

// Four boxes of a crowd at rest, one on another, as gram's navigation left them: a float32 step of
// overlap shared by halves moved round the stack in every pass of every tick, never gone.
func TestTick_AStackTouchingWithinRoundingComesToRest(t *testing.T) {
	space, err := aabbworld.NewSpace(aabbworld.Config{Width: 384, Height: 256, BucketSize: 64})
	if err != nil {
		t.Fatal(err)
	}
	s := newSceneIn(space, 16)
	s.put(18, 189.15367, 155.1397)
	s.put(19, 192.07181, 165.13968)
	s.put(10, 180.23291, 145.13971)
	s.put(15, 188.63841, 135.13971)
	counts := ticked(s, 6)
	for k, n := range counts[1:] {
		if n != 0 {
			t.Fatalf("contacts by tick: %v; tick %d still pushes the stack about", counts, k+1)
		}
	}
	if len(s.moved) != 0 {
		t.Errorf("the last tick moved %v, want the stack at rest", s.moved)
	}
}

// Columns of boxes, each laid a step or two of float32 into the one under it, at scales from a
// hundred to fifteen thousand units: once pushed apart they lie still, every pair touching or apart
// within float32's rounding.
func TestTick_StacksLaidWithinRoundingComeToRest(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 7))
	for _, at := range []float32{100, 1000, 6000, 15000} {
		space, err := aabbworld.NewSpace(aabbworld.Config{Width: 16384, Height: 16384, BucketSize: 64})
		if err != nil {
			t.Fatal(err)
		}
		s := newSceneIn(space, 16)
		id := uid.UID64(0)
		for col := range 6 {
			x := at + float32(col)*12
			y := at
			for range 5 {
				id++
				s.put(id, x+float32(rng.IntN(5))-2, y)
				y += 10
				for range rng.IntN(3) {
					y = math.Nextafter32(y, 0)
				}
			}
		}
		counts := ticked(s, 6)
		for k, n := range counts[2:] {
			if n != 0 {
				t.Errorf("at %v: contacts by tick %v; tick %d still pushes", at, counts, k+2)
				break
			}
		}
		for i := range s.items {
			for j := i + 1; j < len(s.items); j++ {
				a, b := s.items[i].Box.AABB, s.items[j].Box.AABB
				if over := overlapOf(a, b); over > 4*ulp(at+100) {
					t.Errorf("at %v: %v and %v overlap by %v after the ticks", at, a, b, over)
				}
			}
		}
	}
}

// the smaller of how far a and b overlap across and down, 0 where they do not
func overlapOf(a, b geom.AABB) float32 {
	w := min(a.BottomRight.X, b.BottomRight.X) - max(a.TopLeft.X, b.TopLeft.X)
	h := min(a.BottomRight.Y, b.BottomRight.Y) - max(a.TopLeft.Y, b.TopLeft.Y)
	return max(min(w, h), 0)
}

func ulp(x float32) float32 { return math.Nextafter32(x, float32(math.Inf(1))) - x }
