package plane

import (
	"testing"

	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/plane"
)

type axisRule struct {
	name       string
	wrap, open Edges
}

var (
	rulesX = []axisRule{{"closed", 0, 0}, {"open", 0, OpenX}, {"wrap", WrapX, 0}}
	rulesY = []axisRule{{"closed", 0, 0}, {"open", 0, OpenY}, {"wrap", WrapY, 0}}
)

// afterPush is where a 10-long box at 50 of a 100-long axis ends up, pushed by delta under rule.
func afterPush(rule string, delta float32) (lo, hi, overhang float32) {
	lo = 50 + delta
	switch rule {
	case "closed":
		lo = min(max(lo, 0), 90)
	case "wrap":
		for lo < 0 {
			lo += 100
		}
		for lo >= 100 {
			lo -= 100
		}
		if lo+10 > 100 {
			return lo, 100, lo + 10 - 100
		}
	}
	return lo, lo + 10, 0
}

func TestSurface_EachAxisFollowsItsOwnRule(t *testing.T) {
	pushes := []float32{-57, -45, -3, 0, 3, 44, 47, 120}
	for _, rx := range rulesX {
		for _, ry := range rulesY {
			s := NewSurface(100, 100, rx.wrap|rx.open|ry.wrap|ry.open)
			for _, dx := range pushes {
				for _, dy := range pushes {
					box := plane.NewAABB(vec(50, 50), 10, 10)
					s.Translate(&box, vec(dx, dy))

					x0, x1, ox := afterPush(rx.name, dx)
					y0, y1, oy := afterPush(ry.name, dy)
					want := plane.AABB{AABB: geom.NewAABB(vec(x0, y0), vec(x1, y1)), Size: vec(10, 10), Overhang: vec(ox, oy)}
					if box != want {
						t.Fatalf("x %s, y %s, push (%v,%v): box %+v, want %+v", rx.name, ry.name, dx, dy, box, want)
					}
				}
			}
		}
	}
}

func TestSurface_OpenAxisLetsABoxLeaveAndComeBackWhereItWas(t *testing.T) {
	s := NewSurface(100, 100, OpenX|WrapY)
	box := plane.NewAABB(vec(5, 50), 10, 10)
	start := box

	s.Translate(&box, vec(-8, 0))
	if s.Left(&box) {
		t.Fatalf("box %v reported gone while part of it is still inside", box)
	}
	s.Translate(&box, vec(-8, 0))
	if !s.Left(&box) {
		t.Fatalf("box %v not reported gone once wholly past the open edge", box)
	}
	s.Translate(&box, vec(16, 0))
	if box != start {
		t.Errorf("box came back as %+v, want exactly %+v", box, start)
	}
}

func TestSurface_OnlyAnOpenAxisCanBeLeft(t *testing.T) {
	far := plane.AABB{AABB: geom.NewAABB(vec(-30, -30), vec(-20, -20)), Size: vec(10, 10)}
	for _, tc := range []struct {
		edges Edges
		want  bool
	}{
		{0, false}, {Torus, false}, {WrapX, false}, {OpenX, true}, {OpenY, true}, {WrapX | OpenY, true},
	} {
		if got := NewSurface(100, 100, tc.edges).Left(&far); got != tc.want {
			t.Errorf("edges %04b: Left = %v, want %v", tc.edges, got, tc.want)
		}
	}
}

func TestEdges_AnAxisCannotBothWrapAndBeOpen(t *testing.T) {
	for _, e := range []Edges{0, Torus, WrapX | OpenY, WrapY | OpenX, OpenX | OpenY} {
		if !e.Valid() {
			t.Errorf("edges %04b refused, want accepted", e)
		}
	}
	for _, e := range []Edges{WrapX | OpenX, WrapY | OpenY, Torus | OpenX, Torus | OpenX | OpenY} {
		if e.Valid() {
			t.Errorf("edges %04b accepted, want refused", e)
		}
	}
}
