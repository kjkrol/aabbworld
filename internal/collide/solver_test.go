package collide_test

import (
	iplane "github.com/kjkrol/aabbworld/internal/plane"
	"github.com/kjkrol/aabbworld/internal/spatial"
	"github.com/kjkrol/uid"
	"slices"
	"testing"

	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/internal/collide"
	"github.com/kjkrol/aabbworld/plane"
)

const iterations = 16

func at(x, y float64) plane.AABB { return plane.NewAABB(geom.NewVec(x, y), 10, 10) }

func flags(staticA, staticB, sensor bool) uint8 {
	var f uint8
	if staticA {
		f |= collide.StaticA
	}
	if staticB {
		f |= collide.StaticB
	}
	if sensor {
		f |= collide.Sensor
	}
	return f
}

func flat() *iplane.Surface { return iplane.NewEuclidean2D(1000, 1000) }

// batch is a few boxes as items, and the pairs entered over them.
type batch struct {
	items []spatial.Item
	s     collide.Solver
}

func boxes(bs ...plane.AABB) *batch {
	b := &batch{}
	for i, box := range bs {
		b.items = append(b.items, spatial.Item{ID: uid.UID64(i + 1), Box: box})
	}
	b.s.Reset(len(b.items))
	return b
}

func (b *batch) pair(i, j int, flags uint8) int {
	return b.s.Add(collide.Pair{A: int32(i), B: int32(j), Flags: flags})
}

func (b *batch) solve(surface *iplane.Surface, onContact func(i int, pen geom.Vec)) {
	b.s.Solve(entries(b.items), surface, iterations, nil, onContact, nil)
}

// entries are items where they lie, as a grid lists them.
func entries(items []spatial.Item) []spatial.Entry {
	es := make([]spatial.Entry, len(items))
	for i := range items {
		es[i] = spatial.Entry{Box: &items[i].Box, ID: items[i].ID, Caps: items[i].Caps}
	}
	return es
}

func (b *batch) box(i int) plane.AABB { return b.items[i].Box }

// depth is how far two boxes interpenetrate on their shallower axis, negative when apart.
func depth(a, b plane.AABB) float64 {
	x := min(a.BottomRight.X, b.BottomRight.X) - max(a.TopLeft.X, b.TopLeft.X)
	y := min(a.BottomRight.Y, b.BottomRight.Y) - max(a.TopLeft.Y, b.TopLeft.Y)
	return min(x, y)
}

func TestSolve_SeparatesAPairAndSplitsTheTravel(t *testing.T) {
	bt := boxes(at(100, 100), at(104, 100))
	bt.pair(0, 1, 0)
	bt.solve(flat(), nil)
	a, b := bt.box(0), bt.box(1)

	if d := depth(a, b); d > 1e-9 {
		t.Errorf("still overlapping by %v after the solve", d)
	}
	if got := 100 - a.TopLeft.X; got != 3 {
		t.Errorf("A moved %v, want 3 — half the overlap", got)
	}
	if got := b.TopLeft.X - 104; got != 3 {
		t.Errorf("B moved %v, want 3 — the other half", got)
	}
}

func TestSolve_OddPenetrationLosesNothingInTheSplit(t *testing.T) {
	bt := boxes(at(100, 100), at(107, 100))
	bt.pair(0, 1, 0)
	bt.solve(flat(), nil)
	a, b := bt.box(0), bt.box(1)

	travelled := (100 - a.TopLeft.X) + (b.TopLeft.X - 107)
	if travelled != 3 {
		t.Errorf("the pair travelled %v apart in total, want the whole 3 units of overlap", travelled)
	}
	if d := depth(a, b); d > 1e-9 {
		t.Errorf("still overlapping by %v", d)
	}
}

func TestSolve_PinnedSideNeverMoves(t *testing.T) {
	for name, tc := range map[string]struct{ staticA, staticB bool }{
		"B pinned": {false, true},
		"A pinned": {true, false},
	} {
		t.Run(name, func(t *testing.T) {
			bt := boxes(at(100, 100), at(104, 100))
			startA, startB := bt.box(0), bt.box(1)
			bt.pair(0, 1, flags(tc.staticA, tc.staticB, false))
			bt.solve(flat(), nil)
			a, b := bt.box(0), bt.box(1)

			if d := depth(a, b); d > 1e-9 {
				t.Errorf("still overlapping by %v", d)
			}
			if tc.staticA && a != startA {
				t.Errorf("pinned A moved from %v to %v", startA.AABB, a.AABB)
			}
			if tc.staticB && b != startB {
				t.Errorf("pinned B moved from %v to %v", startB.AABB, b.AABB)
			}
		})
	}
}

