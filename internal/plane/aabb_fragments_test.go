package plane

import (
	"github.com/kjkrol/aabbworld/plane"
	"testing"
	"unsafe"

	"github.com/kjkrol/aabbworld/geom"
)

func box(x0, y0, x1, y1 float32) geom.AABB {
	return geom.NewAABB(geom.NewVec(x0, y0), geom.NewVec(x1, y1))
}

func TestVisitFragments_RebuildsEveryPieceWhereItBelongs(t *testing.T) {
	space := NewToroidal2D(100, 100)

	cases := map[string]struct {
		x, y, w, h float32
		want       map[plane.FragPosition]geom.AABB
	}{
		"clear of every edge": {
			10, 10, 20, 20,
			nil,
		},
		"past the right edge": {
			95, 10, 20, 20,
			map[plane.FragPosition]geom.AABB{plane.FRAG_RIGHT: box(0, 10, 15, 30)},
		},
		"past the bottom edge": {
			10, 95, 20, 20,
			map[plane.FragPosition]geom.AABB{plane.FRAG_BOTTOM: box(10, 0, 30, 15)},
		},
		"through the corner": {
			95, 95, 20, 20,
			map[plane.FragPosition]geom.AABB{
				plane.FRAG_RIGHT:        box(0, 95, 15, 100),
				plane.FRAG_BOTTOM:       box(95, 0, 100, 15),
				plane.FRAG_BOTTOM_RIGHT: box(0, 0, 15, 15),
			},
		},
		"wider than the world itself": {
			10, 10, 150, 150,
			map[plane.FragPosition]geom.AABB{
				plane.FRAG_RIGHT:        box(0, 10, 60, 100),
				plane.FRAG_BOTTOM:       box(10, 0, 100, 60),
				plane.FRAG_BOTTOM_RIGHT: box(0, 0, 60, 60),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			wrapped := space.WrapAABB(geom.NewAABBAt(geom.NewVec(tc.x, tc.y), tc.w, tc.h))

			got := map[plane.FragPosition]geom.AABB{}
			wrapped.VisitFragments(func(pos plane.FragPosition, frag geom.AABB) bool {
				got[pos] = frag
				return true
			})

			if len(got) != len(tc.want) {
				t.Fatalf("got %d fragments %v, want %d %v", len(got), got, len(tc.want), tc.want)
			}
			for pos, want := range tc.want {
				if got[pos] != want {
					t.Errorf("fragment %d = %v, want %v", pos, got[pos], want)
				}
			}
		})
	}
}

func TestVisitFragments_StopsWhenAsked(t *testing.T) {
	space := NewToroidal2D(100, 100)
	wrapped := space.WrapAABB(geom.NewAABBAt(geom.NewVec(95, 95), 20, 20))

	seen := 0
	wrapped.VisitFragments(func(plane.FragPosition, geom.AABB) bool {
		seen++
		return false
	})
	if seen != 1 {
		t.Errorf("visited %d fragments after returning false, want 1", seen)
	}
}

func TestAABB_StaysSmall(t *testing.T) {
	if got := unsafe.Sizeof(plane.AABB{}); got != 32 {
		t.Errorf("AABB is %d bytes, want 32 — corners, size and overhang and nothing else", got)
	}
}
