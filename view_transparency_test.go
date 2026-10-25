package aabbworld_test

import (
	"math"
	"testing"

	"github.com/kjkrol/aabbworld"
	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

// A scout looks east through a forest 100 deep at a tower 400 away; at τ = 0.5 the forest costs
// twice its depth, so the tower is in reach at 500 and out of it just short of that.
func TestScan_SeesThroughAForestAtTheCostOfReach(t *testing.T) {
	const scout, forest, tower = uid.UID64(1), uid.UID64(2), uid.UID64(3)
	space, err := aabbworld.NewSpace(aabbworld.Config{Width: 1000, Height: 1000, BucketSize: 256})
	if err != nil {
		t.Fatal(err)
	}
	space.Rebuild([]aabbworld.Item{
		{ID: scout, Box: plane.NewAABB(geom.NewVec(100, 100), 10, 10)},
		{ID: forest, Box: plane.NewAABB(geom.NewVec(205, 55), 100, 100)},
		{ID: tower, Box: plane.NewAABB(geom.NewVec(505, 100), 10, 10)},
	})
	sees := func(radius float32, transparency func(uid.UID64) float32) map[uid.UID64]bool {
		var view aabbworld.View
		cone := aabbworld.Cone{Direction: geom.NewVec(1, 0), HalfAngle: math.Pi / 8, Radius: radius, Transparency: transparency}
		if !space.Scan(scout, cone, &view) {
			t.Fatal("Scan refused")
		}
		got := map[uid.UID64]bool{}
		view.Entities(func(id uid.UID64, _ float32) { got[id] = true })
		return got
	}
	forestOnly := func(id uid.UID64) float32 {
		if id == forest {
			return 0.5
		}
		return 0
	}

	if got := sees(600, nil); !got[forest] || got[tower] {
		t.Errorf("without Transparency the forest blocks: saw %v", got)
	}
	if got := sees(501, forestOnly); !got[forest] || !got[tower] {
		t.Errorf("through the forest at 501: saw %v, want the forest and the tower", got)
	}
	if got := sees(499, forestOnly); !got[forest] || got[tower] {
		t.Errorf("through the forest at 499: saw %v, want the forest alone", got)
	}
}
