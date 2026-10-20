package aabbworld_test

import (
	"cmp"
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/kjkrol/aabbworld"
	"github.com/kjkrol/aabbworld/collide"
	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

const pairsReach = 0.5

type idPair struct{ a, b uid.UID64 }

func sortPairs(pairs []idPair) {
	slices.SortFunc(pairs, func(l, r idPair) int {
		if c := cmp.Compare(l.a, r.a); c != 0 {
			return c
		}
		return cmp.Compare(l.b, r.b)
	})
}

// crowd is a population kept beside the space rebuilt from it, so a test
// can work out for itself who overlaps whom.
type crowd struct {
	space *aabbworld.Space
	items []aabbworld.Item
	entry entry

	records []record
	ids     []uid.UID64
	runs    []aabbworld.Run
}

// record is an entity as an ECS keeps it in a column: its box first, its capability at 100,
// 104 bytes in all.
type record struct {
	Box  plane.AABB
	Vel  [4]float64
	Kind uint32
	Caps aabbworld.Capability
}

func recordBox(r *record) *plane.AABB            { return &r.Box }
func recordCaps(r *record) *aabbworld.Capability { return &r.Caps }

// entry is a way of handing a crowd to its space: as items, as runs over records, or as runs
// over the first three quarters and items after them.
type entry int

const (
	byItems entry = iota
	byRuns
	byRunsThenItems
)

var entries = []entry{byItems, byRuns, byRunsThenItems}

func (e entry) String() string { return [...]string{"items", "runs", "runs+items"}[e] }

// runsOf cuts records into runs of uneven lengths, an empty one among them, as the chunks of
// several tables would be.
func runsOf(ids []uid.UID64, records []record, runs []aabbworld.Run) []aabbworld.Run {
	lengths := []int{1, 0, 7, 64, 33, 128}
	for i, k := 0, 0; i < len(records); k++ {
		n := min(lengths[k%len(lengths)], len(records)-i)
		runs = append(runs, aabbworld.RunOf(ids[i:i+n], records[i:i+n], recordBox, recordCaps))
		i += n
	}
	return runs
}

func newCrowd(t testing.TB, width, height uint32, edges aabbworld.Edges) *crowd {
	t.Helper()
	space, err := aabbworld.NewSpace(aabbworld.Config{
		Width: width, Height: height, Edges: edges,
		BucketSize: 64,
	})
	if err != nil {
		t.Fatalf("NewSpace: %v", err)
	}
	return &crowd{space: space}
}

func (c *crowd) add(x, y, w, h float64, caps aabbworld.Capability) {
	box := c.space.WrapAABB(plane.NewAABB(geom.NewVec(x, y), w, h).AABB)
	c.items = append(c.items, aabbworld.Item{ID: uid.UID64(len(c.items)), Box: box, Caps: caps})
}

func (c *crowd) rebuild() {
	if c.entry == byItems {
		c.space.Rebuild(c.items)
		return
	}
	n := len(c.items)
	if c.entry == byRunsThenItems {
		n -= n / 4
	}
	c.records, c.ids = c.records[:0], c.ids[:0]
	for _, it := range c.items[:n] {
		c.records = append(c.records, record{Box: it.Box, Caps: it.Caps})
		c.ids = append(c.ids, it.ID)
	}
	c.runs = runsOf(c.ids, c.records, c.runs[:0])
	c.space.RebuildRuns(c.runs, c.items[n:])
}

// scatter fills the crowd with count boxes of the given sides, anywhere in the world.
func (c *crowd) scatter(rng *rand.Rand, count int, sides []float64, whole bool) {
	width, height, _ := c.space.Bounds()
	for range count {
		w, h := sides[rng.IntN(len(sides))], sides[rng.IntN(len(sides))]
		x, y := rng.Float64()*float64(width), rng.Float64()*float64(height)
		if !c.space.Edges.WrapsX() {
			x = rng.Float64() * (float64(width) - w)
		}
		if !c.space.Edges.WrapsY() {
			y = rng.Float64() * (float64(height) - h)
		}
		if whole {
			x, y = float64(int(x)), float64(int(y))
		}
		caps := aabbworld.CanCollide
		if rng.IntN(5) == 0 {
			caps = aabbworld.Plain
		}
		c.add(x, y, w, h, caps)
	}
	c.rebuild()
}

// overlapping is every pair Tick owes a touch, worked out by comparing every two boxes.
func (c *crowd) overlapping() []idPair {
	var pairs []idPair
	for i := range c.items {
		for j := i + 1; j < len(c.items); j++ {
			if c.items[i].Caps&aabbworld.CanCollide == 0 || c.items[j].Caps&aabbworld.CanCollide == 0 {
				continue
			}
			if _, ok := c.items[i].Box.DeepestOverlapWith(&c.items[j].Box); ok {
				pairs = append(pairs, idPair{c.items[i].ID, c.items[j].ID})
			}
		}
	}
	sortPairs(pairs)
	return pairs
}

// touched lists the pairs Tick asks Touch about, refusing each so nothing is moved.
func touched(space *aabbworld.Space) []idPair {
	var pairs []idPair
	h := &hooks{touch: func(a, b uid.UID64, pen geom.Vec) (geom.Vec, bool) {
		pairs = append(pairs, idPair{a, b})
		return pen, false
	}}
	space.CollideEngine(h, collide.Config{Reach: pairsReach, Iterations: 1}).Tick()
	return pairs
}

func (c *crowd) pairs() []idPair { return touched(c.space) }

func TestBroadPhase_FindsExactlyWhoOverlaps(t *testing.T) {
	sizes := map[string][]float64{
		"one size":            {10},
		"2, 16 and 100":       {2, 16, 100},
		"wider than a bucket": {40, 150},
	}
	for _, edges := range []aabbworld.Edges{0, aabbworld.Torus, aabbworld.WrapX, aabbworld.WrapY, aabbworld.WrapX | aabbworld.OpenY, aabbworld.OpenX | aabbworld.OpenY} {
		for name, sides := range sizes {
			for _, whole := range []bool{false, true} {
				rng := rand.New(rand.NewPCG(17, 23))
				for round := range 12 {
					c := newCrowd(t, 1000, 800, edges)
					c.scatter(rng, 400, sides, whole)

					want := c.overlapping()
					var heard []idPair
					for _, e := range entries {
						c.entry = e
						c.rebuild()
						got := c.pairs()
						at := fmt.Sprintf("edges=%04b %s whole=%v round %d %v",
							edges, name, whole, round, e)
						for _, p := range got {
							if p.a.Index() >= p.b.Index() {
								t.Fatalf("%s: pair (%v, %v) is not lower index first", at, p.a, p.b)
							}
						}
						if e == byItems {
							heard = slices.Clone(got)
						} else if !slices.Equal(got, heard) {
							t.Fatalf("%s: Tick touched %d pairs, through items %d, or others",
								at, len(got), len(heard))
						}
						sortPairs(got)
						if !slices.Equal(got, want) {
							t.Fatalf("%s: Tick touched %d pairs, want %d\nmissing: %v\nextra:   %v",
								at, len(got), len(want), missing(want, got), missing(got, want))
						}
					}
				}
			}
		}
	}
}

// missing is what from holds and in does not, both sorted.
func missing(from, in []idPair) []idPair {
	var out []idPair
	left := slices.Clone(in)
	for _, p := range from {
		if i := slices.Index(left, p); i >= 0 {
			left = slices.Delete(left, i, i+1)
			continue
		}
		out = append(out, p)
		if len(out) == 8 {
			break
		}
	}
	return out
}

func TestBroadPhase_SaysTheSameThingTwice(t *testing.T) {
	for _, e := range entries {
		c := newCrowd(t, 1000, 800, aabbworld.Torus)
		c.entry = e
		c.scatter(rand.New(rand.NewPCG(1, 2)), 500, []float64{2, 16, 100}, false)

		first, second := c.pairs(), c.pairs()
		if len(first) == 0 {
			t.Fatalf("%v: a crowd this dense has to hold pairs", e)
		}
		if !slices.Equal(first, second) {
			t.Errorf("%v: two ticks over the same space named their pairs in a different order", e)
		}
	}
}

func TestBroadPhase_FollowsTheItemsItIsRebuiltFrom(t *testing.T) {
	for _, e := range entries {
		c := newCrowd(t, 1000, 800, 0)
		c.entry = e
		for x := range 4 {
			c.add(100+200*float64(x), 100, 10, 10, aabbworld.CanCollide)
		}
		c.rebuild()
		if got := c.pairs(); len(got) != 0 {
			t.Fatalf("%v: four boxes 190 apart touch: %v", e, got)
		}

		c.space.Move(&c.items[1].Box, geom.NewVec(-195, 0))
		c.space.Move(&c.items[3].Box, geom.NewVec(-195, 0))
		c.rebuild()
		if got := c.pairs(); len(got) != 2 {
			t.Errorf("%v: after moving into overlap Tick touched %v, want the two pairs", e, got)
		}

		c.items = c.items[1:]
		c.rebuild()
		if got := c.pairs(); len(got) != 1 {
			t.Errorf("%v: an item left out of Rebuild is still paired: %v", e, got)
		}
	}
}
