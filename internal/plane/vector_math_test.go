package plane

import (
	"github.com/kjkrol/aabbworld/geom"
	"math"
	"testing"
)

func TestClamp(t *testing.T) {
	bounds := geom.NewVec(4, 6)

	cases := map[string]struct{ start, want geom.Vec }{
		"inside is left alone": {geom.NewVec(2, 3), geom.NewVec(2, 3)},
		"past the far edge":    {geom.NewVec(5, 7), geom.NewVec(4, 6)},
		"below zero":           {geom.NewVec(-3, -1), geom.NewVec(0, 0)},
		"exactly on the edge":  {geom.NewVec(4, 6), geom.NewVec(4, 6)},
		"fractional stays put": {geom.NewVec(1.5, 2.25), geom.NewVec(1.5, 2.25)},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := Clamp(c.start, bounds); got != c.want {
				t.Errorf("Clamp(%v) = %v, want %v", c.start, got, c.want)
			}
		})
	}
}

func TestWrap(t *testing.T) {
	size := geom.NewVec(4, 4)

	cases := map[string]struct{ start, want geom.Vec }{
		"inside is left alone":     {geom.NewVec(1, 3), geom.NewVec(1, 3)},
		"one step past the edge":   {geom.NewVec(5, 7), geom.NewVec(1, 3)},
		"exactly on the far edge":  {geom.NewVec(4, 4), geom.NewVec(0, 0)},
		"just below zero":          {geom.NewVec(-3, -1), geom.NewVec(1, 3)},
		"far out, past the fast p": {geom.NewVec(19, -13), geom.NewVec(3, 3)},
		"fractional wraps too":     {geom.NewVec(4.5, -0.5), geom.NewVec(0.5, 3.5)},
		// -1e-7 + 4 rounds to 4 in float32, which is past the end of [0, 4).
		"a hair below zero comes to zero": {geom.NewVec(-1e-7, -1e-7), geom.NewVec(0, 0)},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got := Wrap(c.start, size)
			if math.Abs(float64(got.X-c.want.X)) > 1e-12 || math.Abs(float64(got.Y-c.want.Y)) > 1e-12 {
				t.Errorf("Wrap(%v) = %v, want %v", c.start, got, c.want)
			}
		})
	}
}

func TestWrap_ZeroSizePassesThrough(t *testing.T) {
	if got := Wrap(geom.NewVec(7, -7), geom.NewVec(0, 0)); got != geom.NewVec(7, -7) {
		t.Errorf("Wrap with a zero size = %v, want the input unchanged", got)
	}
}
