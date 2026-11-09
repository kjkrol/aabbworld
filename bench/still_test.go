package bench_test

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/kjkrol/aabbworld"
	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

// stillLayers is where the standing boxes are handed: nowhere, the units alone; among the moving
// ones, as before v1.20.0; or as a still layer of their own.
var stillLayers = []string{"none", "moving", "still"}

// standingScene is gram's Madeira: 31,000 trees 20x20 and houses 40x40, one in eight a house
// that collides, standing over a world of 20,480x10,240 with 64-unit buckets, and 50 units 10x10
// walking among them; every record in runs of a chunk each.
type standingScene struct {
	space    *aabbworld.Space
	standing []record
	units    []record
	ids      []uid.UID64 // the standing records' ids, then the units'
	runs     []aabbworld.Run
	unitRuns []aabbworld.Run
	steps    []geom.Vec
	view     geom.AABB
	tick     int
}

func newStandingScene(b *testing.B, layer string) *standingScene {
	b.Helper()
	space, err := aabbworld.NewSpace(aabbworld.Config{Width: 20480, Height: 10240, BucketSize: 64})
	if err != nil {
		b.Fatal(err)
	}
	sc := &standingScene{space: space, view: geom.NewAABBAt(geom.NewVec(4000, 3500), 2560, 1440)}
	rng := rand.New(rand.NewPCG(31, 50))
	for i := range 31000 {
		at := geom.NewVec(float32(1000+rng.Float64()*18000), float32(1000+rng.Float64()*8000))
		r := record{Box: plane.NewAABB(at, 20, 20), Kind: 1}
		if i%8 == 0 {
			r.Box, r.Caps = plane.NewAABB(at, 40, 40), aabbworld.CanCollide|aabbworld.Static
		}
		sc.standing = append(sc.standing, r)
	}
	for range 50 {
		at := geom.NewVec(float32(4000+rng.Float64()*2560), float32(3500+rng.Float64()*1440))
		heading := rng.Float64() * 2 * math.Pi
		sc.units = append(sc.units, record{Box: plane.NewAABB(at, 10, 10),
			Vel: [4]float64{math.Cos(heading), math.Sin(heading), 1, 0}, Kind: 2,
			Caps: aabbworld.CanCollide})
	}
	for i := range len(sc.standing) + len(sc.units) {
		sc.ids = append(sc.ids, uid.UID64(i+1))
	}
	for at := 0; at < len(sc.standing); at += chunkRecords {
		to := min(at+chunkRecords, len(sc.standing))
		sc.runs = append(sc.runs, aabbworld.RunOf(sc.ids[at:to], sc.standing[at:to], recordBox,
			recordCaps))
	}
	unitIDs := sc.ids[len(sc.standing):]
	sc.unitRuns = []aabbworld.Run{aabbworld.RunOf(unitIDs, sc.units, recordBox, recordCaps)}
	switch layer {
	case "still":
		sc.space.RebuildStill(sc.runs, nil)
	case "moving":
		sc.unitRuns = append(sc.runs, sc.unitRuns...)
	}
	sc.space.RebuildRuns(sc.unitRuns, nil)
	sc.steps = make([]geom.Vec, len(sc.units))
	return sc
}

// walk moves every unit a unit along its heading, turning it round every hundred ticks.
func (sc *standingScene) walk() {
	sc.tick++
	turn := float64(1 - 2*(sc.tick/100%2))
	for i := range sc.units {
		v := &sc.units[i].Vel
		sc.steps[i] = geom.NewVec(float32(v[0]*turn), float32(v[1]*turn))
	}
	run := sc.unitRuns[len(sc.unitRuns)-1]
	sc.space.MoveRun(run, sc.steps, nil)
}

func (sc *standingScene) query() int {
	return sc.space.Query(sc.view, aabbworld.AnyCapability, func(uid.UID64) {})
}

// Benchmark_Still_RebuildQuery is what gram pays after every move: RebuildRuns over every run of
// the world and the first Query of a view-sized box, which indexes them.
func Benchmark_Still_RebuildQuery(b *testing.B) {
	for _, layer := range stillLayers {
		b.Run("layer="+layer, func(b *testing.B) {
			sc := newStandingScene(b, layer)
			b.ReportAllocs()
			for b.Loop() {
				sc.space.RebuildRuns(sc.unitRuns, nil)
				sc.query()
			}
		})
	}
}

// Benchmark_Still_Query is a Query of a view-sized box, 2,560x1,440, over cells already built.
func Benchmark_Still_Query(b *testing.B) {
	for _, layer := range stillLayers {
		b.Run("layer="+layer, func(b *testing.B) {
			sc := newStandingScene(b, layer)
			found := sc.query()
			b.ReportAllocs()
			for b.Loop() {
				sc.query()
			}
			b.ReportMetric(float64(found), "found")
		})
	}
}

// Benchmark_Still_Tick is a collision tick after the units walk a step: pairing them with each
// other and with the houses, and separating what overlaps.
func Benchmark_Still_Tick(b *testing.B) {
	for _, layer := range stillLayers {
		b.Run("layer="+layer, func(b *testing.B) {
			sc := newStandingScene(b, layer)
			h := &inPlace{}
			engine := sc.space.CollideEngine(h, columnCollide)
			engine.Tick()
			h.contacts = 0
			b.ReportAllocs()
			for b.Loop() {
				sc.walk()
				engine.Tick()
			}
			b.ReportMetric(float64(h.contacts)/float64(b.N), "contacts/op")
		})
	}
}
