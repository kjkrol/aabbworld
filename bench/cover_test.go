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

// forestCell is the side of a cell of the forest scenes, forestSide how many cells the forest spans.
const (
	forestCell = 20.0
	forestSide = 40
)

// forestCover is a forest on a grid as a CoverField: a flat slice of cells walked cell by cell.
type forestCover struct {
	origin geom.Vec // the forest's top-left corner
	tau    []float32
}

func (f *forestCover) Walk(origin, dir geom.Vec, length float32, visit func(near, far, bottom, top, tau float32) bool) {
	ox, oy := float64(origin.X-f.origin.X)/forestCell, float64(origin.Y-f.origin.Y)/forestCell
	cx, cy := int(math.Floor(ox)), int(math.Floor(oy))
	axis := func(o, d float64, c int) (int, float64, float64) {
		switch {
		case d > 0:
			return 1, (float64(c+1) - o) * forestCell / d, forestCell / d
		case d < 0:
			return -1, (float64(c) - o) * forestCell / d, -forestCell / d
		}
		return 0, math.Inf(1), math.Inf(1)
	}
	sx, nx, dx := axis(ox, float64(dir.X), cx)
	sy, ny, dy := axis(oy, float64(dir.Y), cy)
	inf := float32(math.Inf(1))
	for t := 0.0; t < float64(length); {
		exit := math.Min(math.Min(nx, ny), float64(length))
		if cx >= 0 && cy >= 0 && cx < forestSide && cy < forestSide {
			if tau := f.tau[cy*forestSide+cx]; tau > 0 && !visit(float32(t), float32(exit), -inf, inf, tau) {
				return
			}
		}
		if nx < ny {
			cx, t, nx = cx+sx, nx, nx+dx
		} else {
			cy, t, ny = cy+sy, ny, ny+dy
		}
	}
}

// forestScene is a forest of τ 0.5 just ahead of an observer at (2000, 2000): every cell of a
// forestSide square, or three in ten scattered over it. It comes as cover, as one entity per cell,
// and, when whole, as one entity for the lot.
func forestScene(b *testing.B, share float64) (cover *forestCover, perCell, merged *aabbworld.Space, observer uid.UID64) {
	b.Helper()
	newSpace := func() *aabbworld.Space {
		s, err := aabbworld.NewSpace(aabbworld.Config{Width: 4000, Height: 4000, BucketSize: 256})
		if err != nil {
			b.Fatal(err)
		}
		return s
	}
	observer = uid.UID64(1)
	eye := aabbworld.Item{ID: observer, Box: plane.NewAABB(geom.NewVec(2000, 1995), 10, 10)}
	cover = &forestCover{origin: geom.NewVec(2020, 2000-forestSide/2*forestCell), tau: make([]float32, forestSide*forestSide)}
	cells := []aabbworld.Item{eye}
	r := rand.New(rand.NewPCG(5, 7))
	for i := range cover.tau {
		if r.Float64() >= share {
			continue
		}
		cover.tau[i] = 0.5
		x, y := cover.origin.X+float32(i%forestSide)*forestCell, cover.origin.Y+float32(i/forestSide)*forestCell
		cells = append(cells, aabbworld.Item{ID: uid.UID64(100 + i), Box: plane.NewAABB(geom.NewVec(x, y), forestCell, forestCell)})
	}
	perCell = newSpace()
	perCell.Rebuild(cells)
	if share >= 1 {
		merged = newSpace()
		merged.Rebuild([]aabbworld.Item{eye, {ID: 100, Box: plane.NewAABB(cover.origin, forestSide*forestCell, forestSide*forestCell)}})
	}
	return cover, perCell, merged, observer
}

// Benchmark_View_Cover scans a 90° cone of radius 220 into a forest — whole, or three cells in
// ten — through the forest as cover, as one entity per cell and, whole, as one merged entity.
func Benchmark_View_Cover(b *testing.B) {
	cone := aabbworld.Cone{Direction: geom.NewVec(1, 0), HalfAngle: math.Pi / 4, Radius: 220}
	see := func(id uid.UID64) float32 { return 0.5 }
	for _, layout := range []struct {
		name  string
		share float64
	}{{"forest=whole", 1}, {"forest=30%", 0.3}} {
		cover, perCell, merged, observer := forestScene(b, layout.share)
		run := func(b *testing.B, space *aabbworld.Space, c aabbworld.Cone) {
			v := &aabbworld.View{}
			var fan []geom.Vec
			b.ReportAllocs()
			for b.Loop() {
				if space.Scan(observer, c, v) {
					v.Entities(func(uid.UID64, float32) {})
					fan = v.Outline(0, fan[:0])
				}
			}
		}
		b.Run(layout.name+"/terrain=cover", func(b *testing.B) {
			c := cone
			c.Cover = cover
			run(b, perCell0(b, observer), c)
		})
		b.Run(layout.name+"/terrain=cell-entities", func(b *testing.B) {
			c := cone
			c.Transparency = see
			run(b, perCell, c)
		})
		if merged != nil {
			b.Run(layout.name+"/terrain=merged-entity", func(b *testing.B) {
				c := cone
				c.Transparency = see
				run(b, merged, c)
			})
		}
	}
}

// perCell0 is a Space holding the observer alone, for a scan whose terrain is all cover.
func perCell0(b *testing.B, observer uid.UID64) *aabbworld.Space {
	b.Helper()
	s, err := aabbworld.NewSpace(aabbworld.Config{Width: 4000, Height: 4000, BucketSize: 256})
	if err != nil {
		b.Fatal(err)
	}
	s.Rebuild([]aabbworld.Item{{ID: observer, Box: plane.NewAABB(geom.NewVec(2000, 1995), 10, 10)}})
	return s
}
