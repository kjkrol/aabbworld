//go:build goexperiment.simd && (amd64 || arm64 || wasm)

package spatial

import (
	"math"
	"math/rand/v2"
	"slices"
	"sync"
	"testing"

	"github.com/kjkrol/aabbworld/geom"
	iplane "github.com/kjkrol/aabbworld/internal/plane"
	"github.com/kjkrol/aabbworld/internal/simdx"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

func TestQuery_WideFindsWhatNarrowFinds(t *testing.T) {
	keepWideQuery(t)
	for name, edges := range map[string]iplane.Edges{
		"closed": 0, "torus": iplane.Torus, "wrapX openY": iplane.WrapX | iplane.OpenY,
	} {
		for _, cells := range []Resolution{Size16x16, Size64x64, Size256x256} {
			t.Run(name+"/"+cells.String(), func(t *testing.T) {
				rng := rand.New(rand.NewPCG(43, 47))
				sc := newScene(t, edges, rng, 900, []float32{4, 5, 16, 60})
				sc.grid = NewGrid(sc.surface, Size1024x1024, cells)
				sc.grid.Rebuild(sc.items)
				found := 0
				for round := range 6 {
					for q := range 50 {
						box := randomArea(rng)
						want := AnyCapability
						if q%3 == 0 {
							want = collides
						}
						narrow := queryWith(sc.grid, box, want, false)
						wide := queryWith(sc.grid, box, want, true)
						if !slices.Equal(wide, narrow) {
							t.Fatalf("round %d, %+v: wide found %v, narrow %v", round, box, wide,
								narrow)
						}
						found += len(narrow)
					}
					sc.step(rng)
					sc.grid.Rebuild(sc.items)
				}
				if found == 0 {
					t.Error("no query found anything")
				}
			})
		}
	}
}

func TestQuery_WideAgreesOnEdgesAndNaN(t *testing.T) {
	keepWideQuery(t)
	g := NewGrid(iplane.NewEuclidean2D(256, 256), Size256x256, Size32x32)
	var items []Item
	add := func(x, y, w, h float32) {
		items = append(items, Item{ID: uid.UID64(len(items) + 1),
			Box: plane.NewAABB(geom.NewVec(x, y), w, h)})
	}
	// Cells of 4, 8, 9, 16 and 17 pieces, so every vector width runs whole, with a tail and twice.
	for _, k := range []int{4, 8, 9, 16, 17} {
		for i := range k {
			add(float32(32*k%224)+1.5*float32(i%16), float32(4*k)+float32(i/16)*2, 1, 1)
		}
	}
	// Boxes on cell edges, across cells, past the grid and with NaN corners.
	add(32, 32, 32, 32)
	add(100, 100, 20, 20)
	add(-6, -6, 8, 8)
	add(250, 250, 10, 10)
	add(1e7, 10, 4, 4)
	add(float32(math.NaN()), 30, 4, 4)
	add(30, float32(math.NaN()), 4, 4)
	g.Rebuild(items)
	box := func(x1, y1, x2, y2 float32) geom.AABB {
		return geom.NewAABB(geom.NewVec(x1, y1), geom.NewVec(x2, y2))
	}
	nan, inf := float32(math.NaN()), float32(math.Inf(1))
	for _, b := range []geom.AABB{
		box(0, 0, 256, 256), box(32, 32, 64, 64), box(64, 64, 64, 64), box(31.5, 0, 32, 100),
		box(-100, -100, 0, 0), box(200, 0, 1e9, 1e9), box(nan, 0, 100, 100), box(0, 0, inf, inf),
		box(0, 0, 0, 0),
	} {
		narrow, wide := queryWith(g, b, AnyCapability, false), queryWith(g, b, AnyCapability, true)
		if !slices.Equal(wide, narrow) {
			t.Errorf("%+v: wide %v, narrow %v", b, wide, narrow)
		}
	}
}

// After a wide query the grid builds its columns with its cells, so a moved box is found where it
// went, not where it was.
func TestQuery_ColumnsFollowTheCells(t *testing.T) {
	keepWideQuery(t)
	surface := iplane.NewEuclidean2D(1024, 1024)
	g := NewGrid(surface, Size1024x1024, Size32x32)
	items := []Item{{ID: 1, Box: plane.NewAABB(geom.NewVec(560, 495), 10, 10)}}
	g.Rebuild(items)
	at := geom.NewAABB(geom.NewVec(550, 490), geom.NewVec(580, 510))
	if got := queryWith(g, at, AnyCapability, true); len(got) != 1 {
		t.Fatalf("the query finds %v, want [1]", got)
	}
	surface.Translate(&items[0].Box, geom.NewVec(-120, 0))
	g.Invalidate()
	g.Ready()
	if g.colsBehind.Load() {
		t.Error("the cells were built again without their columns, once a wide query had been asked")
	}
	if got := queryWith(g, at, AnyCapability, true); len(got) != 0 {
		t.Errorf("the query finds %v where the box was before it moved", got)
	}
}

// Query goes wide only where the occupied cells hold a vector of pieces each on average.
func TestQuery_WideOnlyOverCrowdedCells(t *testing.T) {
	keepWideQuery(t)
	queryWideFrom, wideQuery = 1, true
	n := lanes32()
	g := NewGrid(iplane.NewEuclidean2D(256, 256), Size256x256, Size32x32)
	var items []Item
	for i := range n - 1 {
		items = append(items, Item{ID: uid.UID64(i + 1),
			Box: plane.NewAABB(geom.NewVec(1+float32(i), 1), 0.5, 0.5)})
	}
	g.Rebuild(items)
	all := geom.NewAABB(geom.NewVec(0, 0), geom.NewVec(256, 256))
	g.Query(all, AnyCapability, func(uid.UID64) {})
	if !g.colsBehind.Load() {
		t.Error("a cell of fewer pieces than a vector was queried wide")
	}
	items = append(items, Item{ID: uid.UID64(n),
		Box: plane.NewAABB(geom.NewVec(float32(n), 1), 0.5, 0.5)})
	g.Rebuild(items)
	if ids, got := askQuery(g, all, AnyCapability); got != n || len(ids) != n {
		t.Errorf("the query counted %d and reported %v, want %d", got, ids, n)
	}
	if g.colsBehind.Load() {
		t.Error("a cell of a vector of pieces was not queried wide")
	}
}

// With AVX2 Query stays plain, there being no AVX2 kernel for it; elsewhere it goes wide on
// simd's vectors.
func TestQuery_WideOnSimdsVectorsOnlyWithoutAVX2(t *testing.T) {
	if got, want := wideQuery, simdx.On() && !simdx.ArchFirst(); got != want {
		t.Errorf("wideQuery = %v where simd's vectors run %v and AVX2 %v", got, simdx.On(),
			simdx.ArchFirst())
	}
}

// The first wide queries a grid is asked build its columns once, however many goroutines ask at
// once.
func TestQuery_ManyGoroutinesAskAFreshGridAsOneDoes(t *testing.T) {
	keepWideQuery(t)
	rng := rand.New(rand.NewPCG(53, 59))
	sc := newScene(t, iplane.Torus, rng, 600, []float32{4, 16})
	areas := make([]geom.AABB, 40)
	for i := range areas {
		areas[i] = randomArea(rng)
	}
	ask := func(g *Grid) [][]uid.UID64 {
		out := make([][]uid.UID64, len(areas))
		for i, a := range areas {
			out[i], _ = askQuery(g, a, AnyCapability)
		}
		return out
	}
	wideQuery = false
	want := ask(sc.grid)
	wideQuery = true
	fresh := NewGrid(sc.surface, Size1024x1024, Size16x16)
	for round := range 2 {
		fresh.Rebuild(sc.items)
		got := make([][][]uid.UID64, 8)
		var wg sync.WaitGroup
		for i := range got {
			wg.Go(func() { got[i] = ask(fresh) })
		}
		wg.Wait()
		for i := range got {
			for k := range want {
				if !slices.Equal(got[i][k], want[k]) {
					t.Fatalf("round %d, goroutine %d, area %d: %v, the plain query %v", round, i,
						k, got[i][k], want[k])
				}
			}
		}
	}
}

// A cone and a wide query read the same columns: whichever asks first builds them for the other,
// and both follow the cells as they are built again.
func TestQuery_ColumnsServeConesAndQueriesAlike(t *testing.T) {
	keepWideQuery(t)
	if !simdx.On() && !simdx.ArchFirst() {
		t.Skip("no wide kernel runs here")
	}
	wasWide := wide
	t.Cleanup(func() { wide = wasWide })
	rng := rand.New(rand.NewPCG(61, 67))
	sc := newScene(t, iplane.Torus, rng, 600, []float32{4, 16})
	plain := func(c cone, a geom.AABB) ([]uid.UID64, []uid.UID64) {
		wide, wideQuery = false, false
		defer func() { wide, wideQuery = true, true }()
		cs, _ := askCone(sc.grid, c.square(), c, AnyCapability)
		qs, _ := askQuery(sc.grid, a, AnyCapability)
		return cs, qs
	}
	wide = true
	for round := range 4 {
		c, a := randomCone(rng), randomArea(rng)
		sc.grid.Rebuild(sc.items)
		wantCone, wantQuery := plain(c, a)
		// Built plain, the cells are built again wide: with their columns once either has asked.
		sc.grid.Invalidate()
		var gotCone, gotQuery []uid.UID64
		if round%2 == 0 {
			gotCone, _ = askCone(sc.grid, c.square(), c, AnyCapability)
			gotQuery, _ = askQuery(sc.grid, a, AnyCapability)
		} else {
			gotQuery, _ = askQuery(sc.grid, a, AnyCapability)
			gotCone, _ = askCone(sc.grid, c.square(), c, AnyCapability)
		}
		if !slices.Equal(gotCone, wantCone) {
			t.Errorf("round %d: the cone found %v, the plain cone %v", round, gotCone, wantCone)
		}
		if !slices.Equal(gotQuery, wantQuery) {
			t.Errorf("round %d: the query found %v, the plain query %v", round, gotQuery, wantQuery)
		}
		sc.step(rng)
	}
}

// keepWideQuery skips t where vectors are emulated, has Query go wide however sparse the cells,
// with AVX2 too, and puts wideQuery and queryWideFrom back as t found them.
func keepWideQuery(t *testing.T) {
	if !simdx.On() {
		t.Skip("the simd package is emulated here")
	}
	was, wasFrom, wasSparse := wideQuery, queryWideFrom, sparse
	// A sparse layout lists its cells and answers from the list, before the vectors.
	queryWideFrom, sparse = 0, false
	t.Cleanup(func() { wideQuery, queryWideFrom, sparse = was, wasFrom, wasSparse })
}

// queryWith is what the grid reports for box, querying wide or not.
func queryWith(g *Grid, box geom.AABB, want Capability, on bool) []uid.UID64 {
	wideQuery = on
	got, _ := askQuery(g, box, want)
	return got
}

// askQuery is the ids the grid reports for box, and how many it counts.
func askQuery(g *Grid, box geom.AABB, want Capability) (ids []uid.UID64, n int) {
	n = g.Query(box, want, func(id uid.UID64) { ids = append(ids, id) })
	return ids, n
}

// randomArea is a box up to 300 a side about the 1024 by 1024 world, spilling past its edges.
func randomArea(rng *rand.Rand) geom.AABB {
	at := geom.NewVec(float32(rng.Float64()*1224-100), float32(rng.Float64()*1224-100))
	return geom.NewAABB(at, at.Add(geom.NewVec(float32(rng.Float64()*300),
		float32(rng.Float64()*300))))
}
