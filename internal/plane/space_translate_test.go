package plane

import (
	"github.com/kjkrol/aabbworld/plane"
	"testing"

	"github.com/kjkrol/aabbworld/geom"
)

func TestToroidal2DTranslate(t *testing.T) {
	t.Run("CrossesRightEdge", func(t *testing.T) {
		toroidal := NewToroidal2D(10, 10)
		aabb := plane.NewAABB(vec(8, 4), 4, 2)

		toroidal.Translate(&aabb, vec(0, 0))

		expectAABBState(t, aabb, vec(8, 4), vec(10, 6), map[plane.FragPosition][2]geom.Vec{
			plane.FRAG_RIGHT: {vec(0, 4), vec(2, 6)},
		})
	})

	t.Run("HugeShift", func(t *testing.T) {
		toroidal := NewToroidal2D(10, 10)
		aabb := plane.NewAABB(vec(8, 4), 4, 2)

		toroidal.Translate(&aabb, vec(100, 100))
		expectAABBState(t, aabb, vec(8, 4), vec(10, 6), map[plane.FragPosition][2]geom.Vec{
			plane.FRAG_RIGHT: {vec(0, 4), vec(2, 6)},
		})

		toroidal.Translate(&aabb, vec(100, 100))
		expectAABBState(t, aabb, vec(8, 4), vec(10, 6), map[plane.FragPosition][2]geom.Vec{
			plane.FRAG_RIGHT: {vec(0, 4), vec(2, 6)},
		})
	})

	t.Run("BackAndForth", func(t *testing.T) {
		toroidal := NewToroidal2D(10, 10)
		aabb := plane.NewAABB(vec(8, 8), 2, 2)

		toroidal.Translate(&aabb, vec(5, 5))
		expectAABBState(t, aabb, vec(3, 3), vec(5, 5), map[plane.FragPosition][2]geom.Vec{})

		toroidal.Translate(&aabb, vec(5, 5))
		expectAABBState(t, aabb, vec(8, 8), vec(10, 10), map[plane.FragPosition][2]geom.Vec{})
	})

	t.Run("CrossesBottomEdge", func(t *testing.T) {
		toroidal := NewToroidal2D(10, 10)
		aabb := plane.NewAABB(vec(4, 8), 2, 4)

		toroidal.Translate(&aabb, vec(0, 0))

		expectAABBState(t, aabb, vec(4, 8), vec(6, 10), map[plane.FragPosition][2]geom.Vec{
			plane.FRAG_BOTTOM: {vec(4, 0), vec(6, 2)},
		})
	})

	t.Run("CrossesCorner", func(t *testing.T) {
		toroidal := NewToroidal2D(10, 10)
		aabb := plane.NewAABB(vec(9, 9), 2, 2)

		toroidal.Translate(&aabb, vec(0, 0))

		expectAABBState(t, aabb, vec(9, 9), vec(10, 10), map[plane.FragPosition][2]geom.Vec{
			plane.FRAG_RIGHT:        {vec(0, 9), vec(1, 10)},
			plane.FRAG_BOTTOM:       {vec(9, 0), vec(10, 1)},
			plane.FRAG_BOTTOM_RIGHT: {vec(0, 0), vec(1, 1)},
		})
	})

	t.Run("ClearsFragmentsWhenNotWrapping", func(t *testing.T) {
		toroidal := NewToroidal2D(10, 10)
		aabb := plane.NewAABB(vec(8, 4), 4, 2)

		toroidal.Translate(&aabb, vec(0, 0))
		expectAABBState(t, aabb, vec(8, 4), vec(10, 6), map[plane.FragPosition][2]geom.Vec{
			plane.FRAG_RIGHT: {vec(0, 4), vec(2, 6)},
		})

		toroidal.Translate(&aabb, vec(8, 0))
		expectAABBState(t, aabb, vec(6, 4), vec(10, 6), map[plane.FragPosition][2]geom.Vec{})
	})

	t.Run("ThroughEdge", func(t *testing.T) {
		toroidal := NewToroidal2D(10, 10)
		aabb := plane.NewAABB(vec(2, 2), 2, 2)

		toroidal.Translate(&aabb, vec(8, 0))

		expectAABBState(t, aabb, vec(0, 2), vec(2, 4), map[plane.FragPosition][2]geom.Vec{})
	})

	t.Run("FragmentsMergeSequence", func(t *testing.T) {
		toroidal := NewToroidal2D(10, 10)
		aabb := plane.NewAABB(vec(2, 2), 2, 2)

		toroidal.Translate(&aabb, vec(7, 0))
		expectAABBState(t, aabb, vec(9, 2), vec(10, 4), map[plane.FragPosition][2]geom.Vec{
			plane.FRAG_RIGHT: {vec(0, 2), vec(1, 4)},
		})

		toroidal.Translate(&aabb, vec(0, 7))
		expectAABBState(t, aabb, vec(9, 9), vec(10, 10), map[plane.FragPosition][2]geom.Vec{
			plane.FRAG_RIGHT:        {vec(0, 9), vec(1, 10)},
			plane.FRAG_BOTTOM:       {vec(9, 0), vec(10, 1)},
			plane.FRAG_BOTTOM_RIGHT: {vec(0, 0), vec(1, 1)},
		})

		toroidal.Translate(&aabb, vec(3, 3))
		expectAABBState(t, aabb, vec(2, 2), vec(4, 4), map[plane.FragPosition][2]geom.Vec{})
	})
}

