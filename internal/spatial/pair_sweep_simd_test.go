//go:build goexperiment.simd && amd64

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

func TestPairSweep_WideFindsWhatNarrowFinds(t *testing.T) {
	keepWide(t)
	for name, edges := range map[string]iplane.Edges{"closed": 0, "torus": iplane.Torus, "wrapX openY": iplane.WrapX | iplane.OpenY} {
		for _, cells := range []Resolution{Size16x16, Size64x64, Size256x256} {
			t.Run(name+"/"+cells.String(), func(t *testing.T) {
				rng := rand.New(rand.NewPCG(13, 17))
				sc := newScene(t, edges, rng, 900, []float64{4, 5, 16, 60})
				sc.grid = NewGrid(sc.surface, Size1024x1024, cells)
				sc.grid.Rebuild(sc.items)
				for round := range 10 {
					narrow, wide := pairsWith(sc.grid, 0.5, false), pairsWith(sc.grid, 0.5, true)
					if len(narrow) == 0 || !slices.Equal(wide, narrow) {
						t.Fatalf("round %d: wide found %d pairs, narrow %d, or not in the same order", round, len(wide), len(narrow))
					}
					sc.step(rng)
				}
			})
		}
	}
}

func TestPairSweep_WideAgreesOnEdgesAndNaN(t *testing.T) {
	keepWide(t)
	g := NewGrid(iplane.NewEuclidean2D(256, 256), Size256x256, Size32x32)
	var items []Item
	add := func(x, y, w, h float64) {
		items = append(items, Item{ID: uid.UID64(len(items) + 1), Box: plane.NewAABB(geom.NewVec(x, y), w, h), Caps: collides})
	}
	// Boxes touching edge to edge and corner to corner, across the corner of four cells.
	for y := 24.0; y < 40; y += 4 {
		for x := 24.0; x < 40; x += 4 {
			add(x, y, 4, 4)
		}
	}
	add(30.5, 30.5, 3, 3)
	// A crowded first cell, with two boxes reaching past the grid's start.
	for k := range 8 {
		add(4+3*float64(k), 12, 2, 2)
	}
	add(-6, -6, 8, 8)
	add(-4, -5, 8, 8)
	add(250, 250, 10, 10)
	add(1e9, 10, 4, 4)
	add(math.NaN(), 30, 4, 4)
	add(30, math.NaN(), 4, 4)
	g.Rebuild(items)
	for _, reach := range []float64{0, 0.5} {
		narrow, wide := pairsWith(g, reach, false), pairsWith(g, reach, true)
		if len(narrow) == 0 || !slices.Equal(wide, narrow) {
			t.Errorf("reach %v: wide %v, narrow %v", reach, wide, narrow)
		}
	}
}

func TestPairSweep_WideNeverPairsABoxWithItself(t *testing.T) {
	keepWide(t)
	surface := iplane.NewSurface(64, 64, iplane.Torus)
	g := NewGrid(surface, Size64x64, Size64x64)
	var items []Item
	add := func(x, y, w, h float64) {
		box := plane.NewAABB(geom.NewVec(x, y), w, h)
		surface.Translate(&box, geom.Vec{})
		items = append(items, Item{ID: uid.UID64(len(items) + 1), Box: box, Caps: collides})
	}
	// As wide as the world, so its two pieces touch across the seam.
	add(1, 20, 64, 4)
	for k := range 8 {
		add(4+6*float64(k), 30, 3, 3)
	}
	g.Rebuild(items)
	narrow, wide := pairsWith(g, 0, false), pairsWith(g, 0, true)
	if !slices.Equal(wide, narrow) {
		t.Errorf("wide %v, narrow %v", wide, narrow)
	}
}

// keepWide puts wide back as the test found it.
func keepWide(t *testing.T) {
	if !wide {
		t.Skip("the CPU has no AVX2")
	}
	t.Cleanup(func() { wide = true })
}

func pairsWith(g *Grid, reach float64, on bool) [][2]int32 {
	wide = on
	var got [][2]int32
	g.Pairs(reach, collides, func(a, b int32) { got = append(got, [2]int32{a, b}) })
	return got
}
