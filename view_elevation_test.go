package aabbworld_test

import (
	"math"
	"testing"

	"github.com/kjkrol/aabbworld"
	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

// A scout and a hawk look east at a wall 10 tall with a walker behind it: the scout, eye 1.5 up,
// sees the wall alone; the hawk, 41 up, sees over it.
func TestScan_SightHasHeights(t *testing.T) {
	const scout, wall, walker = uid.UID64(1), uid.UID64(2), uid.UID64(3)
	space, err := aabbworld.NewSpace(aabbworld.Config{Width: 1000, Height: 1000, BucketSize: 256})
	if err != nil {
		t.Fatal(err)
	}
	space.Rebuild([]aabbworld.Item{
		{ID: scout, Box: plane.NewAABB(geom.NewVec(100, 100), 10, 10)},
		{ID: wall, Box: plane.NewAABB(geom.NewVec(305, 55), 10, 100)},
		{ID: walker, Box: plane.NewAABB(geom.NewVec(505, 100), 10, 10)},
	})
	heights := map[uid.UID64][2]float32{wall: {0, 10}, walker: {0, 2}}
	sees := func(eye float32) map[uid.UID64]bool {
		var view aabbworld.View
		cone := aabbworld.Cone{
			Direction: geom.NewVec(1, 0), HalfAngle: math.Pi / 8, Radius: 600,
			Eye:       eye,
			Elevation: func(id uid.UID64) (float32, float32) { return heights[id][0], heights[id][1] },
		}
		if !space.Scan(scout, cone, &view) {
			t.Fatal("Scan refused")
		}
		got := map[uid.UID64]bool{}
		view.Entities(func(id uid.UID64, _ float32) { got[id] = true })
		return got
	}

	if got := sees(1.5); !got[wall] || got[walker] {
		t.Errorf("from 1.5 up: saw %v, want the wall alone", got)
	}
	if got := sees(41); !got[wall] || !got[walker] {
		t.Errorf("from 41 up: saw %v, want the wall and the walker behind it", got)
	}
}
