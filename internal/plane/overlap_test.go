package plane

import (
	"github.com/kjkrol/aabbworld/plane"
	"math"
	"testing"

	"github.com/kjkrol/aabbworld/geom"
)

// boxAt spells a box as position and size.
func boxAt(x, y, w, h float32) geom.AABB {
	return geom.NewAABB(geom.NewVec(x, y), geom.NewVec(x+w, y+h))
}

func TestDeepestOverlap_FragmentMeetsFragment(t *testing.T) {
	space := NewToroidal2D(100, 100)

	a := space.WrapAABB(boxAt(98, 1, 10, 10))
	b := space.WrapAABB(boxAt(3, 98, 10, 10))

	if a.Overhang.Y != 0 || b.Overhang.X != 0 {
		t.Fatalf("fixture is wrong: a overhang %v, b overhang %v — a must wrap only right, b only bottom", a.Overhang, b.Overhang)
	}
	if a.AABB.Intersects(b.AABB) {
		t.Fatal("fixture is wrong: the main boxes already meet, so the case proves nothing")
	}

	got, ok := a.DeepestOverlapWith(&b)
	if !ok {
		t.Fatal("no overlap found, though a's wrapped part stands on b's wrapped part near the origin")
	}
	if got.Penetration.X == 0 && got.Penetration.Y == 0 {
		t.Errorf("Penetration = %v, want a non-zero push", got.Penetration)
	}
	if got.BoxA.TopLeft.X != 0 {
		t.Errorf("BoxA = %v, want a's wrapped part starting at x=0", got.BoxA)
	}
	if got.BoxB.TopLeft.Y != 0 {
		t.Errorf("BoxB = %v, want b's wrapped part starting at y=0", got.BoxB)
	}
}

func TestDeepestOverlap_AgreesWithAnImageByImageWalk(t *testing.T) {
	space := NewToroidal2D(100, 100)

	cases := map[string]struct{ a, b plane.AABB }{
		"clear of each other":     {space.WrapAABB(boxAt(10, 10, 10, 10)), space.WrapAABB(boxAt(50, 50, 10, 10))},
		"plain overlap":           {space.WrapAABB(boxAt(10, 10, 10, 10)), space.WrapAABB(boxAt(15, 10, 10, 10))},
		"across the right seam":   {space.WrapAABB(boxAt(96, 10, 10, 10)), space.WrapAABB(boxAt(2, 10, 10, 10))},
		"across the bottom seam":  {space.WrapAABB(boxAt(10, 96, 10, 10)), space.WrapAABB(boxAt(10, 2, 10, 10))},
		"fragment meets fragment": {space.WrapAABB(boxAt(98, 1, 10, 10)), space.WrapAABB(boxAt(3, 98, 10, 10))},
		"corner against corner":   {space.WrapAABB(boxAt(97, 97, 10, 10)), space.WrapAABB(boxAt(2, 2, 10, 10))},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, got := tc.a.DeepestOverlapWith(&tc.b)
			if want := meetAcrossSeams(tc.a, tc.b); got != want {
				t.Errorf("DeepestOverlapWith says %v, an image-by-image walk says %v", got, want)
			}
		})
	}
}

func TestDeepestOverlap_DeeperFragmentBeatsTheMainBoxes(t *testing.T) {
	space := NewToroidal2D(16, 16)
	a := space.WrapAABB(boxAt(0, 0, 13, 10))
	b := space.WrapAABB(boxAt(12, 0, 10, 10))

	got, ok := a.DeepestOverlapWith(&b)
	if !ok {
		t.Fatal("no overlap found")
	}
	if !got.BoxA.Equals(a.AABB) {
		t.Errorf("BoxA = %v, want a's main box — a never wraps here", got.BoxA)
	}
	if got.BoxB.TopLeft.X != 0 {
		t.Errorf("BoxB = %v, want b's wrapped part starting at x=0", got.BoxB)
	}
	if math.Abs(float64(got.Penetration.X)) <= 1 {
		t.Errorf("penetration %v across, want the fragment's deeper overlap rather than the main boxes' single unit", got.Penetration.X)
	}
}

func TestDeepestOverlap_FragmentFindsWhatTheMainBoxesMiss(t *testing.T) {
	space := NewToroidal2D(20, 20)
	a := space.WrapAABB(boxAt(15, 0, 10, 10))
	b := space.WrapAABB(boxAt(0, 0, 10, 10))

	got, ok := a.DeepestOverlapWith(&b)
	if !ok {
		t.Fatal("no contact found, though a's wrapped part stands on top of b")
	}
	if got.BoxA.TopLeft.X != 0 {
		t.Errorf("BoxA = %v, want a's wrapped part starting at x=0", got.BoxA)
	}
	if !got.BoxB.Equals(b.AABB) {
		t.Errorf("BoxB = %v, want b's main box — b never wraps here", got.BoxB)
	}
}

func TestDeepestOverlap_UnwrappedBoxesReportTheirMainBoxes(t *testing.T) {
	space := NewToroidal2D(1000, 1000)
	a := space.WrapAABB(boxAt(0, 0, 10, 10))
	b := space.WrapAABB(boxAt(8, 0, 10, 10))

	got, ok := a.DeepestOverlapWith(&b)
	if !ok {
		t.Fatal("no overlap found")
	}
	if !got.BoxA.Equals(a.AABB) || !got.BoxB.Equals(b.AABB) {
		t.Errorf("reported %v / %v, want the main boxes when neither side wraps", got.BoxA, got.BoxB)
	}
	if got.Penetration != geom.NewVec(-2, 0) {
		t.Errorf("Penetration = %v, want {-2 0}", got.Penetration)
	}
}

func TestDeepestOverlap_ReportsNothingWhenNothingMeets(t *testing.T) {
	space := NewToroidal2D(1000, 1000)
	a := space.WrapAABB(boxAt(0, 0, 10, 10))
	b := space.WrapAABB(boxAt(500, 500, 10, 10))

	if got, ok := a.DeepestOverlapWith(&b); ok {
		t.Errorf("reported an overlap %v between boxes half a world apart", got.Penetration)
	}
}
