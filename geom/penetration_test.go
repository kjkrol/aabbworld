package geom

import "testing"

func boxAt(x, y, w, h float32) AABB { return NewAABB(NewVec(x, y), NewVec(x+w, y+h)) }

func TestPenetration(t *testing.T) {
	cases := map[string]struct {
		r1, r2 AABB
		want   Vec
	}{
		"nowhere near each other": {
			boxAt(0, 0, 10, 10), boxAt(100, 100, 10, 10), Vec{},
		},
		"sharing an edge, not overlapping": {
			boxAt(0, 0, 10, 10), boxAt(10, 0, 10, 10), Vec{},
		},
		"shallow across, deep down: push sideways": {
			boxAt(0, 0, 10, 10), boxAt(8, 0, 10, 10), NewVec(-2, 0),
		},
		"shallow down, deep across: push up": {
			boxAt(0, 0, 10, 10), boxAt(0, 9, 10, 10), NewVec(0, -1),
		},
		"one box swallowed by the other: out its nearest edge": {
			boxAt(0, 0, 100, 100), boxAt(90, 40, 5, 5), NewVec(-10, 0),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := tc.r1.Penetration(tc.r2); got != tc.want {
				t.Errorf("Penetration = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPenetration_ApplyingItSeparatesTheBoxes(t *testing.T) {
	r1, r2 := boxAt(0, 0, 10, 10), boxAt(8, 4, 10, 10)
	pen := r1.Penetration(r2)

	moved := NewAABB(r1.TopLeft.Add(pen), r1.BottomRight.Add(pen))
	if moved.Penetration(r2) != (Vec{}) {
		t.Errorf("after applying %v the boxes still overlap: %v vs %v", pen, moved, r2)
	}
}
