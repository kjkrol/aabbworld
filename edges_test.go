package aabbworld_test

import (
	"math"
	"slices"
	"testing"

	"github.com/kjkrol/aabbworld"
	"github.com/kjkrol/aabbworld/collide"
	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

// world is a Space and the items it is rebuilt from, so a test can move a box and rebuild.
type world struct {
	space *aabbworld.Space
	items []aabbworld.Item
}

func edgedWorld(t *testing.T, edges aabbworld.Edges) *world {
	t.Helper()
	space, err := aabbworld.NewSpace(aabbworld.Config{Width: 1000, Height: 1000, Edges: edges, BucketSize: 64})
	if err != nil {
		t.Fatalf("NewSpace: %v", err)
	}
	return &world{space: space}
}

// put places a box for id under the edge rules and returns it; the box lives in the world's items.
func (w *world) put(id uid.UID64, box plane.AABB, caps aabbworld.Capability) *plane.AABB {
	w.space.Place(&box)
	w.items = append(w.items, aabbworld.Item{ID: id, Box: box, Caps: caps})
	return &w.items[len(w.items)-1].Box
}

// drop forgets id.
func (w *world) drop(id uid.UID64) {
	w.items = slices.DeleteFunc(w.items, func(it aabbworld.Item) bool { return it.ID == id })
}

func (w *world) rebuild() { w.space.Rebuild(w.items) }

// found is every id a query over the whole world reports, each once, in order.
func (w *world) found() []uid.UID64 {
	var ids []uid.UID64
	w.space.Query(geom.NewAABBAt(geom.NewVec(0, 0), 1000, 1000), aabbworld.AnyCapability, func(id uid.UID64) {
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	})
	slices.Sort(ids)
	return ids
}

// paired is how many overlapping pairs the engine finds, touching nothing.
func (w *world) paired() int {
	n := 0
	h := &hooks{touch: func(_, _ uid.UID64, pen geom.Vec) (geom.Vec, bool) {
		n++
		return pen, false
	}}
	w.space.CollideEngine(h, collide.Config{Reach: 0.5, Iterations: 1}).Tick()
	return n
}

func TestNewSpace_RefusesAnAxisThatBothWrapsAndIsOpen(t *testing.T) {
	for _, edges := range []aabbworld.Edges{aabbworld.WrapX | aabbworld.OpenX, aabbworld.WrapY | aabbworld.OpenY} {
		if _, err := aabbworld.NewSpace(aabbworld.Config{Width: 100, Height: 100, Edges: edges, BucketSize: 16}); err == nil {
			t.Errorf("edges %04b accepted, want an error", edges)
		}
	}
}

func TestMove_ClosedWorldKeepsTheBoxWholeAtEveryEdge(t *testing.T) {
	w := edgedWorld(t, 0)
	box := w.put(1, plane.NewAABB(geom.NewVec(500, 500), 20, 20), aabbworld.Plain)

	for _, push := range []geom.Vec{{X: -5000}, {X: 5000}, {Y: -5000}, {Y: 5000}, {X: 5000, Y: 5000}} {
		if !w.space.Move(box, push) {
			t.Fatalf("push %v: reported gone from a closed world", push)
		}
		if wd, h := box.BottomRight.X-box.TopLeft.X, box.BottomRight.Y-box.TopLeft.Y; wd != 20 || h != 20 {
			t.Fatalf("push %v: box is %vx%v, want it whole at 20x20", push, wd, h)
		}
		if box.TopLeft.X < 0 || box.TopLeft.Y < 0 || box.BottomRight.X > 1000 || box.BottomRight.Y > 1000 {
			t.Fatalf("push %v: box %v reaches outside the world", push, box)
		}
	}
}

func TestMove_EntityOnTheSeamFallsThroughTheOpenBottom(t *testing.T) {
	w := edgedWorld(t, aabbworld.WrapX|aabbworld.OpenY)
	faller, stayer := uid.UID64(1), uid.UID64(2)
	box := w.put(faller, plane.NewAABB(geom.NewVec(990, 970), 20, 20), aabbworld.CanCollide)
	w.put(stayer, plane.NewAABB(geom.NewVec(5, 975), 20, 20), aabbworld.CanCollide)
	w.rebuild()

	if box.Overhang.X != 10 {
		t.Fatalf("overhang %v, want the box wrapped 10 past the right edge", box.Overhang)
	}
	if got := w.paired(); got != 1 {
		t.Fatalf("the engine named %d pairs across the seam, want 1", got)
	}

	if !w.space.Move(box, geom.NewVec(0, 25)) {
		t.Fatal("reported gone while 5 units of it are still inside")
	}
	if w.space.Move(box, geom.NewVec(0, 5)) {
		t.Fatal("not reported gone once wholly below the open bottom")
	}
	if w.space.Move(box, geom.NewVec(0, 5)) {
		t.Fatal("reported back while still outside")
	}
	w.drop(faller)
	w.rebuild()

	if got := w.found(); !slices.Equal(got, []uid.UID64{stayer}) {
		t.Errorf("Query finds %v after the fall, want only %v", got, stayer)
	}
	if got := w.paired(); got != 0 {
		t.Errorf("the engine still names %d pairs with the fallen entity", got)
	}

	w.put(faller, plane.NewAABB(geom.NewVec(990, 970), 20, 20), aabbworld.CanCollide)
	w.rebuild()
	if got := w.found(); !slices.Equal(got, []uid.UID64{faller, stayer}) {
		t.Errorf("Query finds %v after the faller is placed again, want both", got)
	}
}

func TestMove_OpenEdgeLetsABoxComeBackWhereItWas(t *testing.T) {
	w := edgedWorld(t, aabbworld.OpenX|aabbworld.OpenY)
	box := w.put(1, plane.NewAABB(geom.NewVec(5, 500), 10, 10), aabbworld.Plain)
	start := *box

	w.space.Move(box, geom.NewVec(-8, 0))
	w.rebuild()
	if got := w.found(); len(got) != 1 {
		t.Fatalf("Query finds %v while 2 units of the box are inside, want it", got)
	}
	w.space.Move(box, geom.NewVec(8, 0))
	if *box != start {
		t.Errorf("box came back as %+v, want exactly %+v", *box, start)
	}
}

func TestTick_NamesABoxItPushedOutOfAnOpenEdge(t *testing.T) {
	w := edgedWorld(t, aabbworld.OpenX)
	wall, pushed := uid.UID64(1), uid.UID64(2)
	w.put(wall, plane.NewAABB(geom.NewVec(0, 100), 10, 10), aabbworld.CanCollide|aabbworld.Static)
	w.put(pushed, plane.NewAABB(geom.NewVec(-9, 100), 10, 10), aabbworld.CanCollide)
	w.rebuild()

	var moved []uid.UID64
	h := &hooks{moved: func(id uid.UID64, _ plane.AABB) { moved = append(moved, id) }}
	e := w.space.CollideEngine(h, collide.Config{Reach: 0.5, Iterations: 4})
	e.Tick()

	got := e.Left()
	if len(got) != 1 || got[0].ID != pushed {
		t.Fatalf("Left = %v after the wall pushed %v past the open edge, want exactly it", got, pushed)
	}
	if box := got[0].Box; box.BottomRight.X > 0 || box.Penetration(plane.NewAABB(geom.NewVec(0, 100), 10, 10).AABB) != (geom.Vec{}) {
		t.Errorf("Left names it at %v, want it wholly past the edge, clear of the wall", box.AABB)
	}
	if len(moved) != 0 {
		t.Errorf("onMoved heard %v, want nothing: the wall stood and the other box left", moved)
	}
}

func TestMoveTo_PlacesTheBoxUnderTheEdgeRules(t *testing.T) {
	t.Run("onto a wrapping seam", func(t *testing.T) {
		w := edgedWorld(t, aabbworld.Torus)
		box := w.put(1, plane.NewAABB(geom.NewVec(500, 500), 20, 20), aabbworld.Plain)
		if !w.space.MoveTo(box, geom.NewVec(990, 500)) {
			t.Fatal("reported gone from a torus")
		}
		w.rebuild()

		if box.Overhang.X != 10 {
			t.Errorf("overhang %v, want the box wrapped 10 past the right edge", box.Overhang)
		}
		for name, probe := range map[string]geom.AABB{
			"before the seam": geom.NewAABB(geom.NewVec(992, 505), geom.NewVec(998, 510)),
			"after the seam":  geom.NewAABB(geom.NewVec(2, 505), geom.NewVec(8, 510)),
		} {
			if n := w.space.Query(probe, aabbworld.AnyCapability, func(uid.UID64) {}); n != 1 {
				t.Errorf("%s: Query finds %d pieces, want the box", name, n)
			}
		}
		if n := w.space.Query(geom.NewAABB(geom.NewVec(495, 495), geom.NewVec(525, 525)), aabbworld.AnyCapability, func(uid.UID64) {}); n != 0 {
			t.Errorf("Query still finds %d pieces where the box used to be", n)
		}
	})

	t.Run("into a closed corner", func(t *testing.T) {
		w := edgedWorld(t, 0)
		box := w.put(1, plane.NewAABB(geom.NewVec(500, 500), 20, 20), aabbworld.Plain)
		w.space.MoveTo(box, geom.NewVec(5000, -5000))

		want := plane.NewAABB(geom.NewVec(980, 0), 20, 20)
		if *box != want {
			t.Errorf("box %+v, want it whole in the corner at %+v", *box, want)
		}
	})

	t.Run("past an open edge", func(t *testing.T) {
		w := edgedWorld(t, aabbworld.OpenX)
		box := w.put(1, plane.NewAABB(geom.NewVec(500, 500), 20, 20), aabbworld.Plain)
		if w.space.MoveTo(box, geom.NewVec(1500, 500)) {
			t.Fatal("not reported gone once moved past the open edge")
		}
	})
}

func TestScan_SeesThroughAWrappingSeamOnly(t *testing.T) {
	observer, target := uid.UID64(1), uid.UID64(2)
	for _, tc := range []struct {
		name      string
		edges     aabbworld.Edges
		eye, mark geom.Vec
		facing    geom.Vec
		wantSeen  bool
	}{
		{"west across a wrapping X seam", aabbworld.WrapX, geom.NewVec(20, 500), geom.NewVec(940, 500), geom.NewVec(-1, 0), true},
		{"north across a closed top", aabbworld.WrapX, geom.NewVec(500, 20), geom.NewVec(500, 940), geom.NewVec(0, -1), false},
		{"north across a wrapping Y seam", aabbworld.WrapY, geom.NewVec(500, 20), geom.NewVec(500, 940), geom.NewVec(0, -1), true},
		{"west across a closed side", aabbworld.WrapY, geom.NewVec(20, 500), geom.NewVec(940, 500), geom.NewVec(-1, 0), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := edgedWorld(t, tc.edges)
			w.put(observer, plane.NewAABB(tc.eye, 10, 10), aabbworld.Plain)
			w.put(target, plane.NewAABB(tc.mark, 10, 10), aabbworld.Plain)
			w.rebuild()

			var view aabbworld.View
			if !w.space.Scan(observer, aabbworld.Cone{Direction: tc.facing, HalfAngle: math.Pi / 6, Radius: 200}, &view) {
				t.Fatal("Scan refused")
			}
			seen := false
			view.Entities(func(id uid.UID64, _ float64) { seen = seen || id == target })
			if seen != tc.wantSeen {
				t.Errorf("target seen = %v, want %v", seen, tc.wantSeen)
			}
		})
	}
}

