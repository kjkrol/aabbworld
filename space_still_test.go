package aabbworld_test

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/kjkrol/aabbworld"
	"github.com/kjkrol/aabbworld/collide"
	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

func stillSpace(t *testing.T, edges aabbworld.Edges) *aabbworld.Space {
	t.Helper()
	space, err := aabbworld.NewSpace(aabbworld.Config{
		Width: 1000, Height: 1000, Edges: edges, BucketSize: 64,
	})
	if err != nil {
		t.Fatal(err)
	}
	return space
}

func box10(id uid.UID64, x, y float32, caps aabbworld.Capability) aabbworld.Item {
	return aabbworld.Item{ID: id, Box: plane.NewAABB(geom.NewVec(x, y), 10, 10), Caps: caps}
}

func found(space *aabbworld.Space, box geom.AABB) []uid.UID64 {
	var ids []uid.UID64
	space.Query(box, aabbworld.AnyCapability, func(id uid.UID64) { ids = append(ids, id) })
	slices.Sort(ids)
	return slices.Compact(ids)
}

func TestSpace_SeesAStillBoxByQueryQueryConeAndScan(t *testing.T) {
	space := stillSpace(t, 0)
	space.RebuildStill(nil, []aabbworld.Item{box10(2, 300, 95, aabbworld.Plain)})
	space.Rebuild([]aabbworld.Item{
		box10(1, 100, 95, aabbworld.CanCollide),
		box10(3, 500, 95, aabbworld.CanCollide), // behind the still box, from 1
	})

	if got := found(space, geom.NewAABBAt(geom.NewVec(290, 90), 20, 20)); !slices.Equal(got,
		[]uid.UID64{2}) {
		t.Errorf("Query finds %v over the still box, want [2]", got)
	}
	var coned []uid.UID64
	space.QueryCone(geom.NewVec(105, 100), geom.NewVec(1, 0), 0.5, 300, aabbworld.AnyCapability,
		func(id uid.UID64) { coned = append(coned, id) })
	slices.Sort(coned)
	if !slices.Equal(coned, []uid.UID64{1, 2}) {
		t.Errorf("QueryCone finds %v, want the observer and the still box", coned)
	}

	cone := aabbworld.Cone{Direction: geom.NewVec(1, 0), HalfAngle: math.Pi / 8, Radius: 600}
	var view aabbworld.View
	if !space.Scan(1, cone, &view) {
		t.Fatal("Scan from a moving observer failed")
	}
	var seen []uid.UID64
	view.Entities(func(id uid.UID64, _ float32) { seen = append(seen, id) })
	if !slices.Equal(seen, []uid.UID64{2}) {
		t.Errorf("1 sees %v, want the still box hiding 3 behind it", seen)
	}
	cone.Direction = geom.NewVec(-1, 0)
	if !space.Scan(2, cone, &view) {
		t.Fatal("Scan from a still observer failed")
	}
	seen = seen[:0]
	view.Entities(func(id uid.UID64, _ float32) { seen = append(seen, id) })
	if !slices.Equal(seen, []uid.UID64{1}) {
		t.Errorf("the still box sees %v looking back, want [1]", seen)
	}
}

// stillTick ticks an engine over space and tells who touched whom and who moved.
func stillTick(space *aabbworld.Space) (contacts []idPair, moved []uid.UID64) {
	h := &hooks{
		contact: func(a, b uid.UID64, _ geom.Vec) { contacts = append(contacts, idPair{a, b}) },
		moved:   func(id uid.UID64, _ plane.AABB) { moved = append(moved, id) },
	}
	space.CollideEngine(h, collide.Config{Reach: 0.5, Iterations: 16}).Tick()
	return contacts, moved
}

func TestTick_PushesAMovingBoxOutOfAStillOneAndNeverTheStillOne(t *testing.T) {
	for _, caps := range []aabbworld.Capability{
		aabbworld.CanCollide, aabbworld.CanCollide | aabbworld.Static,
	} {
		space := stillSpace(t, 0)
		still := []aabbworld.Item{box10(5, 100, 100, caps)}
		moving := []aabbworld.Item{box10(2, 104, 100, aabbworld.CanCollide)}
		space.RebuildStill(nil, still)
		space.Rebuild(moving)

		contacts, moved := stillTick(space)
		if !slices.Equal(contacts, []idPair{{2, 5}}) || !slices.Equal(moved, []uid.UID64{2}) {
			t.Errorf("caps %v: contacts %v, moved %v; want [{2 5}] and [2]", caps, contacts,
				moved)
		}
		if still[0].Box != plane.NewAABB(geom.NewVec(100, 100), 10, 10) {
			t.Errorf("caps %v: the still box was pushed to %v", caps, still[0].Box.AABB)
		}
		if b := moving[0].Box; b.TopLeft.X != 110 || b.TopLeft.Y != 100 {
			t.Errorf("caps %v: the moving box rests at %v, want pushed whole to (110, 100)", caps,
				b.TopLeft)
		}
	}
}

