//go:build goexperiment.simd && amd64

package spatial

import (
	"math"
	"math/rand/v2"
	"slices"
	"sync"
	"testing"

	"github.com/kjkrol/aabbworld/geom"
	iplane "github.com/kjkrol/aabbworld/internal/plane"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

func TestQueryCone_WideFindsWhatNarrowFinds(t *testing.T) {
	keepWide(t)
	for name, edges := range map[string]iplane.Edges{
		"closed": 0, "torus": iplane.Torus, "wrapX openY": iplane.WrapX | iplane.OpenY,
	} {
		for _, cells := range []Resolution{Size16x16, Size64x64, Size256x256} {
			t.Run(name+"/"+cells.String(), func(t *testing.T) {
				rng := rand.New(rand.NewPCG(29, 31))
				sc := newScene(t, edges, rng, 900, []float32{4, 5, 16, 60})
				sc.grid = NewGrid(sc.surface, Size1024x1024, cells)
				sc.grid.Rebuild(sc.items)
				found := 0
				for round := range 6 {
					for q := range 50 {
						c := randomCone(rng)
						want := AnyCapability
						if q%3 == 0 {
							want = collides
						}
						narrow := coneWith(sc.grid, c, want, false)
						wide := coneWith(sc.grid, c, want, true)
						if !slices.Equal(wide, narrow) {
							t.Fatalf("round %d, %+v: wide found %v, narrow %v", round, c, wide,
								narrow)
						}
						found += len(narrow)
					}
					sc.step(rng)
				}
				if found == 0 {
					t.Error("no cone found anything")
				}
			})
		}
	}
}

func TestQueryCone_WideAgreesOnEdgesAndNaN(t *testing.T) {
	keepWide(t)
	g := NewGrid(iplane.NewEuclidean2D(256, 256), Size256x256, Size32x32)
	var items []Item
	add := func(x, y, w, h float32) {
		items = append(items, Item{ID: uid.UID64(len(items) + 1),
			Box: plane.NewAABB(geom.NewVec(x, y), w, h)})
	}
	// Cells of 8, 9 and 16 pieces, so the wide walk runs whole, with a tail and twice.
	for k := range 8 {
		add(66+3*float32(k), 70, 2, 2)
	}
	for k := range 9 {
		add(98+3*float32(k), 70, 2, 2)
	}
	for k := range 16 {
		add(130+1.5*float32(k), 75, 1, 1)
	}
	// Corners on the edges of the cones below, and boxes across cells and past the grid.
	add(100, 100, 20, 20)
	add(150, 50, 10, 10)
	add(-6, -6, 8, 8)
	add(250, 250, 10, 10)
	add(1e7, 10, 4, 4)
	add(float32(math.NaN()), 30, 4, 4)
	add(30, float32(math.NaN()), 4, 4)
	g.Rebuild(items)
	for _, c := range []cone{
		{origin: geom.NewVec(64, 64), dir: geom.NewVec(1, 1), half: math.Pi / 8, reach: 200},
		{origin: geom.NewVec(64, 64), dir: geom.NewVec(1, 0), half: math.Pi / 4, reach: 120},
		{origin: geom.NewVec(120, 120), dir: geom.NewVec(-1, -1), half: 1, reach: 500},
		{origin: geom.NewVec(0, 0), dir: geom.NewVec(1, 1), half: math.Pi / 4, reach: 400},
		{origin: geom.NewVec(128, 128), dir: geom.NewVec(0, -1), half: 3, reach: 90},
		{origin: geom.NewVec(-20, 300), dir: geom.NewVec(1, -1), half: 0.7, reach: 300},
	} {
		narrow, wide := coneWith(g, c, AnyCapability, false), coneWith(g, c, AnyCapability, true)
		if len(narrow) == 0 || !slices.Equal(wide, narrow) {
			t.Errorf("%+v: wide %v, narrow %v", c, wide, narrow)
		}
	}
	nan, inf := float32(math.NaN()), float32(math.Inf(1))
	for _, c := range []cone{
		{origin: geom.NewVec(64, 64), dir: geom.NewVec(nan, 1), half: math.Pi / 8, reach: 200},
		{origin: geom.NewVec(64, 64), dir: geom.NewVec(nan, 1), half: 2, reach: 200},
		{origin: geom.NewVec(inf, 64), dir: geom.NewVec(-1, 0), half: 0.5, reach: 200},
		{origin: geom.NewVec(64, 64), dir: geom.NewVec(1, 0), half: 0.5, reach: inf},
	} {
		narrow, wide := coneWith(g, c, AnyCapability, false), coneWith(g, c, AnyCapability, true)
		if !slices.Equal(wide, narrow) {
			t.Errorf("%+v: wide %v, narrow %v", c, wide, narrow)
		}
	}
}

func TestQueryCone_ColumnsFollowTheCells(t *testing.T) {
	keepWide(t)
	surface := iplane.NewEuclidean2D(1024, 1024)
	g := NewGrid(surface, Size1024x1024, Size32x32)
	items := []Item{{ID: 1, Box: plane.NewAABB(geom.NewVec(560, 495), 10, 10)}}
	g.Rebuild(items)
	c := cone{origin: geom.NewVec(500, 500), dir: geom.NewVec(1, 0), half: 0.5, reach: 100}
	if got := coneWith(g, c, AnyCapability, true); len(got) != 1 {
		t.Fatalf("the cone finds %v ahead of it, want [1]", got)
	}
	surface.Translate(&items[0].Box, geom.NewVec(-120, 0))
	g.Invalidate()
	g.Ready()
	if g.colsBehind.Load() {
		t.Error("the cells were built again without their columns, once a cone had been asked")
	}
	if got := coneWith(g, c, AnyCapability, true); len(got) != 0 {
		t.Errorf("the cone finds %v where the box was before it moved behind", got)
	}
}

// The first cones a grid is asked build its columns once, however many goroutines ask at once.
func TestQueryCone_ManyGoroutinesAskAFreshGridAsOneDoes(t *testing.T) {
	keepWide(t)
	rng := rand.New(rand.NewPCG(37, 41))
	sc := newScene(t, iplane.Torus, rng, 600, []float32{4, 16})
	cones := make([]cone, 40)
	for i := range cones {
		cones[i] = randomCone(rng)
	}
	ask := func(g *Grid) [][]uid.UID64 {
		out := make([][]uid.UID64, len(cones))
		for i, c := range cones {
			out[i], _ = askCone(g, c.square(), c, AnyCapability)
		}
		return out
	}
	want := ask(sc.grid)
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
					t.Fatalf("round %d, goroutine %d, cone %d: %v, one goroutine alone %v", round,
						i, k, got[i][k], want[k])
				}
			}
		}
	}
}

// coneWith is what the grid reports for c over the square round it, walking wide or not.
func coneWith(g *Grid, c cone, want Capability, on bool) []uid.UID64 {
	wide = on
	got, _ := askCone(g, c.square(), c, want)
	return got
}