func TestQuery_CrossesAWrappingSeamOnly_AndFiltersByCapability(t *testing.T) {
	w := edgedWorld(t, aabbworld.WrapX)
	acrossX, acrossY, solid := uid.UID64(1), uid.UID64(2), uid.UID64(3)
	w.put(acrossX, plane.NewAABB(geom.NewVec(10, 20), 10, 10), aabbworld.Plain)
	w.put(acrossY, plane.NewAABB(geom.NewVec(960, 980), 10, 10), aabbworld.Plain)
	w.put(solid, plane.NewAABB(geom.NewVec(970, 20), 10, 10), aabbworld.Plain|aabbworld.CanCollide)
	w.rebuild()

	corner := geom.NewAABB(geom.NewVec(950, -30), geom.NewVec(1030, 40))
	ask := func(want aabbworld.Capability) []uid.UID64 {
		var got []uid.UID64
		n := w.space.Query(corner, want, func(id uid.UID64) { got = append(got, id) })
		if n != len(got) {
			t.Errorf("Query returned %d, called fn %d times", n, len(got))
		}
		slices.Sort(got)
		return got
	}

	if got := ask(aabbworld.AnyCapability); !slices.Equal(got, []uid.UID64{acrossX, solid}) {
		t.Errorf("found %v, want %v: through the X seam, never through the closed top", got, []uid.UID64{acrossX, solid})
	}
	if got := ask(aabbworld.CanCollide); !slices.Equal(got, []uid.UID64{solid}) {
		t.Errorf("found %v asking for CanCollide, want only %v", got, solid)
	}
}

