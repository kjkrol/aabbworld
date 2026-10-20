package bench_test

import (
	"math/rand/v2"
	"testing"

	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/internal/collide"
	iplane "github.com/kjkrol/aabbworld/internal/plane"
	"github.com/kjkrol/aabbworld/internal/spatial"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

// solverIterations caps the passes one Solve spends on chained overlaps.
const solverIterations = 16

// solverBatch is a field of boxes standing clear of their neighbours, every 25th nudged into one.
type solverBatch struct {
	home  []plane.AABB
	pairs [][2]int
}

func newSolverBatch(cols, rows int) *solverBatch {
	b := &solverBatch{}
	for r := range rows {
		for c := range cols {
			x := float64(c) * 12
			if (r*cols+c)%25 == 0 {
				x += 4
			}
			b.home = append(b.home, plane.NewAABB(geom.NewVec(x, float64(r)*12), 10, 10))
		}
	}
	for r := range rows {
		for c := range cols {
			i := r*cols + c
			if c+1 < cols {
				b.pairs = append(b.pairs, [2]int{i, i + 1})
			}
			if r+1 < rows {
				b.pairs = append(b.pairs, [2]int{i, i + cols})
			}
		}
	}
	return b
}

// Benchmark_Solver_Field solves a 100x50 field of boxes whose every neighbour pair is a candidate,
// one in twenty-five of them really overlapping.
func Benchmark_Solver_Field(b *testing.B) {
	batch := newSolverBatch(100, 50)
	surface := iplane.NewEuclidean2D(2000, 2000)

	items := make([]spatial.Item, len(batch.home))
	in := entries(items)
	var s collide.Solver
	b.ReportAllocs()
	for b.Loop() {
		for i, box := range batch.home {
			items[i] = spatial.Item{ID: uid.UID64(i + 1), Box: box}
		}
		s.Reset(len(items))
		for _, p := range batch.pairs {
			s.Add(collide.Pair{A: int32(p[0]), B: int32(p[1])})
		}
		s.Solve(in, surface, solverIterations, nil, nil, nil)
	}
	b.ReportMetric(float64(len(batch.pairs)), "pairs")
}

// entries are items where they lie, as a grid lists them.
func entries(items []spatial.Item) []spatial.Entry {
	es := make([]spatial.Entry, len(items))
	for i := range items {
		es[i] = spatial.Entry{Box: &items[i].Box, Caps: &items[i].Caps, ID: items[i].ID}
	}
	return es
}

// denseScene is the collision demo's shape: 5x5 boxes over a fifth of a torus, paired by the grid.
type denseScene struct {
	surface *iplane.Surface
	home    []spatial.Item
	pairs   []collide.Pair
}

func newDenseScene(n int) *denseScene {
	rng := rand.New(rand.NewPCG(3, 5))
	surface := iplane.NewToroidal2D(1024, 1024)
	sc := &denseScene{surface: surface}
	for i := range n {
		box := plane.NewAABB(geom.NewVec(rng.Float64()*1024, rng.Float64()*1024), 5, 5)
		surface.Translate(&box, geom.Vec{})
		sc.home = append(sc.home, spatial.Item{ID: uid.UID64(i + 1), Box: box, Caps: spatial.CanCollide})
	}
	grid := spatial.NewGrid(surface, spatial.Size1024x1024, spatial.Size16x16)
	grid.Rebuild(sc.home)
	grid.Pairs(0.5, spatial.CanCollide, func(a, b int32) {
		sc.pairs = append(sc.pairs, collide.Pair{A: a, B: b})
	})
	return sc
}

// Benchmark_Solver_Dense solves the candidate pairs of a dense crowd of 8388 boxes on a torus.
func Benchmark_Solver_Dense(b *testing.B) {
	sc := newDenseScene(8388)
	items := make([]spatial.Item, len(sc.home))
	in := entries(items)
	var s collide.Solver
	contacts := 0
	onContact := func(int, geom.Vec) { contacts++ }
	b.ReportAllocs()
	for b.Loop() {
		copy(items, sc.home)
		s.Reset(len(items))
		for _, p := range sc.pairs {
			s.Add(p)
		}
		contacts = 0
		s.Solve(in, sc.surface, solverIterations, nil, onContact, nil)
	}
	b.ReportMetric(float64(len(sc.pairs)), "pairs")
	b.ReportMetric(float64(contacts), "contacts")
}
