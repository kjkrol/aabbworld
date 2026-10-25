package spatial

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/kjkrol/aabbworld/geom"
	iplane "github.com/kjkrol/aabbworld/internal/plane"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

const collides = Capability(1 << 1)

// scene is a population on a surface, held by a grid and by the slice the grid was built from.
type scene struct {
	surface *iplane.Surface
	grid    *Grid
	items   []Item
}

func newScene(t testing.TB, edges iplane.Edges, rng *rand.Rand, count int, sides []float32) *scene {
	t.Helper()
	surface := iplane.NewSurface(1024, 1024, edges)
	sc := &scene{surface: surface, grid: NewGrid(surface, Size1024x1024, Size16x16)}
	for i := range count {
		w, h := sides[rng.IntN(len(sides))], sides[rng.IntN(len(sides))]
		box := plane.NewAABB(geom.NewVec(float32(rng.Float64()*1024)-w/2, float32(rng.Float64()*1024)-h/2), w, h)
		surface.Translate(&box, geom.Vec{})
		if surface.Left(&box) {
			continue
		}
		caps := Plain
		if rng.IntN(3) != 0 {
			caps |= collides
		}
		sc.items = append(sc.items, Item{ID: uid.UID64(i + 1), Box: box, Caps: caps})
	}
	sc.grid.Rebuild(sc.items)
	return sc
}

// step moves every item by a small random delta and rebuilds.
func (sc *scene) step(rng *rand.Rand) {
	kept := sc.items[:0]
	for i := range sc.items {
		it := &sc.items[i]
		side := min(it.Box.Size.X, it.Box.Size.Y)
		sc.surface.Translate(&it.Box, geom.NewVec(float32(rng.Float64()*2-1)*side/2, float32(rng.Float64()*2-1)*side/2))
		if !sc.surface.Left(&it.Box) {
			kept = append(kept, *it)
		}
	}
	sc.items = kept
	sc.grid.Rebuild(sc.items)
}

// inside lists, by brute force, every item sharing want whose main box or image intersects box.
func (sc *scene) inside(box geom.AABB, want Capability) (ids []uid.UID64, pieces int) {
	area, ok := sc.grid.clamp(box)
	if !ok {
		return nil, 0
	}
	for _, it := range sc.items {
		if !it.Caps.matches(want) {
			continue
		}
		hit := false
		for _, img := range images(it.Box) {
			if clamped, ok := sc.grid.clamp(img); ok && clamped.Intersects(area) {
				pieces++
				hit = true
			}
		}
		if hit {
			ids = append(ids, it.ID)
		}
	}
	slices.Sort(ids)
	return ids, pieces
}

func TestGrid_AgreesWithBruteForce(t *testing.T) {
	for name, edges := range map[string]iplane.Edges{"closed": 0, "torus": iplane.Torus, "wrapX openY": iplane.WrapX | iplane.OpenY} {
		t.Run(name, func(t *testing.T) {
			rng := rand.New(rand.NewPCG(7, 11))
			sc := newScene(t, edges, rng, 600, []float32{4, 5, 16, 60})
			for round := range 40 {
				for q := range 30 {
					w, h := 8+float32(rng.Float64()*120), 8+float32(rng.Float64()*120)
					box := sc.surface.WrapAABB(geom.NewAABBAt(geom.NewVec(float32(rng.Float64()*1024)-20, float32(rng.Float64()*1024)-20), w, h))
					want := collides
					if rng.IntN(2) == 0 {
						want = AnyCapability
					}
					wantIDs, wantPieces := sc.inside(box.AABB, want)
					var got []uid.UID64
					n := sc.grid.Query(box.AABB, want, func(id uid.UID64) { got = append(got, id) })
					slices.Sort(got)
					got = slices.Compact(got)
					if n != wantPieces || !slices.Equal(got, wantIDs) {
						t.Fatalf("round %d query %d: grid %d %v, brute force %d %v", round, q, n, got, wantPieces, wantIDs)
					}
				}

				want := sc.near(0.5, collides)
				var got []idPair
				sc.grid.Pairs(0.5, collides, func(x, y int32) { got = append(got, idPair{sc.items[x].ID, sc.items[y].ID}) })
				sortPairs(got)
				if !slices.Equal(got, want) {
					t.Fatalf("round %d: grid found %d pairs, brute force %d", round, len(got), len(want))
				}

				for _, it := range sc.items {
					box, ok := sc.grid.EntryAABB(it.ID)
					if clamped, _ := sc.grid.clamp(it.Box.AABB); !ok || box != clamped {
						t.Fatalf("round %d: EntryAABB(%v) = %v/%v, want %v", round, it.ID, box, ok, clamped)
					}
				}
				if _, ok := sc.grid.EntryAABB(uid.UID64(99999)); ok {
					t.Fatal("grid knows an entity it was never told of")
				}
				sc.step(rng)
			}
		})
	}
}

