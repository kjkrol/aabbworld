package spatial

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/kjkrol/aabbworld/geom"
	iplane "github.com/kjkrol/aabbworld/internal/plane"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

// layered is a scene whose every third item stands: the grid holds the rest by Rebuild, those
// by RebuildStill, and sc.items both, for the brute force.
type layered struct {
	*scene
	moving, still []Item
}

func newLayered(t *testing.T, edges iplane.Edges, cells Resolution, seed uint64) *layered {
	rng := rand.New(rand.NewPCG(seed, 41))
	sc := newScene(t, edges, rng, 500, []float32{4, 5, 16, 60})
	sc.grid = NewGrid(sc.surface, Size1024x1024, cells)
	l := &layered{scene: sc}
	for i, it := range sc.items {
		if i%3 == 0 {
			l.still = append(l.still, it)
		} else {
			l.moving = append(l.moving, it)
		}
	}
	sc.grid.Rebuild(l.moving)
	sc.grid.RebuildStill(nil, l.still)
	return l
}

// step moves every moving item a little and rebuilds them; the still ones stay.
func (l *layered) step(rng *rand.Rand) {
	for i := range l.moving {
		it := &l.moving[i]
		side := min(it.Box.Size.X, it.Box.Size.Y)
		l.surface.Translate(&it.Box, geom.NewVec(float32(rng.Float64()*2-1)*side/2,
			float32(rng.Float64()*2-1)*side/2))
	}
	l.items = append(append(l.items[:0], l.moving...), l.still...)
	l.grid.Rebuild(l.moving)
}

func (l *layered) id(at int32) uid.UID64 {
	if int(at) < len(l.moving) {
		return l.moving[at].ID
	}
	return l.still[int(at)-len(l.moving)].ID
}

func (l *layered) stands(id uid.UID64) bool {
	return slices.ContainsFunc(l.still, func(it Item) bool { return it.ID == id })
}

func TestGrid_PairsAStillItemWithEveryMovingOneItTouchesAndNoStillOne(t *testing.T) {
	edges := map[string]iplane.Edges{
		"closed": 0, "torus": iplane.Torus, "wrapX openY": iplane.WrapX | iplane.OpenY,
	}
	for name, e := range edges {
		for _, cells := range []Resolution{Size4x4, Size16x16, Size64x64} {
			t.Run(fmt.Sprintf("%s/%s", name, cells), func(t *testing.T) {
				l := newLayered(t, e, cells, 3)
				rng := rand.New(rand.NewPCG(5, 7))
				for round := range 12 {
					var want []idPair
					for _, p := range l.near(0.5, collides) {
						if !l.stands(p.a) || !l.stands(p.b) {
							want = append(want, p)
						}
					}
					var got []idPair
					l.grid.Pairs(0.5, collides, func(a, b int32) {
						if a >= b {
							t.Fatalf("pair (%d, %d) not lower position first", a, b)
						}
						x, y := l.id(a), l.id(b)
						if x.Index() > y.Index() {
							x, y = y, x
						}
						got = append(got, idPair{x, y})
					})
					sortPairs(got)
					if len(want) == 0 || !slices.Equal(got, want) {
						t.Fatalf("round %d: grid found %d pairs, brute force %d", round, len(got),
							len(want))
					}
					l.step(rng)
				}
			})
		}
	}
}

func TestGrid_QueriesSeeBothLayers(t *testing.T) {
	for name, e := range map[string]iplane.Edges{"closed": 0, "torus": iplane.Torus} {
		t.Run(name, func(t *testing.T) {
			l := newLayered(t, e, Size16x16, 9)
			rng := rand.New(rand.NewPCG(11, 13))
			for round := range 10 {
				for range 20 {
					box := l.surface.WrapAABB(geom.NewAABBAt(geom.NewVec(
						float32(rng.Float64()*1024)-20, float32(rng.Float64()*1024)-20),
						8+float32(rng.Float64()*120), 8+float32(rng.Float64()*120)))
					wantIDs, wantPieces := l.inside(box.AABB, AnyCapability)
					var got []uid.UID64
					n := l.grid.Query(box.AABB, AnyCapability,
						func(id uid.UID64) { got = append(got, id) })
					slices.Sort(got)
					if got = slices.Compact(got); n != wantPieces || !slices.Equal(got, wantIDs) {
						t.Fatalf("round %d: grid %d %v, brute force %d %v", round, n, got,
							wantPieces, wantIDs)
					}
					c := randomCone(rng)
					checkCone(t, l.scene, c.square(), c, AnyCapability)
				}
				l.step(rng)
			}
		})
	}
}