func TestPlace_PutsTheBoxUnderTheEdgeRules(t *testing.T) {
	t.Run("on a wrapping seam", func(t *testing.T) {
		w := edgedWorld(t, aabbworld.Torus)
		box := w.put(1, plane.NewAABB(geom.NewVec(990, 500), 20, 20), aabbworld.Plain)
		w.rebuild()

		if box.Overhang.X != 10 {
			t.Errorf("overhang %v, want the box wrapped 10 past the right edge", box.Overhang)
		}
		across := geom.NewAABB(geom.NewVec(2, 505), geom.NewVec(8, 510))
		if n := w.space.Query(across, aabbworld.AnyCapability, func(uid.UID64) {}); n != 1 {
			t.Errorf("Query finds %d pieces across the seam, want the box", n)
		}
	})

	t.Run("past a closed corner", func(t *testing.T) {
		w := edgedWorld(t, 0)
		box := w.put(1, plane.NewAABB(geom.NewVec(990, -10), 20, 20), aabbworld.Plain)
		w.rebuild()

		want := plane.NewAABB(geom.NewVec(980, 0), 20, 20)
		if *box != want {
			t.Errorf("box %+v, want it whole in the corner at %+v", *box, want)
		}
		if got := w.found(); !slices.Equal(got, []uid.UID64{1}) {
			t.Errorf("Query finds %v, want the box", got)
		}
	})

	t.Run("past an open edge", func(t *testing.T) {
		w := edgedWorld(t, aabbworld.OpenX)
		box := plane.NewAABB(geom.NewVec(1500, 500), 20, 20)
		if w.space.Place(&box) {
			t.Fatal("accepted a box lying wholly past the open edge")
		}
	})
}