func TestGrid_IsEmptyBeforeItsFirstRebuild(t *testing.T) {
	g := NewGrid(iplane.NewToroidal2D(256, 256), Size256x256, Size32x32)
	if n := g.Query(geom.NewAABBAt(geom.NewVec(0, 0), 256, 256), AnyCapability, func(uid.UID64) {}); n != 0 {
		t.Errorf("a fresh grid answered a query with %d pieces", n)
	}
	g.Pairs(0.5, AnyCapability, func(int32, int32) { t.Error("a fresh grid paired something") })
	if _, ok := g.EntryAABB(1); ok {
		t.Error("a fresh grid knows an entity")
	}
}

func TestGrid_QueryFollowsABoxMovedSinceRebuild(t *testing.T) {
	surface := iplane.NewEuclidean2D(256, 256)
	g := NewGrid(surface, Size256x256, Size32x32)
	items := []Item{{ID: 1, Box: plane.NewAABB(geom.NewVec(10, 10), 8, 8), Caps: Plain}}
	g.Rebuild(items)
	if n := g.Query(geom.NewAABBAt(geom.NewVec(0, 0), 32, 32), AnyCapability, func(uid.UID64) {}); n != 1 {
		t.Fatalf("Query finds %d pieces where the item was put, want 1", n)
	}

	surface.Translate(&items[0].Box, geom.NewVec(100, 100))
	g.Invalidate()

	if n := g.Query(geom.NewAABBAt(geom.NewVec(0, 0), 32, 32), AnyCapability, func(uid.UID64) {}); n != 0 {
		t.Errorf("Query finds %d pieces where the item used to be", n)
	}
	if n := g.Query(geom.NewAABBAt(geom.NewVec(96, 96), 32, 32), AnyCapability, func(uid.UID64) {}); n != 1 {
		t.Errorf("Query finds %d pieces where the item moved to, want 1", n)
	}
}

func TestGrid_ForgetsWhatTheLastRebuildLeftOut(t *testing.T) {
	surface := iplane.NewEuclidean2D(256, 256)
	g := NewGrid(surface, Size256x256, Size32x32)
	items := []Item{
		{ID: 1, Box: plane.NewAABB(geom.NewVec(10, 10), 8, 8), Caps: Plain},
		{ID: 2, Box: plane.NewAABB(geom.NewVec(100, 100), 8, 8), Caps: Plain},
	}
	g.Rebuild(items)
	g.Rebuild(items[1:])

	if _, ok := g.EntryAABB(1); ok {
		t.Error("an item left out of the last Rebuild is still known")
	}
	if n := g.Query(geom.NewAABBAt(geom.NewVec(0, 0), 32, 32), AnyCapability, func(uid.UID64) {}); n != 0 {
		t.Errorf("Query finds %d pieces where the left-out item was", n)
	}
	if n := g.Query(geom.NewAABBAt(geom.NewVec(96, 96), 32, 32), AnyCapability, func(uid.UID64) {}); n != 1 {
		t.Errorf("Query finds %d pieces where the kept item is, want 1", n)
	}
}

func TestGrid_RefusesCoordinatesWithNoCell(t *testing.T) {
	g := NewGrid(iplane.NewEuclidean2D(256, 256), Size256x256, Size32x32)
	for name, v := range map[string]geom.Vec{
		"negative":          geom.NewVec(-5, -5),
		"NaN":               geom.NewVec(float32(math.NaN()), 10),
		"beyond the grid":   geom.NewVec(1e7, 10),
		"positive infinity": geom.NewVec(float32(math.Inf(1)), 10),
	} {
		t.Run(name, func(t *testing.T) {
			if c := g.cellOf(v.X); c != 0 && c != g.side-1 {
				t.Errorf("cellOf(%v) = %d, want it held at an edge cell", v.X, c)
			}
			if _, ok := g.clamp(geom.NewAABBAt(v, 4, 4)); ok && (v.X < 0 || math.IsNaN(float64(v.X))) {
				box, _ := g.clamp(geom.NewAABBAt(v, 4, 4))
				if box.TopLeft.X != 0 {
					t.Errorf("clamp put a box with no cell at %v", box)
				}
			}
		})
	}
}

