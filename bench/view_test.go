package bench_test

import (
	"fmt"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/kjkrol/aabbworld"
	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

// realSpace scatters n entities through a bucket-indexed Space and returns it with an observer.
func realSpace(b *testing.B, n int) (*aabbworld.Space, uid.UID64) {
	b.Helper()
	space, err := aabbworld.NewSpace(aabbworld.Config{
		Width: 4000, Height: 4000,
		BucketSize: 256,
	})
	if err != nil {
		b.Fatal(err)
	}
	observer := uid.UID64(1)
	items := []aabbworld.Item{{ID: observer, Box: plane.NewAABB(geom.NewVec(2000, 2000), 10, 10)}}
	r := rand.New(rand.NewPCG(1, 2))
	for i := range n {
		items = append(items, aabbworld.Item{ID: uid.UID64(100 + i), Box: plane.NewAABB(geom.NewVec(float32(r.IntN(4000)), float32(r.IntN(4000))), 20, 20)})
	}
	space.Rebuild(items)
	return space, observer
}

func entities(n int) string { return fmt.Sprintf("entities=%d", n) }

var viewCone = aabbworld.Cone{Direction: geom.NewVec(1.0, 0.0), HalfAngle: math.Pi / 4, Radius: 800}

// Benchmark_View_Visible scans a cone and lists what it sees, nearest first.
func Benchmark_View_Visible(b *testing.B) {
	for _, n := range []int{100, 1000} {
		b.Run(entities(n), func(b *testing.B) {
			space, observer := realSpace(b, n)
			v := &aabbworld.View{}
			b.ReportAllocs()
			for b.Loop() {
				if space.Scan(observer, viewCone, v) {
					v.Entities(func(uid.UID64, float32) {})
				}
			}
		})
	}
}

// Benchmark_View_Outline scans a cone and traces the lit region as a fan of points.
func Benchmark_View_Outline(b *testing.B) {
	for _, n := range []int{100, 1000} {
		b.Run(entities(n), func(b *testing.B) {
			space, observer := realSpace(b, n)
			v := &aabbworld.View{}
			var fog []geom.Vec
			b.ReportAllocs()
			for b.Loop() {
				if space.Scan(observer, viewCone, v) {
					fog = v.Outline(0, fog[:0])
				}
			}
		})
	}
}

// Benchmark_View_Both scans once and reads both the entities and the outline.
func Benchmark_View_Both(b *testing.B) {
	for _, n := range []int{100, 1000} {
		b.Run(entities(n), func(b *testing.B) {
			space, observer := realSpace(b, n)
			v := &aabbworld.View{}
			var fog []geom.Vec
			b.ReportAllocs()
			for b.Loop() {
				if space.Scan(observer, viewCone, v) {
					v.Entities(func(uid.UID64, float32) {})
					fog = v.Outline(0, fog[:0])
				}
			}
		})
	}
}

// Benchmark_View_Depths scans a cone and samples its reach at 63 angles.
func Benchmark_View_Depths(b *testing.B) {
	for _, n := range []int{100, 1000} {
		b.Run(entities(n), func(b *testing.B) {
			space, observer := realSpace(b, n)
			v := &aabbworld.View{}
			var depths []float32
			b.ReportAllocs()
			for b.Loop() {
				if space.Scan(observer, viewCone, v) {
					depths = v.Depths(63, depths[:0])
				}
			}
		})
	}
}

// Benchmark_View_Translucent is Benchmark_View_Both with three entities in ten see-through at τ = 0.5.
func Benchmark_View_Translucent(b *testing.B) {
	translucent := func(id uid.UID64) float32 {
		if id%10 < 3 {
			return 0.5
		}
		return 0
	}
	cone := viewCone
	cone.Transparency = translucent
	for _, n := range []int{100, 1000} {
		b.Run(entities(n), func(b *testing.B) {
			space, observer := realSpace(b, n)
			v := &aabbworld.View{}
			var fog []geom.Vec
			b.ReportAllocs()
			for b.Loop() {
				if space.Scan(observer, cone, v) {
					v.Entities(func(uid.UID64, float32) {})
					fog = v.Outline(0, fog[:0])
				}
			}
		})
	}
}

// rolling is the ground of the elevated scene: a sine over the plane, 0 to 10.
func rolling(p geom.Vec) float32 {
	return float32(5 + 5*math.Sin(float64(p.X)/90)*math.Cos(float64(p.Y)/70))
}

// raster samples rolling once per 32-unit cell and answers by lookup, as an engine's heightfield would.
func raster() func(geom.Vec) float32 {
	const cell, side = 32.0, 4000/32 + 1
	grid := make([]float32, side*side)
	for y := range side {
		for x := range side {
			grid[y*side+x] = rolling(geom.NewVec(float32(x)*cell, float32(y)*cell))
		}
	}
	return func(p geom.Vec) float32 {
		x, y := int(p.X/cell), int(p.Y/cell)
		if x < 0 || y < 0 || x >= side || y >= side {
			return 0
		}
		return grid[y*side+x]
	}
}

// Benchmark_View_Elevated is Benchmark_View_Translucent's scene and cone climbed rung by rung into
// heights: none (the reference), entity bands alone, then ground sampled every 32 as a constant, a
// raster lookup, the raster at the default step, and the sine computed on the spot. An eye 6 up;
// every fifth entity flies at 30–34, every other stands to 2, the rest to 12.
func Benchmark_View_Elevated(b *testing.B) {
	base := viewCone
	base.Transparency = func(id uid.UID64) float32 {
		if id%10 < 3 {
			return 0.5
		}
		return 0
	}
	bands := func(id uid.UID64) (float32, float32) {
		switch {
		case id%5 == 0:
			return 30, 34
		case id%2 == 0:
			return 0, 2
		}
		return 0, 12
	}
	withHeights := func(ground func(geom.Vec) float32, step float32) aabbworld.Cone {
		c := base
		c.Eye, c.Elevation, c.Ground, c.GroundStep = 6, bands, ground, step
		return c
	}
	rungs := []struct {
		name string
		cone aabbworld.Cone
	}{
		{"none", base},
		{"entities", withHeights(nil, 0)},
		{"flat", withHeights(func(geom.Vec) float32 { return 5 }, 32)},
		{"raster", withHeights(raster(), 32)},
		{"raster-step50", withHeights(raster(), 0)},
		{"sine", withHeights(rolling, 32)},
	}
	for _, n := range []int{100, 1000} {
		for _, r := range rungs {
			b.Run(entities(n)+"/heights="+r.name, func(b *testing.B) {
				space, observer := realSpace(b, n)
				v := &aabbworld.View{}
				var fog []geom.Vec
				b.ReportAllocs()
				for b.Loop() {
					if space.Scan(observer, r.cone, v) {
						v.Entities(func(uid.UID64, float32) {})
						fog = v.Outline(0, fog[:0])
					}
				}
			})
		}
	}
}

// Benchmark_View_Shadows reads a scan two ways at 63 angles: the reach (Depths) and the stretches of
// hidden ground (Shadows), on a plane and over the raster rung of Benchmark_View_Elevated.
func Benchmark_View_Shadows(b *testing.B) {
	flat := viewCone
	flat.Transparency = func(id uid.UID64) float32 {
		if id%10 < 3 {
			return 0.5
		}
		return 0
	}
	high := flat
	high.Eye, high.Ground, high.GroundStep = 6, raster(), 32
	high.Elevation = func(id uid.UID64) (float32, float32) {
		switch {
		case id%5 == 0:
			return 30, 34
		case id%2 == 0:
			return 0, 2
		}
		return 0, 12
	}
	for _, n := range []int{100, 1000} {
		for _, c := range []struct {
			name string
			cone aabbworld.Cone
		}{{"flat", flat}, {"raster", high}} {
			b.Run(entities(n)+"/"+c.name+"/depths", func(b *testing.B) {
				space, observer := realSpace(b, n)
				v := &aabbworld.View{}
				var depths []float32
				b.ReportAllocs()
				for b.Loop() {
					if space.Scan(observer, c.cone, v) {
						depths = v.Depths(63, depths[:0])
					}
				}
			})
			b.Run(entities(n)+"/"+c.name+"/shadows", func(b *testing.B) {
				space, observer := realSpace(b, n)
				v := &aabbworld.View{}
				var shadows []aabbworld.Shadow
				b.ReportAllocs()
				for b.Loop() {
					if space.Scan(observer, c.cone, v) {
						shadows = v.Shadows(63, shadows[:0])
					}
				}
			})
		}
	}
}
