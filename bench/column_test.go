package bench_test

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/kjkrol/aabbworld"
	"github.com/kjkrol/aabbworld/collide"
	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

// record is an entity as gram keeps it in a column of goke: its box first, then its velocity,
// its kind and its capability at 68, 72 bytes in all.
type record struct {
	Box  plane.AABB
	Vel  [4]float64
	Kind uint32
	Caps aabbworld.Capability
}

func recordBox(r *record) *plane.AABB            { return &r.Box }
func recordCaps(r *record) *aabbworld.Capability { return &r.Caps }

// chunkRecords is about how many records of gram's size one chunk of goke holds.
const chunkRecords = 128

// columnScene is records on a torus, each moving its own way every tick.
type columnScene struct {
	space   *aabbworld.Space
	records []record
	ids     []uid.UID64
	probes  []geom.AABB
}

func newColumnScene(b *testing.B, side, bucket uint32) *columnScene {
	b.Helper()
	space, err := aabbworld.NewSpace(aabbworld.Config{
		Width: side, Height: side, Edges: aabbworld.Torus, BucketSize: bucket,
	})
	if err != nil {
		b.Fatal(err)
	}
	sc := &columnScene{space: space}
	rng := rand.New(rand.NewPCG(13, 21))
	for range 8 {
		at := geom.NewVec(float32(rng.Float64()*float64(side-100)), float32(rng.Float64()*float64(side-100)))
		sc.probes = append(sc.probes, geom.NewAABBAt(at, 100, 100))
	}
	return sc
}

func (sc *columnScene) put(box plane.AABB, dx, dy float64) {
	sc.space.Place(&box)
	r := record{Box: box, Vel: [4]float64{dx, dy, 1, 0}, Kind: 1, Caps: aabbworld.CanCollide}
	sc.records = append(sc.records, r)
	sc.ids = append(sc.ids, uid.UID64(len(sc.ids)+1))
}

// spreadScene is gram's world benchmark: 5,000 boxes 20x20, 30 apart on a 4000x4000 torus, all
// moving the same way, so none ever meet.
func spreadScene(b *testing.B) *columnScene {
	sc := newColumnScene(b, 4000, 64)
	for i := range 5000 {
		sc.put(plane.NewAABB(geom.NewVec(float32(10+i%71*30), float32(10+i/71*30)), 20, 20), 0.5, 0)
	}
	return sc
}

// crowdScene is 5,000 boxes 6x6 over a sixth of a 1024x1024 torus, each heading its own way at a
// unit a tick, run for a hundred ticks so it is past the first overlaps.
func crowdScene(b *testing.B) *columnScene {
	sc := newColumnScene(b, 1024, 16)
	rng := rand.New(rand.NewPCG(3, 8))
	for range 5000 {
		heading := rng.Float64() * 2 * math.Pi
		at := geom.NewVec(float32(rng.Float64()*1024), float32(rng.Float64()*1024))
		sc.put(plane.NewAABB(at, 6, 6), math.Cos(heading), math.Sin(heading))
	}
	engine := sc.space.CollideEngine(&writeBack{records: sc.records}, columnCollide)
	items := make([]aabbworld.Item, len(sc.records))
	for range 100 {
		sc.move()
		for i := range sc.records {
			r := &sc.records[i]
			items[i] = aabbworld.Item{ID: sc.ids[i], Box: r.Box, Caps: r.Caps}
		}
		sc.space.Rebuild(items)
		engine.Tick()
	}
	return sc
}

var columnCollide = collide.Config{Reach: 0.5, Iterations: 16}

func (sc *columnScene) move() {
	for i := range sc.records {
		r := &sc.records[i]
		sc.space.Move(&r.Box, geom.NewVec(float32(r.Vel[0]*r.Vel[2]), float32(r.Vel[1]*r.Vel[2])))
	}
}

func (sc *columnScene) query() int {
	found := 0
	for _, p := range sc.probes {
		found += sc.space.Query(p, aabbworld.CanCollide, func(uid.UID64) {})
	}
	return found
}

// writeBack is a Handler for records handed over as copies: Moved writes the box back.
type writeBack struct {
	records  []record
	contacts int
}

