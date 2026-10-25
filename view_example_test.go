package aabbworld_test

import (
	"fmt"
	"math"

	"github.com/kjkrol/aabbworld"
	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

func ExampleView() {
	space, err := aabbworld.NewSpace(aabbworld.Config{
		Width: 1000, Height: 1000,
		BucketSize: 256,
	})
	if err != nil {
		panic(err)
	}

	const guard = uid.UID64(1)
	space.Rebuild([]aabbworld.Item{
		{ID: guard, Box: plane.NewAABB(geom.NewVec(100, 100), 10, 10)},
		{ID: uid.UID64(2), Box: plane.NewAABB(geom.NewVec(300, 100), 10, 10)},
		{ID: uid.UID64(3), Box: plane.NewAABB(geom.NewVec(500, 100), 10, 10)},
	})

	cone := aabbworld.Cone{
		Direction: geom.NewVec(1.0, 0.0),
		HalfAngle: math.Pi / 6,
		Radius:    600,
	}

	var view aabbworld.View
	var fog []geom.Vec

	grew := 0
	for tick := 1; tick <= 3; tick++ {
		before := cap(fog)
		if !space.Scan(guard, cone, &view) {
			continue
		}

		view.Entities(func(id uid.UID64, dist float32) {
			if tick == 1 {
				fmt.Printf("sees %d at %.0f\n", id, dist)
			}
		})

		fog = view.Outline(0, fog[:0])
		if cap(fog) != before {
			grew++
		}
	}
	fmt.Printf("outline buffer grew %d time(s) over 3 ticks\n", grew)
	// Output:
	// sees 2 at 195
	// outline buffer grew 1 time(s) over 3 ticks
}
