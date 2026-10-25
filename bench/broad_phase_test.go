package bench_test

import (
	"math/rand/v2"
	"testing"

	"github.com/kjkrol/aabbworld"
	"github.com/kjkrol/aabbworld/collide"
	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

// pairsReach is how far a box may travel in a tick, as a fraction of its shorter side.
const pairsReach = 0.5

// crowd is a population kept beside the Space rebuilt from it.
type crowd struct {
	space *aabbworld.Space
	items []aabbworld.Item
}

func (c *crowd) add(x, y, w, h float32, caps aabbworld.Capability) {
	box := c.space.WrapAABB(plane.NewAABB(geom.NewVec(x, y), w, h).AABB)
	c.items = append(c.items, aabbworld.Item{ID: uid.UID64(len(c.items)), Box: box, Caps: caps})
}

func (c *crowd) rebuild() { c.space.Rebuild(c.items) }

// hooks is a Handler made of optional funcs; an unset Touch confirms every pair.
type hooks struct {
	touch func(a, b uid.UID64, pen geom.Vec) (geom.Vec, bool)
}

func (h *hooks) Touch(a, b uid.UID64, pen geom.Vec) (geom.Vec, float32, bool) {
	if h.touch == nil {
		return pen, 0.5, true
	}
	pen, ok := h.touch(a, b, pen)
	return pen, 0.5, ok
}
func (h *hooks) Contact(uid.UID64, uid.UID64, geom.Vec) {}
func (h *hooks) Moved(uid.UID64, plane.AABB)            {}

// broadPhaseScene is a population and the bucket size a caller sizing its
// buckets from its largest entity would have picked for it.
type broadPhaseScene struct {
	name   string
	bucket uint32
	margin float32 // what a probe per entity reaches: the smallest side
	groups []sceneGroup
}

type sceneGroup struct {
	count int
	side  float32
}

var broadPhaseScenes = []broadPhaseScene{
	{name: "side=10,fill=20%", bucket: 32, margin: 10, groups: []sceneGroup{{8388, 10}}},
	{name: "side=10,fill=40%", bucket: 32, margin: 10, groups: []sceneGroup{{16777, 10}}},
	{name: "side=5,fill=20%", bucket: 16, margin: 5, groups: []sceneGroup{{33554, 5}}},
	{name: "crowd=8,few=100", bucket: 256, margin: 8, groups: []sceneGroup{{12000, 8}, {20, 100}}},
}

func (sc broadPhaseScene) build(b *testing.B) *crowd {
	b.Helper()
	space, err := aabbworld.NewSpace(aabbworld.Config{
		Width: 2048, Height: 2048, Edges: aabbworld.Torus,
		BucketSize: sc.bucket,
	})
	if err != nil {
		b.Fatalf("NewSpace: %v", err)
	}
	c := &crowd{space: space}
	rng := rand.New(rand.NewPCG(41, 43))
	for _, g := range sc.groups {
		for range g.count {
			c.add(float32(rng.Float64()*2048), float32(rng.Float64()*2048), g.side, g.side, aabbworld.CanCollide)
		}
	}
	c.rebuild()
	return c
}

// grown is the whole of b, seam pieces and all, reaching margin further on every side.
func grown(b plane.AABB, margin float32) geom.AABB {
	return geom.NewAABB(
		geom.NewVec(b.TopLeft.X-margin, b.TopLeft.Y-margin),
		geom.NewVec(b.TopLeft.X+b.Size.X+margin, b.TopLeft.Y+b.Size.Y+margin),
	)
}

// Benchmark_BroadPhase compares two ways of finding who is near whom in a crowd: one Query per
// entity over its grown box (probe-per-entity), and one Engine.Tick that pairs every box grown by
// pairsReach and asks Touch about each overlapping pair (tick; Touch refuses, so nothing moves).
func Benchmark_BroadPhase(b *testing.B) {
	for _, sc := range broadPhaseScenes {
		b.Run(sc.name+"/probe-per-entity", func(b *testing.B) {
			c := sc.build(b)
			var self uid.UID64
			found := 0
			onFound := func(other uid.UID64) {
				if other.Index() > self.Index() {
					found++
				}
			}
			b.ReportAllocs()
			for b.Loop() {
				found = 0
				for _, it := range c.items {
					self = it.ID
					c.space.Query(grown(it.Box, sc.margin), aabbworld.CanCollide, onFound)
				}
			}
			b.ReportMetric(float64(found), "pairs")
		})
		b.Run(sc.name+"/tick", func(b *testing.B) {
			c := sc.build(b)
			found := 0
			h := &hooks{touch: func(_, _ uid.UID64, pen geom.Vec) (geom.Vec, bool) {
				found++
				return pen, false
			}}
			e := c.space.CollideEngine(h, collide.Config{Reach: pairsReach, Iterations: 1})
			b.ReportAllocs()
			for b.Loop() {
				found = 0
				e.Tick()
			}
			b.ReportMetric(float64(found), "overlaps")
		})
	}
}
