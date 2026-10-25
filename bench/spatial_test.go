package bench_test

import (
	"math/rand/v2"
	"testing"

	"github.com/kjkrol/aabbworld/geom"
	iplane "github.com/kjkrol/aabbworld/internal/plane"
	"github.com/kjkrol/aabbworld/internal/spatial"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

// gridScene is a population on a torus, held by a grid and by the slice the grid was built from.
type gridScene struct {
	surface *iplane.Surface
	grid    *spatial.Grid
	items   []spatial.Item
}

// newGridScene scatters count boxes of the given sides over a 1024x1024 torus, two in three of
// them CanCollide, the same way the spatial package's own tests do.
func newGridScene(rng *rand.Rand, count int, sides []float32) *gridScene {
	surface := iplane.NewSurface(1024, 1024, iplane.Torus)
	sc := &gridScene{surface: surface, grid: spatial.NewGrid(surface, spatial.Size1024x1024, spatial.Size16x16)}
	for i := range count {
		w, h := sides[rng.IntN(len(sides))], sides[rng.IntN(len(sides))]
		box := plane.NewAABB(geom.NewVec(float32(rng.Float64()*1024)-w/2, float32(rng.Float64()*1024)-h/2), w, h)
		surface.Translate(&box, geom.Vec{})
		if surface.Left(&box) {
			continue
		}
		caps := spatial.Plain
		if rng.IntN(3) != 0 {
			caps |= spatial.CanCollide
		}
		sc.items = append(sc.items, spatial.Item{ID: uid.UID64(i + 1), Box: box, Caps: caps})
	}
	sc.grid.Rebuild(sc.items)
	return sc
}

// Benchmark_Grid_RebuildPairsQuery is one tick of the index: every box moves a little, the grid
// is rebuilt, every pair within reach is found, and eight 100x100 probes are answered.
func Benchmark_Grid_RebuildPairsQuery(b *testing.B) {
	for _, cfg := range []struct {
		name  string
		n     int
		sides []float32
	}{
		{"n=2000,5x5", 2000, []float32{5}},
		{"n=8388,5x5", 8388, []float32{5}},
		{"n=8388,mixed", 8388, []float32{4, 5, 16, 60}},
	} {
		b.Run(cfg.name, func(b *testing.B) {
			rng := rand.New(rand.NewPCG(3, 5))
			sc := newGridScene(rng, cfg.n, cfg.sides)
			deltas := make([]geom.Vec, len(sc.items))
			for i, it := range sc.items {
				side := min(it.Box.Size.X, it.Box.Size.Y)
				deltas[i] = geom.NewVec(float32(rng.Float64()*2-1)*side/2, float32(rng.Float64()*2-1)*side/2)
			}
			var probes []geom.AABB
			for range 8 {
				probes = append(probes, geom.NewAABBAt(geom.NewVec(float32(rng.Float64()*900), float32(rng.Float64()*900)), 100, 100))
			}
			pairs, found := 0, 0
			onPair := func(_, _ int32) { pairs++ }
			onFound := func(uid.UID64) { found++ }
			b.ReportAllocs()
			for b.Loop() {
				for i := range sc.items {
					sc.surface.Translate(&sc.items[i].Box, deltas[i])
				}
				sc.grid.Rebuild(sc.items)
				sc.grid.Pairs(0.5, spatial.CanCollide, onPair)
				for _, p := range probes {
					sc.grid.Query(p, spatial.CanCollide, onFound)
				}
			}
		})
	}
}
