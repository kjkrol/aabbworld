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

func querySeam(t *testing.T, edges aabbworld.Edges, boxes ...plane.AABB) func(geom.AABB) []uid.UID64 {
	t.Helper()
	space, err := aabbworld.NewSpace(aabbworld.Config{Width: 512, Height: 512, Edges: edges,
		BucketSize: 32})
	if err != nil {
		t.Fatal(err)
	}
	items := make([]aabbworld.Item, len(boxes))
	for i, b := range boxes {
		items[i] = aabbworld.Item{ID: uid.UID64(i + 1), Caps: aabbworld.CanCollide, Box: b}
		space.Place(&items[i].Box)
	}
	space.Rebuild(items)
	return func(area geom.AABB) []uid.UID64 {
		var got []uid.UID64
		space.Query(area, aabbworld.CanCollide, func(id uid.UID64) { got = append(got, id) })
		slices.Sort(got)
		return got
	}
}

func TestSpace_Query_InTheWorldsLastUnitMeetsOnlyWhatReachesIntoIt(t *testing.T) {
	ask := querySeam(t, 0,
		plane.NewAABB(geom.NewVec(505, 100), 6, 6),     // 1: ends at 511
		plane.NewAABB(geom.NewVec(506, 200), 6, 6),     // 2: ends at 512, the world's edge
		plane.NewAABB(geom.NewVec(511.5, 300), 0.5, 6)) // 3: within the last unit
	for name, c := range map[string]struct {
		area geom.AABB
		want []uid.UID64
	}{
		"beside the first":     {geom.NewAABB(geom.NewVec(511.5, 100), geom.NewVec(511.9, 106)), nil},
		"touching the first":   {geom.NewAABB(geom.NewVec(511, 100), geom.NewVec(511.9, 106)), []uid.UID64{1}},
		"into the second":      {geom.NewAABB(geom.NewVec(511.5, 200), geom.NewVec(511.9, 206)), []uid.UID64{2}},
		"the third":            {geom.NewAABB(geom.NewVec(511.6, 300), geom.NewVec(511.8, 306)), []uid.UID64{3}},
		"past the edge":        {geom.NewAABB(geom.NewVec(600, 0), geom.NewVec(700, 512)), nil},
		"up to the edge, wide": {geom.NewAABB(geom.NewVec(511.5, 0), geom.NewVec(600, 512)), []uid.UID64{2, 3}},
	} {
		if got := ask(c.area); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v found %v, want %v", name, c.area, got, c.want)
		}
	}
}

func TestSpace_Query_OnTheSeamMeetsWhatBeginsOnItsOtherSide(t *testing.T) {
	ask := querySeam(t, aabbworld.Torus,
		plane.NewAABB(geom.NewVec(0, 100), 6, 6),   // 1: begins on the seam
		plane.NewAABB(geom.NewVec(506, 200), 6, 6), // 2: ends on the seam
		plane.NewAABB(geom.NewVec(0, 0), 6, 6),     // 3: in the corner
		plane.NewAABB(geom.NewVec(506, 506), 6, 6)) // 4: in the far corner
	for name, c := range map[string]struct {
		area geom.AABB
		want []uid.UID64
	}{
		"ending on the seam":    {geom.NewAABB(geom.NewVec(500, 100), geom.NewVec(512, 106)), []uid.UID64{1}},
		"short of the seam":     {geom.NewAABB(geom.NewVec(500, 100), geom.NewVec(511.9, 106)), nil},
		"beginning on the seam": {geom.NewAABB(geom.NewVec(0, 200), geom.NewVec(20, 206)), []uid.UID64{2}},
		"off the seam":          {geom.NewAABB(geom.NewVec(0.1, 200), geom.NewVec(20, 206)), nil},
		"ending in the corner":  {geom.NewAABB(geom.NewVec(500, 500), geom.NewVec(512, 512)), []uid.UID64{3, 4}},
		"from the corner":       {geom.NewAABB(geom.NewVec(0, 0), geom.NewVec(20, 20)), []uid.UID64{3, 4}},
		"the whole width":       {geom.NewAABB(geom.NewVec(0, 100), geom.NewVec(512, 106)), []uid.UID64{1}},
	} {
		if got := ask(c.area); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v found %v, want %v", name, c.area, got, c.want)
		}
	}
}