func TestGrid_HoldsItemsAtFractionalPositions(t *testing.T) {
	g := NewGrid(iplane.NewEuclidean2D(256, 256), Size256x256, Size32x32)
	g.Rebuild([]Item{{ID: 1, Box: plane.NewAABB(geom.NewVec(41.7, 43.2), 3.5, 3.5), Caps: Plain}})
	if n := g.Query(geom.NewAABBAt(geom.NewVec(40, 40), 10, 10), AnyCapability, func(uid.UID64) {}); n != 1 {
		t.Errorf("Query finds %d pieces, want the fractionally placed item", n)
	}
}

func TestGrid_AnItemSpanningFourCellsIsReportedOnce(t *testing.T) {
	g := NewGrid(iplane.NewEuclidean2D(256, 256), Size256x256, Size32x32)
	g.Rebuild([]Item{{ID: 1, Box: plane.NewAABB(geom.NewVec(28, 28), 8, 8), Caps: Plain}})
	seen := 0
	if n := g.Query(geom.NewAABBAt(geom.NewVec(0, 0), 64, 64), AnyCapability, func(uid.UID64) { seen++ }); n != 1 || seen != 1 {
		t.Errorf("a box across four cells was reported %d times (count %d), want once", seen, n)
	}
}

func TestGrid_QuerySeesOnlyWhatItAskedFor(t *testing.T) {
	g := NewGrid(iplane.NewToroidal2D(256, 256), Size256x256, Size32x32)
	g.Rebuild([]Item{
		{ID: 1, Box: plane.NewAABB(geom.NewVec(100, 100), 8, 8), Caps: Plain},
		{ID: 2, Box: plane.NewAABB(geom.NewVec(112, 100), 8, 8), Caps: Plain | collides},
	})
	box := geom.NewAABBAt(geom.NewVec(90, 90), 40, 40)
	var got []uid.UID64
	g.Query(box, collides, func(id uid.UID64) { got = append(got, id) })
	if !slices.Equal(got, []uid.UID64{2}) {
		t.Errorf("asking for collides found %v, want [2]", got)
	}
	got = got[:0]
	g.Query(box, AnyCapability, func(id uid.UID64) { got = append(got, id) })
	slices.Sort(got)
	if !slices.Equal(got, []uid.UID64{1, 2}) {
		t.Errorf("asking for anything found %v, want both", got)
	}
}

// count is how many pieces Query finds in box.
func count(g *Grid, box geom.AABB, want Capability) int {
	return g.Query(box, want, func(uid.UID64) {})
}

// pairsOf is how many pairs Pairs reports for reach and want.
func pairsOf(g *Grid, reach float32, want Capability) int {
	n := 0
	g.Pairs(reach, want, func(int32, int32) { n++ })
	return n
}

func TestGrid_QueryReadsActualBoxesFromAGridGrownForPairs(t *testing.T) {
	g := NewGrid(iplane.NewToroidal2D(256, 256), Size256x256, Size32x32)
	g.Rebuild([]Item{{ID: 1, Box: plane.NewAABB(geom.NewVec(100, 100), 8, 8), Caps: collides}})
	pairsOf(g, 0.5, collides) // the cells now hold the box grown by 4 on every side

	if n := count(g, geom.NewAABBAt(geom.NewVec(110, 100), 8, 8), AnyCapability); n != 0 {
		t.Errorf("Query finds %d pieces in the grown margin, where the box is not", n)
	}
	if n := count(g, geom.NewAABBAt(geom.NewVec(104, 100), 8, 8), AnyCapability); n != 1 {
		t.Errorf("Query finds %d pieces over the box, want 1", n)
	}
}

func TestGrid_PairsSkipsWhatCannotCollide(t *testing.T) {
	g := NewGrid(iplane.NewEuclidean2D(256, 256), Size256x256, Size32x32)
	g.Rebuild([]Item{
		{ID: 1, Box: plane.NewAABB(geom.NewVec(100, 100), 8, 8), Caps: Plain},
		{ID: 2, Box: plane.NewAABB(geom.NewVec(104, 100), 8, 8), Caps: collides},
	})
	if n := pairsOf(g, 0.5, collides); n != 0 {
		t.Errorf("Pairs found %d pairs, want none when one side cannot collide", n)
	}
	if n := pairsOf(g, 0.5, AnyCapability); n != 1 {
		t.Errorf("Pairs found %d pairs for anything, want 1", n)
	}
}

