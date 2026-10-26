package aabbworld_test

import (
	"slices"
	"testing"

	"github.com/kjkrol/aabbworld"
	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

// boxes ahead of the eye within reach are found, those behind it, beyond it or of another
// capability are not; the seam of a torus hides none
func TestSpace_QueryCone_FindsWhatTheConeMayReach(t *testing.T) {
	for name, edges := range map[string]aabbworld.Edges{"closed": 0, "torus": aabbworld.WrapX | aabbworld.WrapY} {
		t.Run(name, func(t *testing.T) {
			space, err := aabbworld.NewSpace(aabbworld.Config{Width: 400, Height: 400,
				Edges: edges, BucketSize: 64})
			if err != nil {
				t.Fatal(err)
			}
			at := func(id uid.UID64, x, y float32, caps aabbworld.Capability) aabbworld.Item {
				return aabbworld.Item{ID: id, Box: plane.NewAABB(geom.NewVec(x, y), 10, 10),
					Caps: caps}
			}
			space.Rebuild([]aabbworld.Item{
				at(1, 150, 95, aabbworld.CanCollide),  // ahead, in reach
				at(2, 150, 95, aabbworld.Sensor),      // ahead, another capability
				at(3, 40, 95, aabbworld.CanCollide),   // behind the eye
				at(4, 300, 95, aabbworld.CanCollide),  // beyond the reach
				at(5, 150, 300, aabbworld.CanCollide), // off the cone's edge
				at(6, 390, 95, aabbworld.CanCollide),  // across the seam: behind on a torus too
			})
			var got []uid.UID64
			space.QueryCone(geom.NewVec(100, 100), geom.NewVec(1, 0), 0.5, 100,
				aabbworld.CanCollide, func(id uid.UID64) { got = append(got, id) })
			slices.Sort(got)
			got = slices.Compact(got)
			if !slices.Equal(got, []uid.UID64{1}) {
				t.Errorf("the cone reaches %v, want [1]", got)
			}
		})
	}
}