func TestSolve_ReportsButNeverSeparates(t *testing.T) {
	for name, f := range map[string]uint8{
		"a sensor pair":     collide.Sensor,
		"two pinned bodies": collide.StaticA | collide.StaticB,
	} {
		t.Run(name, func(t *testing.T) {
			bt := boxes(at(100, 100), at(104, 100))
			startA, startB := bt.box(0), bt.box(1)
			bt.pair(0, 1, f)

			reported := 0
			bt.solve(flat(), func(int, geom.Vec) { reported++ })

			if reported != 1 {
				t.Errorf("contact reported %d times, want exactly 1", reported)
			}
			if a, b := bt.box(0), bt.box(1); a != startA || b != startB {
				t.Error("boxes moved, though this pair must only be reported")
			}
		})
	}
}

func TestSolve_ReportsEachContactOnceEvenAcrossPasses(t *testing.T) {
	bt := boxes(at(100, 100), at(102, 100), at(104, 100))
	bt.pair(0, 1, 0)
	bt.pair(1, 2, 0)
	bt.pair(0, 2, 0)

	counts := map[int]int{}
	bt.solve(flat(), func(i int, _ geom.Vec) { counts[i]++ })

	for i := range 3 {
		if counts[i] != 1 {
			t.Errorf("pair %d reported %d times, want exactly 1", i, counts[i])
		}
	}
}

func TestSolve_KeepsGoingWhileSeparationCreatesNewOverlap(t *testing.T) {
	bt := boxes(at(100, 100), at(102, 100), at(104, 100))
	bt.pair(0, 1, 0)
	bt.pair(1, 2, 0)
	bt.pair(0, 2, 0)
	bt.solve(flat(), nil)

	for _, pair := range [][2]int{{0, 1}, {1, 2}, {0, 2}} {
		if d := depth(bt.box(pair[0]), bt.box(pair[1])); d > 1e-6 {
			t.Errorf("boxes %d and %d still overlap by %v", pair[0], pair[1], d)
		}
	}
}

func TestSolve_SeparatesAcrossAToroidalSeam(t *testing.T) {
	space := iplane.NewToroidal2D(200, 200)
	bt := boxes(
		space.WrapAABB(geom.NewAABB(geom.NewVec(196, 100), geom.NewVec(206, 110))),
		space.WrapAABB(geom.NewAABB(geom.NewVec(2, 100), geom.NewVec(12, 110))),
	)
	bt.pair(0, 1, 0)
	bt.solve(space, nil)
	a, b := bt.box(0), bt.box(1)

	if got, ok := a.DeepestOverlapWith(&b); ok {
		t.Errorf("still overlapping across the seam by %v", got.Penetration)
	}
	if moved := 196 - a.TopLeft.X; moved != 2 {
		t.Errorf("A moved %v, want 2 — the short way out", moved)
	}
}

func TestVisitMoved_NamesOnlyTheItemsThatWerePushed(t *testing.T) {
	bt := boxes(at(100, 100), at(104, 100), at(500, 500), at(600, 500), at(700, 700), at(704, 700))
	bt.pair(0, 1, 0)
	bt.pair(2, 3, 0)
	bt.pair(4, 5, collide.StaticA)
	bt.solve(flat(), nil)

	var moved []int32
	bt.s.VisitMoved(func(item int32) {
		moved = append(moved, item)
		if box := bt.box(int(item)); box.TopLeft == at(100, 100).TopLeft || box.TopLeft == at(500, 500).TopLeft {
			t.Errorf("item %d reported moved but still at its start", item)
		}
	})
	if want := []int32{0, 1, 5}; !slices.Equal(moved, want) {
		t.Errorf("moved = %v, want %v: both sides of the overlapping pair and the free side of the pinned one", moved, want)
	}
}

func TestReset_KeepsTheBuffersAndDropsTheBatch(t *testing.T) {
	bt := boxes(at(100, 100), at(104, 100))
	bt.pair(0, 1, 0)
	bt.s.Reset(len(bt.items))

	start := bt.box(0)
	bt.solve(flat(), func(int, geom.Vec) { t.Error("a reset Solver reported a contact") })
	if a := bt.box(0); a != start {
		t.Error("a reset Solver moved a box from the dropped batch")
	}
}
