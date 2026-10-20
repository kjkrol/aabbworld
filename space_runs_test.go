package aabbworld_test

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"
	"unsafe"

	"github.com/kjkrol/aabbworld"
	"github.com/kjkrol/aabbworld/collide"
	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

// untouched is what a push must leave of a record besides its box.
var untouched = record{Vel: [4]float64{1, 2, 3, 4}, Kind: 77}

func at(x, y float64, caps aabbworld.Capability) record {
	r := untouched
	r.Box, r.Caps = plane.NewAABB(geom.NewVec(x, y), 10, 10), caps
	return r
}

func runOf(ids []uid.UID64, records []record) []aabbworld.Run {
	return []aabbworld.Run{aabbworld.RunOf(ids, records, recordBox, recordCaps)}
}

func TestTick_PushesTheBoxesOfRunsAndItemsWhereTheyLie(t *testing.T) {
	space := engineSpace(t, false)
	ids := []uid.UID64{1, 2, 3}
	records := []record{
		at(100, 100, aabbworld.CanCollide), at(104, 100, aabbworld.CanCollide),
		at(500, 500, aabbworld.CanCollide),
	}
	items := []aabbworld.Item{{ID: 4, Box: plane.NewAABB(geom.NewVec(508, 500), 10, 10),
		Caps: aabbworld.CanCollide}}
	space.RebuildRuns(runOf(ids, records), items)

	moved := map[uid.UID64]plane.AABB{}
	h := &hooks{moved: func(id uid.UID64, box plane.AABB) { moved[id] = box }}
	space.CollideEngine(h, collide.Config{Reach: 0.5, Iterations: 16}).Tick()

	lying := map[uid.UID64]plane.AABB{4: items[0].Box}
	for i, id := range ids {
		lying[id] = records[i].Box
		if r := records[i]; r.Vel != untouched.Vel || r.Kind != untouched.Kind {
			t.Errorf("the push into record %d wrote past its box: %+v", i, r)
		}
	}
	if len(moved) != 4 {
		t.Fatalf("Moved heard %v, want all four boxes", moved)
	}
	for id, box := range moved {
		if lying[id] != box {
			t.Errorf("Moved told %v at %v, where it lies is %v", id, box.AABB, lying[id].AABB)
		}
	}
	if records[0].Box.Penetration(records[1].Box.AABB) != (geom.Vec{}) ||
		records[2].Box.Penetration(items[0].Box.AABB) != (geom.Vec{}) {
		t.Errorf("still overlapping: %v", lying)
	}

	var found []uid.UID64
	space.Query(geom.NewAABBAt(geom.NewVec(97.5, 101), 1, 1), aabbworld.AnyCapability,
		func(id uid.UID64) { found = append(found, id) })
	if !slices.Equal(found, []uid.UID64{1}) {
		t.Errorf("Query finds %v where the push left box 1, want it with no Rebuild", found)
	}
}

func TestTick_PushesARunOutOfTheGroundAndPastAnOpenEdgeWhereItLies(t *testing.T) {
	space, err := aabbworld.NewSpace(aabbworld.Config{
		Width: 1000, Height: 1000, Edges: aabbworld.OpenX, BucketSize: 64,
	})
	if err != nil {
		t.Fatal(err)
	}
	ids := []uid.UID64{1, 2, 3}
	records := []record{
		at(-8, 100, aabbworld.CanCollide), at(0, 100, aabbworld.CanCollide|aabbworld.Static),
		at(50, 203, aabbworld.CanCollide),
	}
	space.RebuildRuns(runOf(ids, records), nil)
	field := wallRow(20, 10, 0, 9)
	h := &fieldHooks{}
	engine := space.CollideEngine(h, collide.Config{Reach: 0.5, Iterations: 16, Field: field})
	engine.Tick()

	if got := records[2].Box.TopLeft; got != geom.NewVec(50, 190) {
		t.Errorf("the ground pushed the record to %v, want (50, 190)", got)
	}
	left := engine.Left()
	if len(left) != 1 || left[0].ID != 1 || left[0].Box != records[0].Box {
		t.Fatalf("Left = %v, want box 1 where it lies, %v", left, records[0].Box.AABB)
	}
	if records[0].Box.BottomRight.X > 0 {
		t.Errorf("box 1 lies at %v, want it wholly past the open edge", records[0].Box.AABB)
	}
	if records[1].Box != at(0, 100, 0).Box {
		t.Errorf("the static record moved to %v", records[1].Box.AABB)
	}
}

