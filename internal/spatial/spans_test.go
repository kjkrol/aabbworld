package spatial

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/kjkrol/aabbworld/geom"
	iplane "github.com/kjkrol/aabbworld/internal/plane"
	"github.com/kjkrol/uid"
)

// answers is everything a grid tells of a scene, in the order it tells it.
type answers struct {
	pairs [][2]int32
	found []uid.UID64
}

func answersOf(sc *scene, rng *rand.Rand, on bool) answers {
	sparse = on
	sc.grid.Invalidate()
	var a answers
	sc.grid.Pairs(0.5, collides, func(x, y int32) { a.pairs = append(a.pairs, [2]int32{x, y}) })
	for range 20 {
		box := sc.surface.WrapAABB(geom.NewAABBAt(
			geom.NewVec(float32(rng.Float64()*1024)-20, float32(rng.Float64()*1024)-20),
			8+float32(rng.Float64()*200), 8+float32(rng.Float64()*200)))
		sc.grid.Query(box.AABB, AnyCapability, func(id uid.UID64) { a.found = append(a.found, id) })
		c := randomCone(rng)
		sc.grid.QueryCone(c.square(), c.origin, c.dir, c.half, c.reach, AnyCapability,
			func(id uid.UID64) { a.found = append(a.found, id) })
	}
	return a
}

func TestSpans_ListedCellsAnswerAsEveryCellWalked(t *testing.T) {
	t.Cleanup(func() { sparse = true })
	edges := map[string]iplane.Edges{
		"closed": 0, "torus": iplane.Torus, "wrapX openY": iplane.WrapX | iplane.OpenY,
	}
	for name, e := range edges {
		for _, cells := range []Resolution{Size2x2, Size4x4, Size16x16} {
			for _, count := range []int{3, 40, 600} {
				t.Run(fmt.Sprintf("%s/%s/%d", name, cells, count), func(t *testing.T) {
					rng := rand.New(rand.NewPCG(uint64(count), 29))
					sc := newScene(t, e, rng, count, []float32{4, 5, 16, 60})
					sc.grid = NewGrid(sc.surface, Size1024x1024, cells)
					sc.grid.Rebuild(sc.items)
					for round := range 8 {
						seed := rng.Uint64()
						listed := answersOf(sc, rand.New(rand.NewPCG(seed, 1)), true)
						walked := answersOf(sc, rand.New(rand.NewPCG(seed, 1)), false)
						if !slices.Equal(listed.pairs, walked.pairs) ||
							!slices.Equal(listed.found, walked.found) {
							t.Fatalf("round %d: listed %d pairs %d found, walked %d pairs"+
								" %d found, or not in the same order", round, len(listed.pairs),
								len(listed.found), len(walked.pairs), len(walked.found))
						}
						sc.step(rng)
					}
				})
			}
		}
	}
}

func TestSpans_ListedPairsAgreeWithBruteForce(t *testing.T) {
	for _, count := range []int{2, 30, 300} {
		rng := rand.New(rand.NewPCG(31, uint64(count)))
		sc := newScene(t, iplane.Torus, rng, count, []float32{4, 5, 16, 60})
		sc.grid = NewGrid(sc.surface, Size1024x1024, Size4x4)
		sc.grid.Rebuild(sc.items)
		for round := range 10 {
			want := sc.near(0.5, collides)
			var got []idPair
			sc.grid.Pairs(0.5, collides, func(x, y int32) {
				got = append(got, idPair{sc.items[x].ID, sc.items[y].ID})
			})
			sortPairs(got)
			if !slices.Equal(got, want) {
				t.Fatalf("%d boxes, round %d: grid found %d pairs, brute force %d",
					count, round, len(got), len(want))
			}
			sc.step(rng)
		}
	}
}

func TestSpans_ACellListedLastTimeIsEmptyNow(t *testing.T) {
	var s spans
	s.reset(8192)
	s.count(5)
	s.count(5)
	s.count(700)
	if n := s.place(); n != 3 || !s.listed {
		t.Fatalf("placed %d, listed %v; want 3 listed", n, s.listed)
	}
	s.reset(8192)
	s.count(9)
	s.place()
	for _, c := range []uint32{5, 700} {
		if from, to := s.entries(c); from != to {
			t.Errorf("cell %d holds %d entries after a reset", c, to-from)
		}
	}
	for c := range uint32(200) {
		s.count(c)
	}
	if s.listed {
		t.Fatal("200 cells of 8192 listed past a limit of 128")
	}
	s.place()
	s.reset(8192)
	for c := range uint32(8192) {
		if from, to := s.entries(c); from != to {
			t.Fatalf("cell %d holds %d entries after a reset from every cell walked", c, to-from)
		}
	}
}
