package aabbworld_test

import (
	"fmt"
	"math"
	"slices"

	"github.com/kjkrol/aabbworld"
	"github.com/kjkrol/aabbworld/collide"
	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

// printer is a collide.Handler that confirms every pair and says what the engine did.
type printer struct{}

func (printer) Touch(_, _ uid.UID64, pen geom.Vec) (geom.Vec, bool) { return pen, true }
func (printer) Contact(a, b uid.UID64, pen geom.Vec) {
	fmt.Printf("contact %d-%d, penetration %s\n", a, b, pen)
}
func (printer) Moved(id uid.UID64, box plane.AABB) { fmt.Printf("moved %d to %s\n", id, box) }

// ExampleSpace is one tick of a small world: move, rebuild, collide, then ask who is where and
// who sees whom.
func ExampleSpace() {
	space, err := aabbworld.NewSpace(aabbworld.Config{
		Width: 100, Height: 100, Edges: aabbworld.Torus, BucketSize: 16,
	})
	if err != nil {
		panic(err)
	}

	// The world as a slice: whose box, where it lies, what may be done with it.
	items := []aabbworld.Item{
		{ID: 1, Box: plane.NewAABB(geom.NewVec(10, 10), 10, 10), Caps: aabbworld.CanCollide},
		{ID: 2, Box: plane.NewAABB(geom.NewVec(24, 10), 10, 10), Caps: aabbworld.CanCollide},
		{ID: 3, Box: plane.NewAABB(geom.NewVec(60, 10), 10, 10), Caps: aabbworld.CanCollide | aabbworld.Static},
	}
	for i := range items {
		space.Place(&items[i].Box)
	}
	engine := space.CollideEngine(printer{}, collide.Config{Reach: 0.5, Iterations: 8})

	// Tick: box 2 drifts left into box 1, the space is told, the engine separates them.
	space.Move(&items[1].Box, geom.NewVec(-6, 0))
	space.Rebuild(items)
	engine.Tick()

	// Query sees the boxes where the engine left them.
	var inQuarter []uid.UID64
	space.Query(geom.NewAABBAt(geom.NewVec(0, 0), 50, 50), aabbworld.AnyCapability, func(id uid.UID64) {
		inQuarter = append(inQuarter, id)
	})
	slices.Sort(inQuarter)
	fmt.Println("in the top-left quarter:", inQuarter)

	// Sight: what box 1 sees looking right.
	var view aabbworld.View
	cone := aabbworld.Cone{Direction: geom.NewVec(1, 0), HalfAngle: math.Pi / 8, Radius: 60}
	if space.Scan(1, cone, &view) {
		view.Entities(func(id uid.UID64, dist float32) { fmt.Printf("1 sees %d at distance %.0f\n", id, dist) })
	}
	// Output:
	// contact 1-2, penetration (-2,0)
	// moved 1 to {(9,10) (19,20)}
	// moved 2 to {(19,10) (29,20)}
	// in the top-left quarter: [1 2]
	// 1 sees 2 at distance 5
}