func TestQueryAndScan_SeeRunsAsTheyDoItems(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 8))
	for _, edges := range []aabbworld.Edges{0, aabbworld.Torus} {
		c := newCrowd(t, 1000, 800, edges)
		c.scatter(rng, 400, []float64{4, 16, 60}, false)
		var probes []geom.AABB
		for range 40 {
			x, y := rng.Float64()*1000, rng.Float64()*800
			probes = append(probes, geom.NewAABBAt(geom.NewVec(x, y), 20+rng.Float64()*200, 30))
		}
		cone := aabbworld.Cone{Direction: geom.NewVec(1, 0), HalfAngle: math.Pi / 3, Radius: 300}

		var heard []uid.UID64
		for _, e := range entries {
			c.entry = e
			c.rebuild()
			var got []uid.UID64
			hear := func(id uid.UID64) { got = append(got, id) }
			for _, p := range probes {
				c.space.Query(p, aabbworld.CanCollide, hear)
				c.space.Query(p, aabbworld.AnyCapability, hear)
			}
			var view aabbworld.View
			for _, observer := range []uid.UID64{0, 99, 399, 400} {
				got = append(got, observer)
				if !c.space.Scan(observer, cone, &view) {
					got = append(got, observer)
					continue
				}
				view.Entities(func(id uid.UID64, _ float64) { hear(id) })
			}
			if e == byItems {
				heard = got
				if len(heard) < 100 {
					t.Fatalf("edges=%04b: Query and Scan answer only %v", edges, heard)
				}
			} else if !slices.Equal(got, heard) {
				t.Fatalf("edges=%04b: through %v Query and Scan answer %d ids, through items %d",
					edges, e, len(got), len(heard))
			}
		}
	}
}

func TestRebuildRuns_ForgetsWhatTheLastOneLeftOut(t *testing.T) {
	space := engineSpace(t, false)
	ids := []uid.UID64{1, 2}
	records := []record{at(100, 100, aabbworld.Plain), at(300, 100, aabbworld.Plain)}
	space.RebuildRuns(runOf(ids, records), nil)
	cone := aabbworld.Cone{Direction: geom.NewVec(1, 0), HalfAngle: math.Pi / 8, Radius: 400}
	var view aabbworld.View
	if !space.Scan(1, cone, &view) {
		t.Fatal("Scan does not know the observer of a run")
	}
	space.RebuildRuns(runOf(ids[1:], records[1:]), nil)
	if space.Scan(1, cone, &view) {
		t.Error("Scan knows an observer left out of the last RebuildRuns")
	}
	if n := space.Query(geom.NewAABBAt(geom.NewVec(95, 95), 20, 20), aabbworld.AnyCapability,
		func(uid.UID64) {}); n != 0 {
		t.Errorf("Query finds %d pieces of a record left out", n)
	}
}

func TestRunOf_ReadsTheOffsetsOfTheRow(t *testing.T) {
	records := []record{at(1, 2, aabbworld.Sensor), at(3, 4, aabbworld.Static)}
	r := aabbworld.RunOf([]uid.UID64{7, 8}, records, recordBox, recordCaps)
	if r.Stride != 104 || r.BoxOff != 0 || r.CapsOff != 100 || len(r.IDs) != 2 {
		t.Errorf("RunOf = stride %d, box at %d, caps at %d, %d ids; want 104, 0, 100, 2",
			r.Stride, r.BoxOff, r.CapsOff, len(r.IDs))
	}
	if empty := aabbworld.RunOf(nil, []record{}, recordBox, recordCaps); len(empty.IDs) != 0 ||
		empty.Base != nil {
		t.Errorf("RunOf over no rows = %+v, want the zero Run", empty)
	}
}

func TestRunOf_RefusesWhatIsNotARunOfTheRows(t *testing.T) {
	records := []record{at(1, 2, aabbworld.Plain)}
	var elsewhere record
	// small is a row whose box would end past it
	type small struct {
		id   uint64
		caps aabbworld.Capability
		_    [55]byte
	}
	for name, build := range map[string]func(){
		"more ids than rows": func() {
			aabbworld.RunOf([]uid.UID64{1, 2}, records, recordBox, recordCaps)
		},
		"a box outside the row": func() {
			aabbworld.RunOf([]uid.UID64{1}, records,
				func(*record) *plane.AABB { return &elsewhere.Box }, recordCaps)
		},
		"a capability outside the row": func() {
			aabbworld.RunOf([]uid.UID64{1}, records, recordBox,
				func(*record) *aabbworld.Capability { return &elsewhere.Caps })
		},
		"a row too small for a box": func() {
			rows := make([]small, 2)
			aabbworld.RunOf([]uid.UID64{1, 2}, rows,
				func(s *small) *plane.AABB { return (*plane.AABB)(unsafe.Pointer(&s.caps)) },
				func(s *small) *aabbworld.Capability { return &s.caps })
		},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("RunOf took it")
				}
			}()
			build()
		})
	}
}