func (w *writeBack) Touch(_, _ uid.UID64, pen geom.Vec) (geom.Vec, float32, bool) {
	return pen, 0.5, true
}
func (w *writeBack) Contact(uid.UID64, uid.UID64, geom.Vec) { w.contacts++ }
func (w *writeBack) Moved(id uid.UID64, box plane.AABB) {
	w.records[id-1].Box = box
}

// inPlace is a Handler for records handed over as runs: the boxes are pushed where they lie.
type inPlace struct{ contacts int }

func (h *inPlace) Touch(_, _ uid.UID64, pen geom.Vec) (geom.Vec, float32, bool) {
	return pen, 0.5, true
}
func (h *inPlace) Contact(uid.UID64, uid.UID64, geom.Vec) { h.contacts++ }
func (h *inPlace) Moved(uid.UID64, plane.AABB)            {}

// Benchmark_Column_RebuildQueryTick is one tick of a space over records in a column of 72-byte
// structs, as gram's are with the box in float32: every record moves, then Rebuild, eight 100x100
// queries and a collision tick confirming every pair — no tick in the spread scene without one,
// gram's world alone. items copies the records into Items and writes the pushed boxes back from
// Moved, as gram does before runs; runs hands the records over as runs of a chunk each, and the
// tick pushes them in place.
func Benchmark_Column_RebuildQueryTick(b *testing.B) {
	for _, scene := range []struct {
		name  string
		build func(*testing.B) *columnScene
		tick  bool
	}{
		{"spread,n=5000,no-tick", spreadScene, false},
		{"spread,n=5000", spreadScene, true},
		{"crowd,n=5000", crowdScene, true},
	} {
		b.Run(scene.name+"/items", func(b *testing.B) {
			sc := scene.build(b)
			h := &writeBack{records: sc.records}
			engine := sc.space.CollideEngine(h, columnCollide)
			items := make([]aabbworld.Item, 0, len(sc.records))
			b.ReportAllocs()
			for b.Loop() {
				sc.move()
				items = items[:0]
				for i := range sc.records {
					r := &sc.records[i]
					items = append(items, aabbworld.Item{ID: sc.ids[i], Box: r.Box, Caps: r.Caps})
				}
				sc.space.Rebuild(items)
				sc.query()
				if scene.tick {
					engine.Tick()
				}
			}
			b.ReportMetric(float64(h.contacts)/float64(b.N), "contacts/tick")
		})
		b.Run(scene.name+"/runs", func(b *testing.B) {
			sc := scene.build(b)
			h := &inPlace{}
			engine := sc.space.CollideEngine(h, columnCollide)
			var runs []aabbworld.Run
			b.ReportAllocs()
			for b.Loop() {
				sc.move()
				runs = runs[:0]
				for i := 0; i < len(sc.records); i += chunkRecords {
					j := min(i+chunkRecords, len(sc.records))
					run := aabbworld.RunOf(sc.ids[i:j], sc.records[i:j], recordBox, recordCaps)
					runs = append(runs, run)
				}
				sc.space.RebuildRuns(runs, nil)
				sc.query()
				if scene.tick {
					engine.Tick()
				}
			}
			b.ReportMetric(float64(h.contacts)/float64(b.N), "contacts/tick")
		})
	}
}

// Benchmark_Column_Move moves every record of the spread scene once: each by Space.Move, or a run
// of a chunk at a time by Space.MoveRun, its deltas worked out first, as gram's motion does.
func Benchmark_Column_Move(b *testing.B) {
	b.Run("each", func(b *testing.B) {
		sc := spreadScene(b)
		b.ReportAllocs()
		for b.Loop() {
			sc.move()
		}
	})
	b.Run("runs", func(b *testing.B) {
		sc := spreadScene(b)
		deltas := make([]geom.Vec, chunkRecords)
		var left []uid.UID64
		b.ReportAllocs()
		for b.Loop() {
			for i := 0; i < len(sc.records); i += chunkRecords {
				rows := sc.records[i:min(i+chunkRecords, len(sc.records))]
				by := deltas[:len(rows)]
				for k := range rows {
					r := &rows[k]
					by[k] = geom.NewVec(float32(r.Vel[0]*r.Vel[2]), float32(r.Vel[1]*r.Vel[2]))
				}
				run := aabbworld.RunOf(sc.ids[i:i+len(rows)], rows, recordBox, recordCaps)
				left = sc.space.MoveRun(run, by, left[:0])
			}
		}
	})
}
