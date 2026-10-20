package collide

import (
	"math/rand/v2"
	"testing"

	"github.com/kjkrol/aabbworld/geom"
	iplane "github.com/kjkrol/aabbworld/internal/plane"
	"github.com/kjkrol/aabbworld/internal/spatial"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

// solveEverything is Solve measuring every pair on every pass: who moved, how often it measured.
func solveEverything(items []spatial.Item, pairs []Pair, surface *iplane.Surface, iterations int, onContact func(i int)) (movedA, movedB []bool, measured int) {
	reported := make([]bool, len(pairs))
	movedA, movedB = make([]bool, len(pairs)), make([]bool, len(pairs))
	for range iterations {
		moved := false
		for i := range pairs {
			p := &pairs[i]
			a, b := &items[p.A].Box, &items[p.B].Box
			measured++
			hit, ok := a.DeepestOverlapWith(b)
			if !ok {
				continue
			}
			if !reported[i] {
				reported[i] = true
				onContact(i)
			}
			if p.Flags&Sensor != 0 {
				continue
			}
			pushA, pushB, ok := split(hit.Penetration, p.Flags&StaticA != 0, p.Flags&StaticB != 0)
			if !ok {
				continue
			}
			if pushA != (geom.Vec{}) {
				surface.Translate(a, pushA)
				movedA[i], moved = true, true
			}
			if pushB != (geom.Vec{}) {
				surface.Translate(b, pushB)
				movedB[i], moved = true, true
			}
		}
		if !moved {
			return
		}
	}
	return
}

// crowd is a random batch dense enough that pushes chain into one another.
type crowd struct {
	items []spatial.Item
	pairs []Pair
}

func newCrowd(rng *rand.Rand, surface *iplane.Surface, count int, field float64) crowd {
	c := crowd{items: make([]spatial.Item, count)}
	pinned := make([]bool, count)
	for i := range c.items {
		box := surface.WrapAABB(plane.NewAABB(geom.NewVec(rng.Float64()*field, rng.Float64()*field), 10, 10).AABB)
		c.items[i] = spatial.Item{ID: uid.UID64(i + 1), Box: box}
		pinned[i] = rng.IntN(10) == 0
	}
	for i := range c.items {
		for j := i + 1; j < count; j++ {
			dx := c.items[i].Box.TopLeft.X - c.items[j].Box.TopLeft.X
			dy := c.items[i].Box.TopLeft.Y - c.items[j].Box.TopLeft.Y
			if dx*dx+dy*dy > 30*30 {
				continue
			}
			var flags uint8
			if pinned[i] {
				flags |= StaticA
			}
			if pinned[j] {
				flags |= StaticB
			}
			if rng.IntN(12) == 0 {
				flags |= Sensor
			}
			c.pairs = append(c.pairs, Pair{A: int32(i), B: int32(j), Flags: flags})
		}
	}
	return c
}

// entries are items where they lie, as a grid lists them.
func entries(items []spatial.Item) []spatial.Entry {
	es := make([]spatial.Entry, len(items))
	for i := range items {
		es[i] = spatial.Entry{Box: &items[i].Box, Caps: &items[i].Caps, ID: items[i].ID}
	}
	return es
}

// fresh is a copy of the crowd's items, so one crowd can be solved again from the same start.
func (c crowd) fresh() []spatial.Item {
	items := make([]spatial.Item, len(c.items))
	copy(items, c.items)
	return items
}

func TestSolve_SkippingChangesNothing(t *testing.T) {
	surfaces := map[string]*iplane.Surface{
		"bounded":  iplane.NewEuclidean2D(400, 400),
		"toroidal": iplane.NewToroidal2D(200, 200),
	}
	for name, surface := range surfaces {
		rng := rand.New(rand.NewPCG(7, 11))
		for round := range 40 {
			c := newCrowd(rng, surface, 120, 200)

			want := c.fresh()
			var wantContacts []int
			wantA, wantB, _ := solveEverything(want, c.pairs, surface, 16, func(i int) { wantContacts = append(wantContacts, i) })

			got := c.fresh()
			var gotContacts []int
			var s Solver
			s.Reset(len(got))
			for _, p := range c.pairs {
				s.Add(p)
			}
			onContact := func(i int, _ geom.Vec) { gotContacts = append(gotContacts, i) }
			s.Solve(entries(got), surface, 16, nil, onContact, nil)

			for i := range want {
				if got[i].Box.AABB != want[i].Box.AABB {
					t.Fatalf("%s round %d: box %d came to rest at %v, want %v", name, round, i, got[i].Box.AABB, want[i].Box.AABB)
				}
			}
			if len(gotContacts) != len(wantContacts) {
				t.Fatalf("%s round %d: %d contacts reported, want %d", name, round, len(gotContacts), len(wantContacts))
			}
			for k := range wantContacts {
				if gotContacts[k] != wantContacts[k] {
					t.Fatalf("%s round %d: contact %d was pair %d, want pair %d", name, round, k, gotContacts[k], wantContacts[k])
				}
			}
			for i := range s.states {
				f := s.states[i].flags
				if wantA[i] != (f&movedA != 0) || wantB[i] != (f&movedB != 0) {
					t.Fatalf("%s round %d: pair %d disagrees on which side moved", name, round, i)
				}
			}
		}
	}
}

func TestSolve_MeasuresLessThanEveryPairEveryPass(t *testing.T) {
	surface := iplane.NewEuclidean2D(400, 400)
	c := newCrowd(rand.New(rand.NewPCG(3, 5)), surface, 120, 200)

	_, _, everything := solveEverything(c.fresh(), c.pairs, surface, 16, func(int) {})

	var s Solver
	items := c.fresh()
	s.Reset(len(items))
	for _, p := range c.pairs {
		s.Add(p)
	}
	s.Solve(entries(items), surface, 16, nil, nil, nil)
	if int(s.clock) >= everything {
		t.Errorf("measured %d pairs, every pair every pass is %d — want the active set to save measurements", s.clock, everything)
	}
}
