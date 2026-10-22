package aabbworld

import (
	"math"
	"sync"
	"testing"

	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

// what one goroutine reads of the space: every box's neighbours and every observer's sight
func readSpace(r *SpaceRead, items []Item) []int {
	var view View
	cone := Cone{Direction: geom.NewVec(1, 0), HalfAngle: math.Pi / 4, Radius: 300}
	out := make([]int, 0, 2*len(items))
	for _, it := range items {
		out = append(out, r.Query(geom.NewAABBAt(it.Box.TopLeft, 120, 120), AnyCapability,
			func(uid.UID64) {}))
		seen := -1
		if r.Scan(it.ID, cone, &view) {
			seen = view.Entities(func(uid.UID64, float64) {})
		}
		out = append(out, seen)
	}
	return out
}

func TestSpaceRead_ManyGoroutinesReadAtOnceAsOneDoes(t *testing.T) {
	space, err := NewSpace(Config{Width: 2000, Height: 2000, Edges: Torus, BucketSize: 128})
	if err != nil {
		t.Fatal(err)
	}
	var items []Item
	for i := range 400 {
		x, y := float64(37*i%1990), float64(53*i%1990)
		items = append(items, Item{ID: uid.UID64(i + 1),
			Box: plane.NewAABB(geom.NewVec(x, y), 8, 8)})
	}
	space.Rebuild(items)
	want := readSpace(space.Read(), items)
	for i := range 4 {
		space.Rebuild(items) // the cells behind again: Ready or the first reader builds them
		if i%2 == 1 {
			space.Read().Ready()
		}
		got := make([][]int, 8)
		var wg sync.WaitGroup
		for g := range got {
			wg.Go(func() { got[g] = readSpace(space.Read(), items) })
		}
		wg.Wait()
		for g := range got {
			for i := range want {
				if got[g][i] != want[i] {
					t.Fatalf("goroutine %d read %d at %d, one goroutine alone %d", g, got[g][i], i,
						want[i])
				}
			}
		}
	}
}