func TestTick_AStillSensorIsTouchedButPushesNothing(t *testing.T) {
	space := stillSpace(t, 0)
	space.RebuildStill(nil, []aabbworld.Item{
		box10(1, 100, 100, aabbworld.CanCollide|aabbworld.Sensor),
	})
	moving := []aabbworld.Item{box10(2, 104, 100, aabbworld.CanCollide)}
	space.Rebuild(moving)
	contacts, moved := stillTick(space)
	if !slices.Equal(contacts, []idPair{{1, 2}}) || len(moved) != 0 {
		t.Errorf("contacts %v, moved %v; want [{1 2}] and none", contacts, moved)
	}
	if moving[0].Box.TopLeft.X != 104 {
		t.Errorf("a still sensor pushed the moving box to %v", moving[0].Box.TopLeft)
	}
}

func TestTick_NeverPairsTwoStillBoxes(t *testing.T) {
	space := stillSpace(t, 0)
	still := []aabbworld.Item{
		box10(1, 100, 100, aabbworld.CanCollide), box10(2, 104, 100, aabbworld.CanCollide),
	}
	space.RebuildStill(nil, still)
	space.Rebuild([]aabbworld.Item{box10(3, 600, 600, aabbworld.CanCollide)})
	contacts, moved := stillTick(space)
	if len(contacts) != 0 || len(moved) != 0 {
		t.Errorf("contacts %v, moved %v between two still boxes, want none", contacts, moved)
	}
	if still[0].Box.TopLeft.X != 100 || still[1].Box.TopLeft.X != 104 {
		t.Errorf("still boxes pushed to %v and %v", still[0].Box.TopLeft, still[1].Box.TopLeft)
	}
}

func TestSpace_RebuildStillAgainReindexesAndRebuildRunsLeavesIt(t *testing.T) {
	space := stillSpace(t, 0)
	space.RebuildStill(nil, []aabbworld.Item{box10(1, 100, 100, aabbworld.CanCollide)})
	ids := []uid.UID64{7}
	records := []record{at(500, 500, aabbworld.CanCollide)}
	for range 3 {
		space.RebuildRuns(runOf(ids, records), nil)
		space.Query(geom.NewAABBAt(geom.NewVec(0, 0), 1000, 1000), aabbworld.AnyCapability,
			func(uid.UID64) {})
	}
	space.Rebuild(nil)
	if got := found(space, geom.NewAABBAt(geom.NewVec(95, 95), 10, 10)); !slices.Equal(got,
		[]uid.UID64{1}) {
		t.Errorf("Query finds %v after three RebuildRuns and a Rebuild, want the still box", got)
	}

	space.RebuildStill(nil, []aabbworld.Item{box10(2, 300, 300, aabbworld.CanCollide)})
	if got := found(space, geom.NewAABBAt(geom.NewVec(95, 95), 10, 10)); len(got) != 0 {
		t.Errorf("Query finds %v where the last RebuildStill left nothing", got)
	}
	if got := found(space, geom.NewAABBAt(geom.NewVec(295, 295), 10, 10)); !slices.Equal(got,
		[]uid.UID64{2}) {
		t.Errorf("Query finds %v over the new still box, want [2]", got)
	}
	var view aabbworld.View
	if space.Scan(1, aabbworld.Cone{Direction: geom.NewVec(1, 0), HalfAngle: 1, Radius: 100},
		&view) {
		t.Error("Scan from a still box the last RebuildStill left out")
	}
}

