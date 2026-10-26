package raycast

import (
	"math"
	"testing"

	"github.com/kjkrol/aabbworld/geom"
)

func TestWrapToPi(t *testing.T) {
	cases := []struct{ in, want float64 }{
		{0, 0},
		{math.Pi / 2, math.Pi / 2},
		{math.Pi, math.Pi},
		{-math.Pi, math.Pi},
		{3 * math.Pi / 2, -math.Pi / 2},
		{-3 * math.Pi / 2, math.Pi / 2},
		{5 * math.Pi, math.Pi},
		{-5 * math.Pi, math.Pi},
	}
	for _, tc := range cases {
		if got := wrapToPi(tc.in); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("wrapToPi(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestShortestDelta(t *testing.T) {
	cases := []struct{ d, size, want float64 }{
		{10, 100, 10},
		{60, 100, -40},
		{-60, 100, 40},
		{50, 100, 50},
		{10, 0, 10},
	}
	for _, tc := range cases {
		if got := shortestDelta(tc.d, tc.size); got != tc.want {
			t.Errorf("shortestDelta(%v, %v) = %v, want %v", tc.d, tc.size, got, tc.want)
		}
	}
}

func TestHitDistance(t *testing.T) {
	box := geom.NewAABB(geom.NewVec(10.0, 10.0), geom.NewVec(20.0, 20.0))
	origin := vec{0.0, 15.0}

	cases := map[string]struct {
		origin, dir vec
		near, far   float64
		hit         bool
	}{
		"straight at it, parallel to Y":   {origin, vec{1.0, 0.0}, 10, 20, true},
		"parallel but above the slab":     {vec{0.0, 5.0}, vec{1.0, 0.0}, 0, 0, false},
		"parallel but below the slab":     {vec{0.0, 25.0}, vec{1.0, 0.0}, 0, 0, false},
		"parallel to X, straight down":    {vec{15.0, 0.0}, vec{0.0, 1.0}, 10, 20, true},
		"pointing away":                   {origin, vec{-1.0, 0.0}, 0, 0, false},
		"diagonal miss":                   {vec{0.0, 0.0}, vec{0.0, 1.0}, 0, 0, false},
		"starting inside hits at zero":    {vec{15.0, 15.0}, vec{1.0, 0.0}, 0, 5, true},
		"diagonal hit through the corner": {vec{0.0, 0.0}, vec{0.7071067811865476, 0.7071067811865476}, 14.142135623730951, 28.284271247461902, true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			near, far, ok := hitDistance(tc.origin, tc.dir, rectOf(box))
			if ok != tc.hit {
				t.Fatalf("hitDistance hit = %v, want %v", ok, tc.hit)
			}
			if ok && (math.Abs(near-tc.near) > 1e-9 || math.Abs(far-tc.far) > 1e-9) {
				t.Errorf("hitDistance = (%v, %v), want (%v, %v)", near, far, tc.near, tc.far)
			}
		})
	}
}

func TestInsertCrossing_KeepsEntriesSortedByWhereTheyAreEntered(t *testing.T) {
	var cross []crossing
	for _, near := range []float64{30, 10, 20, 5, 25} {
		cross = insertCrossing(cross, crossing{near: near, far: near + 1})
	}
	for i := 1; i < len(cross); i++ {
		if cross[i-1].near > cross[i].near {
			t.Fatalf("crossings out of order: %v", cross)
		}
	}
	if len(cross) != 5 {
		t.Errorf("kept %d crossings, want 5", len(cross))
	}
}