func TestGrid_AGrownImageWithNoActualPieceIsNeverQueried(t *testing.T) {
	g := NewGrid(iplane.NewToroidal2D(256, 256), Size256x256, Size32x32)
	box := plane.NewAABB(geom.NewVec(250, 10), 6, 6) // ends exactly at the seam, no fragment
	g.Rebuild([]Item{{ID: 1, Box: box, Caps: collides}})
	pairsOf(g, 0.5, collides) // grown by 3, it now wraps to [0,3) past the seam

	if n := count(g, geom.NewAABBAt(geom.NewVec(0, 8), 4, 4), AnyCapability); n != 0 {
		t.Errorf("Query finds %d pieces past the seam, where only the grown image is", n)
	}
	if n := count(g, geom.NewAABBAt(geom.NewVec(248, 8), 4, 4), AnyCapability); n != 1 {
		t.Errorf("Query finds %d pieces over the box, want 1", n)
	}
}

func TestGrid_ABoxAtTheLeftEdgeIsFoundWhenGrownAcrossTheSeam(t *testing.T) {
	g := NewGrid(iplane.NewToroidal2D(256, 256), Size256x256, Size32x32)
	g.Rebuild([]Item{{ID: 1, Box: plane.NewAABB(geom.NewVec(0, 10), 8, 8), Caps: collides}})
	pairsOf(g, 0.5, collides) // grown by 4, its main image now starts at 252, past the seam

	if n := count(g, geom.NewAABBAt(geom.NewVec(0, 8), 4, 4), AnyCapability); n != 1 {
		t.Errorf("Query finds %d pieces over the box at the left edge, want 1", n)
	}
	if n := count(g, geom.NewAABBAt(geom.NewVec(248, 8), 4, 4), AnyCapability); n != 0 {
		t.Errorf("Query finds %d pieces at the right edge, where only the grown image is", n)
	}
}

func TestGrid_PairsWithANewReachReindexes(t *testing.T) {
	g := NewGrid(iplane.NewEuclidean2D(256, 256), Size256x256, Size32x32)
	g.Rebuild([]Item{
		{ID: 1, Box: plane.NewAABB(geom.NewVec(100, 100), 8, 8), Caps: collides},
		{ID: 2, Box: plane.NewAABB(geom.NewVec(112, 100), 8, 8), Caps: collides},
	})
	if n := pairsOf(g, 0.1, collides); n != 0 {
		t.Errorf("Pairs found %d pairs at a short reach, want none across a gap of 4", n)
	}
	if n := pairsOf(g, 1, collides); n != 1 {
		t.Errorf("Pairs found %d pairs at a reach of a whole side, want 1", n)
	}
}

// near is every pair Pairs owes, worked out by comparing every two grown boxes, images and all.
func (sc *scene) near(reach float32, want Capability) []idPair {
	grown := make([]plane.AABB, len(sc.items))
	for i, it := range sc.items {
		grown[i] = it.Box
		sc.surface.Expand(&grown[i], reach*min(it.Box.Size.X, it.Box.Size.Y))
	}
	var pairs []idPair
	for i := range sc.items {
		if !sc.items[i].Caps.matches(want) {
			continue
		}
		for j := i + 1; j < len(sc.items); j++ {
			if !sc.items[j].Caps.matches(want) || !meet(grown[i], grown[j]) {
				continue
			}
			a, b := sc.items[i].ID, sc.items[j].ID
			if a.Index() > b.Index() {
				a, b = b, a
			}
			pairs = append(pairs, idPair{a, b})
		}
	}
	sortPairs(pairs)
	return pairs
}

func images(ab plane.AABB) []geom.AABB {
	out := []geom.AABB{ab.AABB}
	ab.VisitFragments(func(_ plane.FragPosition, box geom.AABB) bool {
		out = append(out, box)
		return true
	})
	return out
}

func meet(a, b plane.AABB) bool {
	for _, ia := range images(a) {
		for _, ib := range images(b) {
			if ia.Intersects(ib) {
				return true
			}
		}
	}
	return false
}

type idPair struct{ a, b uid.UID64 }

func sortPairs(pairs []idPair) {
	slices.SortFunc(pairs, func(l, r idPair) int {
		if l.a != r.a {
			if l.a < r.a {
				return -1
			}
			return 1
		}
		if l.b < r.b {
			return -1
		}
		if l.b > r.b {
			return 1
		}
		return 0
	})
}
