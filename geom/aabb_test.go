package geom

import "testing"

func TestAABB_Intersects(t *testing.T) {
	mk := func(x, y, w, h float32) AABB {
		return NewAABBAt(NewVec(x, y), w, h)
	}

	base := mk(3, 3, 2, 2)
	testCases := []struct {
		name       string
		box1, box2 AABB
		want       bool
	}{
		{name: "identical", box1: base, box2: base, want: true},
		{name: "overlapLeft", box1: base, box2: mk(2, 3, 2, 2), want: true},
		{name: "overlapRight", box1: base, box2: mk(4, 3, 2, 2), want: true},
		{name: "overlapTop", box1: base, box2: mk(3, 2, 2, 2), want: true},
		{name: "overlapBottom", box1: base, box2: mk(3, 4, 2, 2), want: true},
		{name: "containsOther", box1: base, box2: mk(3, 3, 1, 1), want: true},
		{name: "containedInOther", box1: base, box2: mk(2, 2, 4, 4), want: true},
		{name: "touchLeftEdge", box1: base, box2: mk(1, 3, 2, 2), want: true},
		{name: "touchRightEdge", box1: base, box2: mk(5, 3, 2, 2), want: true},
		{name: "touchTopEdge", box1: base, box2: mk(3, 1, 2, 2), want: true},
		{name: "touchBottomEdge", box1: base, box2: mk(3, 5, 2, 2), want: true},
		{name: "touchTopLeftCorner", box1: base, box2: mk(1, 1, 2, 2), want: true},
		{name: "touchTopRightCorner", box1: base, box2: mk(5, 1, 2, 2), want: true},
		{name: "touchBottomLeftCorner", box1: base, box2: mk(1, 5, 2, 2), want: true},
		{name: "touchBottomRightCorner", box1: base, box2: mk(5, 5, 2, 2), want: true},
		{name: "separateLeft", box1: base, box2: mk(0, 3, 2, 2), want: false},
		{name: "separateRight", box1: base, box2: mk(6, 3, 2, 2), want: false},
		{name: "separateAbove", box1: base, box2: mk(3, 0, 2, 2), want: false},
		{name: "separateBelow", box1: base, box2: mk(3, 6, 2, 2), want: false},
		{name: "separateDiagonal", box1: base, box2: mk(0, 0, 2, 2), want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.box1.Intersects(tc.box2); got != tc.want {
				t.Fatalf("box1 %v intersects box2 %v = %v, want %v", tc.box1, tc.box2, got, tc.want)
			}
			if got := tc.box2.Intersects(tc.box1); got != tc.want {
				t.Fatalf("box2 %v intersects box1 %v = %v, want %v", tc.box2, tc.box1, got, tc.want)
			}
		})
	}
}
