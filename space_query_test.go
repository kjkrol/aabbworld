package aabbworld_test

import (
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/kjkrol/aabbworld"
	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

// A query across a seam finds every box it meets on either side, those straddling the seam
// included, and no other: crowded enough that built with GOEXPERIMENT=simd Query goes wide.
func TestSpace_Query_FindsEveryBoxAcrossTheSeams(t *testing.T) {
	const side = 512
	for name, edges := range map[string]aabbworld.Edges{
		"torus": aabbworld.Torus, "wrapX": aabbworld.WrapX,
	} {
		t.Run(name, func(t *testing.T) {
			space, err := aabbworld.NewSpace(aabbworld.Config{Width: side, Height: side,
				Edges: edges, BucketSize: 32})
			if err != nil {
				t.Fatal(err)
			}
			wrapsY := edges == aabbworld.Torus
			rng := rand.New(rand.NewPCG(79, 83))
			items := make([]aabbworld.Item, 6000)
			for i := range items {
				w, h := 2+float32(rng.IntN(5)), 2+float32(rng.IntN(5))
				y := float32(rng.IntN(side))
				if !wrapsY {
					y = float32(rng.IntN(side - 7))
				}
				caps := aabbworld.CanCollide
				if i%4 == 0 {
					caps = aabbworld.Sensor
				}
				items[i] = aabbworld.Item{ID: uid.UID64(i + 1), Caps: caps,
					Box: plane.NewAABB(geom.NewVec(float32(rng.IntN(side)), y), w, h)}
			}
			// Boxes on and across the seams, where the images are.
			for _, at := range []geom.Vec{{X: side - 2, Y: 100}, {X: 0, Y: 100}, {X: side - 1, Y: 0},
				{X: 255, Y: 0}, {X: side - 3, Y: side - 3}} {
				if !wrapsY && at.Y > side-8 {
					continue
				}
				items = append(items, aabbworld.Item{ID: uid.UID64(len(items) + 1),
					Caps: aabbworld.CanCollide, Box: plane.NewAABB(at, 6, 6)})
			}
			for i := range items {
				space.Place(&items[i].Box)
			}
			space.Rebuild(items)
			for q := range 300 {
				// Every area crosses the seam of x, and of y on a torus half the time. Its edges lie
				// half a unit off the boxes' and the seams', which meet only at a line, and none
				// begins in the world's last unit, which the grid holds to its edge.
				x := float32(side-30+rng.IntN(28)) + 0.5
				y := float32(rng.IntN(side-80)) + 0.5
				if wrapsY && q%2 == 0 {
					y = float32(side-30+rng.IntN(28)) + 0.5
				}
				area := geom.NewAABBAt(geom.NewVec(x, y), float32(32+rng.IntN(48)),
					float32(32+rng.IntN(48)))
				want := aabbworld.CanCollide
				if q%3 == 0 {
					want = aabbworld.AnyCapability
				}
				var got []uid.UID64
				n := space.Query(area, want, func(id uid.UID64) { got = append(got, id) })
				if n != len(got) {
					t.Fatalf("%+v: Query counted %d and reported %d", area, n, len(got))
				}
				slices.Sort(got)
				got = slices.Compact(got)
				if wantIDs := meeting(items, area, want, side, wrapsY); !slices.Equal(got, wantIDs) {
					t.Fatalf("%+v, want %v: found %v, the brute force %v", area, want, got, wantIDs)
				}
			}
		})
	}
}

// meeting lists, by brute force, every item sharing want whose box, or the box a world over along
// a wrapping axis, touches area.
func meeting(items []aabbworld.Item, area geom.AABB, want aabbworld.Capability, side float32,
	wrapsY bool) []uid.UID64 {
	shiftsY := []float32{0}
	if wrapsY {
		shiftsY = []float32{-side, 0, side}
	}
	var ids []uid.UID64
	for _, it := range items {
		if want != aabbworld.AnyCapability && it.Caps&want == 0 {
			continue
		}
		// The whole box from where it begins: its main part stops at the seam.
		b := geom.NewAABBAt(it.Box.TopLeft, it.Box.Size.X, it.Box.Size.Y)
	shifts:
		for _, dx := range []float32{-side, 0, side} {
			for _, dy := range shiftsY {
				if area.Intersects(geom.NewAABB(b.TopLeft.Add(geom.NewVec(dx, dy)),
					b.BottomRight.Add(geom.NewVec(dx, dy)))) {
					ids = append(ids, it.ID)
					break shifts
				}
			}
		}
	}
	slices.Sort(ids)
	return ids
}