func TestTick_AStillBoxAcrossTheSeamStopsAMovingOneOnTheOtherSide(t *testing.T) {
	space := stillSpace(t, aabbworld.Torus)
	still := []aabbworld.Item{box10(1, 995, 100, aabbworld.CanCollide)}
	space.Place(&still[0].Box)
	space.RebuildStill(nil, still)
	moving := []aabbworld.Item{box10(2, 3, 100, aabbworld.CanCollide)}
	space.Rebuild(moving)

	for _, x := range []float32{1, 997} {
		if got := found(space, geom.NewAABBAt(geom.NewVec(x, 102), 1, 1)); !slices.Contains(got,
			1) {
			t.Errorf("Query at x=%v finds %v, want the still box either side of the seam", x, got)
		}
	}
	contacts, moved := stillTick(space)
	if !slices.Equal(contacts, []idPair{{1, 2}}) || !slices.Equal(moved, []uid.UID64{2}) {
		t.Errorf("contacts %v, moved %v; want [{1 2}] and [2]", contacts, moved)
	}
	if moving[0].Box.TopLeft.X != 5 {
		t.Errorf("the moving box rests at x=%v, want pushed past the still piece's end at 5",
			moving[0].Box.TopLeft.X)
	}
	if b := still[0].Box; b.TopLeft.X != 995 || b.Overhang.X == 0 {
		t.Errorf("the still box moved to %v", b)
	}
}

func TestTick_AStillBoxPastAnOpenEdgeIsNeitherFoundNorPaired(t *testing.T) {
	space := stillSpace(t, aabbworld.OpenX)
	still := []aabbworld.Item{
		box10(1, 995, 100, aabbworld.CanCollide),  // half out
		box10(2, 1100, 300, aabbworld.CanCollide), // wholly out
	}
	space.RebuildStill(nil, still)
	moving := []aabbworld.Item{box10(3, 988, 100, aabbworld.CanCollide)}
	space.Rebuild(moving)

	if got := found(space, geom.NewAABBAt(geom.NewVec(996, 101), 2, 2)); !slices.Contains(got,
		1) {
		t.Errorf("Query finds %v over the half of the still box inside, want 1 among them", got)
	}
	if got := found(space, geom.NewAABBAt(geom.NewVec(980, 290), 20, 30)); len(got) != 0 {
		t.Errorf("Query finds %v by the edge, where only a box wholly outside lies", got)
	}
	contacts, moved := stillTick(space)
	if !slices.Equal(contacts, []idPair{{1, 3}}) || !slices.Equal(moved, []uid.UID64{3}) {
		t.Errorf("contacts %v, moved %v; want [{1 3}] and [3]", contacts, moved)
	}
	if moving[0].Box.TopLeft.X != 985 {
		t.Errorf("the moving box rests at x=%v, want pushed back to 985", moving[0].Box.TopLeft.X)
	}
}

// overlaps is every pair a tick asks Touch about, refusing each, so that nothing moves.
func overlaps(space *aabbworld.Space) []idPair {
	var asked []idPair
	h := &hooks{touch: func(a, b uid.UID64, _ geom.Vec) (geom.Vec, bool) {
		asked = append(asked, idPair{a, b})
		return geom.Vec{}, false
	}}
	space.CollideEngine(h, collide.Config{Reach: 0.5, Iterations: 4}).Tick()
	slices.SortFunc(asked, func(l, r idPair) int {
		if l.a != r.a {
			return int(l.a) - int(r.a)
		}
		return int(l.b) - int(r.b)
	})
	return asked
}

func TestTick_StillBoxesOverlapWhatTheyOverlapAmongTheMovingOnes(t *testing.T) {
	for name, edges := range map[string]aabbworld.Edges{
		"closed": 0, "torus": aabbworld.Torus, "openY": aabbworld.OpenY,
	} {
		t.Run(name, func(t *testing.T) {
			rng := rand.New(rand.NewPCG(19, 23))
			var moving, still []aabbworld.Item
			for i := range 1500 {
				it := box10(uid.UID64(i+1), float32(rng.Float64()*1000),
					float32(rng.Float64()*1000), aabbworld.CanCollide)
				it.Box = plane.NewAABB(it.Box.TopLeft, 4+float32(rng.IntN(20)),
					4+float32(rng.IntN(20)))
				if i%2 == 0 {
					it.Caps |= aabbworld.Static
					still = append(still, it)
				} else {
					moving = append(moving, it)
				}
			}
			layered, flat := stillSpace(t, edges), stillSpace(t, edges)
			for i := range moving {
				layered.Place(&moving[i].Box)
			}
			for i := range still {
				layered.Place(&still[i].Box)
			}
			layered.RebuildStill(nil, still)
			layered.Rebuild(moving)
			flat.Rebuild(append(slices.Clone(moving), still...))

			var want []idPair
			for _, p := range overlaps(flat) {
				if p.a%2 == 0 || p.b%2 == 0 { // an even id is a moving box
					want = append(want, p)
				}
			}
			got := overlaps(layered)
			if len(want) == 0 || !slices.Equal(got, want) {
				t.Errorf("with a still layer %d overlaps, all moving %d without the still pairs",
					len(got), len(want))
			}
		})
	}
}