func TestEuclidean2DTranslate(t *testing.T) {
	t.Run("HugeShift", func(t *testing.T) {
		euclidean := NewEuclidean2D(10, 10)
		aabb := plane.NewAABB(vec(8, 4), 4, 2)

		euclidean.Translate(&aabb, vec(100, 100))
		expectAABBState(t, aabb, vec(6, 8), vec(10, 10), map[plane.FragPosition][2]geom.Vec{})
	})

	t.Run("TranslateWithinBounds", func(t *testing.T) {
		euclidean := NewEuclidean2D(10, 10)
		aabb := plane.NewAABB(vec(1, 1), 2, 2)

		euclidean.Translate(&aabb, vec(2, 2))
		expectAABBState(t, aabb, vec(3, 3), vec(5, 5), map[plane.FragPosition][2]geom.Vec{})
	})

	t.Run("ClampAtBoundary", func(t *testing.T) {
		euclidean := NewEuclidean2D(10, 10)
		aabb := plane.NewAABB(vec(9, 9), 3, 3)

		euclidean.Translate(&aabb, vec(2, 2))
		expectAABBState(t, aabb, vec(7, 7), vec(10, 10), map[plane.FragPosition][2]geom.Vec{})
	})
}

func expectAABBState(
	t *testing.T,
	b plane.AABB,
	expectedPos geom.Vec,
	expectedBottomRight geom.Vec,
	expectedFragments map[plane.FragPosition][2]geom.Vec,
) {
	t.Helper()

	if !b.TopLeft.Equals(expectedPos) {
		t.Fatalf("expected bound position %v, got %v", expectedPos, b.TopLeft)
	}

	if !b.BottomRight.Equals(expectedBottomRight) {
		t.Fatalf("expected bound bottom-right %v, got %v", expectedBottomRight, b.BottomRight)
	}

	expectAABBFragments(t, b, expectedFragments)
}

func expectAABBFragments(t *testing.T, b plane.AABB, expected map[plane.FragPosition][2]geom.Vec) {
	t.Helper()

	actual := map[plane.FragPosition][2]geom.Vec{}
	(&b).VisitFragments(func(pos plane.FragPosition, box geom.AABB) bool {
		actual[pos] = [2]geom.Vec{box.TopLeft, box.BottomRight}
		return true
	})

	if len(actual) != len(expected) {
		t.Fatalf("expected %d fragments, got %d", len(expected), len(actual))
	}

	for pos, want := range expected {
		frag, ok := actual[pos]
		if !ok {
			t.Fatalf("missing fragment at %d", pos)
		}
		if !frag[0].Equals(want[0]) || !frag[1].Equals(want[1]) {
			t.Fatalf("fragment at %d has bounds %v..%v, expected %v..%v",
				pos, frag[0], frag[1], want[0], want[1])
		}
	}
}

func TestTranslate_InsideOrTouchingTheEdgeAgreesWithTheFold(t *testing.T) {
	// before is the rule of an axis as it was when every box was folded.
	before := func(lo, length, world float32, wraps, open bool) (float32, float32, float32) {
		switch {
		case wraps:
			return fold(lo, length, world)
		case open:
			return lo, lo + length, 0
		default:
			lo = clamp(lo, max(world-length, 0))
			return lo, lo + length, 0
		}
	}

	// A 10-long box from 50 lands at 0, at world-10, well inside, and 5 past the edge.
	pushes := []float32{-50, 40, -13, 45}
	for _, tc := range []struct {
		name  string
		edges Edges
	}{{"toroidal", Torus}, {"euclidean", 0}, {"open", OpenX | OpenY}} {
		s := NewSurface(100, 100, tc.edges)
		for _, dx := range pushes {
			for _, dy := range pushes {
				box := plane.NewAABB(vec(50, 50), 10, 10)
				s.Translate(&box, vec(dx, dy))

				x0, x1, ox := before(50+dx, 10, 100, tc.edges&WrapX != 0, tc.edges&OpenX != 0)
				y0, y1, oy := before(50+dy, 10, 100, tc.edges&WrapY != 0, tc.edges&OpenY != 0)
				want := plane.AABB{
					AABB: geom.NewAABB(vec(x0, y0), vec(x1, y1)), Size: vec(10, 10), Overhang: vec(ox, oy),
				}
				if box != want {
					t.Fatalf("%s, push (%v,%v): box %+v, want %+v", tc.name, dx, dy, box, want)
				}
			}
		}
	}

	torus := NewToroidal2D(100, 100)
	point := plane.NewAABB(vec(90, 90), 0, 0)
	torus.Translate(&point, vec(10, 10))
	if point.TopLeft != vec(0, 0) || point.BottomRight != vec(0, 0) {
		t.Errorf("zero-length box pushed onto the far edge of the torus came to %+v, want 0,0", point)
	}
}