func TestGrid_ItemFindsAnIDInEitherLayer(t *testing.T) {
	l := newLayered(t, iplane.Torus, Size16x16, 17)
	for _, it := range append(slices.Clone(l.moving), l.still...) {
		e, ok := l.grid.Item(it.ID)
		if !ok || e.ID != it.ID || *e.Box != it.Box {
			t.Fatalf("Item(%v) = %v, %v", it.ID, e, ok)
		}
		box, ok := l.grid.EntryAABB(it.ID)
		if clamped, _ := l.grid.clamp(it.Box.AABB); !ok || box != clamped {
			t.Fatalf("EntryAABB(%v) = %v/%v, want %v", it.ID, box, ok, clamped)
		}
	}
	if len(l.grid.Items()) != len(l.moving) || len(l.grid.StillItems()) != len(l.still) {
		t.Fatalf("Items %d, StillItems %d; want %d and %d", len(l.grid.Items()),
			len(l.grid.StillItems()), len(l.moving), len(l.still))
	}
	for i, e := range l.grid.StillItems() {
		if e.ID != l.still[i].ID {
			t.Fatalf("StillItems[%d] is %v, want %v", i, e.ID, l.still[i].ID)
		}
	}
	if _, ok := l.grid.Item(uid.UID64(99999)); ok {
		t.Fatal("grid knows an entity it was never told of")
	}
}

func TestGrid_RebuildStillReplacesTheStillLayerAndRebuildLeavesIt(t *testing.T) {
	g := NewGrid(iplane.NewEuclidean2D(256, 256), Size256x256, Size32x32)
	at := func(id uid.UID64, x float32) Item {
		return Item{ID: id, Box: plane.NewAABB(geom.NewVec(x, 100), 8, 8), Caps: collides}
	}
	still := []Item{at(1, 10)}
	g.RebuildStill(nil, still)
	g.Rebuild([]Item{at(2, 14)})
	if n := pairsOf(g, 0, collides); n != 1 {
		t.Fatalf("Pairs found %d pairs, want the moving box against the still one", n)
	}
	for range 3 {
		g.Rebuild([]Item{at(3, 200)})
	}
	if n := count(g, geom.NewAABBAt(geom.NewVec(8, 98), 4, 4), AnyCapability); n != 1 {
		t.Errorf("Query finds %d pieces of the still box after three Rebuilds, want 1", n)
	}
	g.RebuildStill(nil, []Item{at(4, 150)})
	if n := count(g, geom.NewAABBAt(geom.NewVec(8, 98), 4, 4), AnyCapability); n != 0 {
		t.Errorf("Query finds %d pieces where the last RebuildStill left nothing", n)
	}
	if n := count(g, geom.NewAABBAt(geom.NewVec(148, 98), 4, 4), AnyCapability); n != 1 {
		t.Errorf("Query finds %d pieces of the new still box, want 1", n)
	}
	if _, ok := g.Item(1); ok {
		t.Error("Item knows a still box the last RebuildStill left out")
	}
	g.Rebuild([]Item{at(5, 154)})
	if n := pairsOf(g, 0, collides); n != 1 {
		t.Errorf("Pairs found %d pairs, want the new still box against the moving one", n)
	}
	g.RebuildStill(nil, nil)
	if n := pairsOf(g, 0, collides); n != 0 {
		t.Errorf("Pairs found %d pairs with the still layer emptied", n)
	}
}

func TestGrid_StillLayerIsLaidOutAgainForANewReach(t *testing.T) {
	g := NewGrid(iplane.NewEuclidean2D(256, 256), Size256x256, Size32x32)
	g.RebuildStill(nil, []Item{
		{ID: 1, Box: plane.NewAABB(geom.NewVec(100, 100), 8, 8), Caps: collides},
	})
	g.Rebuild([]Item{{ID: 2, Box: plane.NewAABB(geom.NewVec(112, 100), 8, 8), Caps: collides}})
	if n := pairsOf(g, 0.1, collides); n != 0 {
		t.Errorf("Pairs found %d pairs at a short reach, want none across a gap of 4", n)
	}
	if n := pairsOf(g, 1, collides); n != 1 {
		t.Errorf("Pairs found %d pairs at a reach of a whole side, want 1", n)
	}
	if n := pairsOf(g, 1, Sensor); n != 0 {
		t.Errorf("Pairs found %d pairs for what neither box is, want none", n)
	}
}
