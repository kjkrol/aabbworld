package spatial

import (
	"math/rand/v2"
	"slices"
	"testing"
	"unsafe"

	"github.com/kjkrol/aabbworld/geom"
	iplane "github.com/kjkrol/aabbworld/internal/plane"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

// record is an entity as an ECS keeps it in a column: its box first, its capability at 100.
type record struct {
	box  plane.AABB
	_    [36]byte
	caps Capability
}

// runsOver copies items into records cut into runs of uneven lengths, the last quarter left as
// items after them.
func runsOver(items []Item) ([]Run, []Item) {
	tail := len(items) - len(items)/4
	records, ids := make([]record, tail), make([]uid.UID64, tail)
	for i, it := range items[:tail] {
		records[i], ids[i] = record{box: it.Box, caps: it.Caps}, it.ID
	}
	var runs []Run
	var r record
	for i, k := 0, 0; i < tail; k++ {
		n := min([]int{3, 0, 50, 17}[k%4], tail-i)
		runs = append(runs, Run{
			IDs: ids[i : i+n], Base: unsafe.Pointer(&records[i]), Stride: unsafe.Sizeof(r),
			BoxOff: unsafe.Offsetof(r.box), CapsOff: unsafe.Offsetof(r.caps),
		})
		i += n
	}
	return runs, items[tail:]
}

func TestGrid_OverRunsAnswersAsOverItems(t *testing.T) {
	for name, edges := range map[string]iplane.Edges{"closed": 0, "torus": iplane.Torus} {
		t.Run(name, func(t *testing.T) {
			rng := rand.New(rand.NewPCG(9, 4))
			sc := newScene(t, edges, rng, 500, []float64{4, 16, 60})
			byRuns := NewGrid(sc.surface, Size1024x1024, Size16x16)
			runs, tail := runsOver(sc.items)
			byRuns.RebuildRuns(runs, tail)

			for q := range 30 {
				box := geom.NewAABBAt(geom.NewVec(rng.Float64()*1000, rng.Float64()*1000), 90, 60)
				var want, got []uid.UID64
				sc.grid.Query(box, collides, func(id uid.UID64) { want = append(want, id) })
				byRuns.Query(box, collides, func(id uid.UID64) { got = append(got, id) })
				if !slices.Equal(got, want) {
					t.Fatalf("query %d: over runs %v, over items %v", q, got, want)
				}
			}
			var want, got [][2]int32
			sc.grid.Pairs(0.5, collides, func(a, b int32) { want = append(want, [2]int32{a, b}) })
			byRuns.Pairs(0.5, collides, func(a, b int32) { got = append(got, [2]int32{a, b}) })
			if len(want) == 0 || !slices.Equal(got, want) {
				t.Fatalf("Pairs over runs %d, over items %d, or others", len(got), len(want))
			}
			entries := byRuns.Items()
			if len(entries) != len(sc.items) {
				t.Fatalf("Items over runs lists %d, want %d", len(entries), len(sc.items))
			}
			for at, it := range sc.items {
				e := entries[at]
				if e.ID != it.ID || *e.Box != it.Box || *e.Caps != it.Caps {
					t.Fatalf("entry %d is %v %v %v, want %v", at, e.ID, *e.Box, *e.Caps, it)
				}
				if box, ok := byRuns.EntryAABB(it.ID); !ok || box != sc.mustEntryAABB(t, it.ID) {
					t.Fatalf("EntryAABB(%v) over runs = %v/%v", it.ID, box, ok)
				}
			}
		})
	}
}

func (sc *scene) mustEntryAABB(t *testing.T, id uid.UID64) geom.AABB {
	box, ok := sc.grid.EntryAABB(id)
	if !ok {
		t.Fatalf("the grid over items does not know %v", id)
	}
	return box
}

func TestGrid_ListsAShorterRebuildWithoutTheRest(t *testing.T) {
	g := NewGrid(iplane.NewEuclidean2D(256, 256), Size256x256, Size32x32)
	items := []Item{
		{ID: 1, Box: plane.NewAABB(geom.NewVec(10, 10), 8, 8)},
		{ID: 2, Box: plane.NewAABB(geom.NewVec(100, 100), 8, 8)},
	}
	g.Rebuild(items)
	if n := len(g.Items()); n != 2 {
		t.Fatalf("Items lists %d, want 2", n)
	}
	g.Rebuild(items[1:])
	if es := g.Items(); len(es) != 1 || es[0].ID != 2 || es[:2][1] != (Entry{}) {
		t.Errorf("Items after a shorter Rebuild = %v, want item 2 and nothing past it", es[:2])
	}
}
