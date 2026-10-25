package bench_test

import (
	"testing"

	"github.com/kjkrol/aabbworld/geom"
	iplane "github.com/kjkrol/aabbworld/internal/plane"
	"github.com/kjkrol/aabbworld/plane"
)

var (
	aabbSink plane.AABB
	vecSink  geom.Vec
)

// surfaces are the two edge rules the primitives are measured under.
var surfaces = []struct {
	name string
	make func(w, h float32) *iplane.Surface
}{
	{"euclidean", iplane.NewEuclidean2D},
	{"toroidal", iplane.NewToroidal2D},
}

// Benchmark_Surface_WrapAABB folds a rectangle crossing the far corner into the plane.
func Benchmark_Surface_WrapAABB(b *testing.B) {
	for _, s := range surfaces {
		b.Run(s.name, func(b *testing.B) {
			surface := s.make(10, 10)
			box := geom.NewAABBAt(geom.NewVec(9, 9), 2, 2)
			b.ReportAllocs()
			for b.Loop() {
				aabbSink = surface.WrapAABB(box)
			}
		})
	}
}

// Benchmark_Surface_Expand grows a box near the far corner by a margin on every side.
func Benchmark_Surface_Expand(b *testing.B) {
	for _, s := range surfaces {
		b.Run(s.name, func(b *testing.B) {
			surface := s.make(100, 100)
			template := plane.NewAABB(geom.NewVec(90, 90), 8, 8)
			b.ReportAllocs()
			for b.Loop() {
				box := template
				surface.Expand(&box, 5)
				aabbSink = box
			}
		})
	}
}

// Benchmark_Surface_Translate moves a box across the far edge on one axis and back on the other,
// and one that stays well inside, by a few units.
func Benchmark_Surface_Translate(b *testing.B) {
	cases := []struct {
		name     string
		template plane.AABB
		delta    geom.Vec
	}{
		{"across", plane.NewAABB(geom.NewVec(80, 80), 15, 10), geom.NewVec(12, -18)},
		{"inside", plane.NewAABB(geom.NewVec(40, 40), 15, 10), geom.NewVec(3, -2)},
	}
	for _, s := range surfaces {
		for _, c := range cases {
			b.Run(s.name+"/"+c.name, func(b *testing.B) {
				surface := s.make(100, 100)
				template, delta := c.template, c.delta
				b.ReportAllocs()
				for b.Loop() {
					box := template
					surface.Translate(&box, delta)
					aabbSink = box
				}
			})
		}
	}
}

// Benchmark_Vec_Clamp bounds a point to the plane.
func Benchmark_Vec_Clamp(b *testing.B) {
	v := geom.NewVec(12.5, -3.25)
	size := geom.NewVec(10, 10)
	b.ReportAllocs()
	for b.Loop() {
		vecSink = iplane.Clamp(v, size)
	}
}

// Benchmark_Vec_Wrap folds a point back into the plane from four distances.
func Benchmark_Vec_Wrap(b *testing.B) {
	size := geom.NewVec(1024, 1024)
	for _, c := range []struct {
		name string
		v    geom.Vec
	}{
		{"already inside", geom.NewVec(512, 512)},
		{"one step over", geom.NewVec(1030, 1030)},
		{"just below zero", geom.NewVec(-6, -6)},
		{"far out", geom.NewVec(99999, -99999)},
	} {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				vecSink = iplane.Wrap(c.v, size)
			}
		})
	}
}
