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

// lattice is rolling at the corners of 32-unit cells, a field of heights.
func lattice() *aabbworld.HeightField {
	const cell, side = 32, 4000/32 + 1
	f := &aabbworld.HeightField{Corners: make([]float32, side*side), Cols: side, Rows: side,
		Cell: cell}
	for y := range side {
		for x := range side {
			f.Corners[y*side+x] = rolling(geom.NewVec(float32(x)*cell, float32(y)*cell))
		}
	}
	return f
}

// latticeProfile hands a field's ground along a ray over as its owner would through Cone.Profile:
// where the ray crosses each cell's fold, where it leaves the cell (and steps to the next one's
// ground where it differs), and the end.
type latticeProfile struct{ f *aabbworld.HeightField }

func (p latticeProfile) Along(origin, dir geom.Vec, length float32,
	visit func(d, alt float32) bool) {
	size, end := float64(p.f.Cell), float64(length)
	fx, fy := float64(origin.X)/size, float64(origin.Y)/size
	cx, cy := math.Floor(fx), math.Floor(fy)
	bx, by := float64(dir.X)/size, float64(dir.Y)/size
	sx, nx, dx := cellCrossings(fx, bx)
	sy, ny, dy := cellCrossings(fy, by)
	h := p.corners(cx, cy)
	for t := 0.0; ; {
		ax, ay := fx-cx, fy-cy
		exit := min(nx, ny)
		if fold, alt, ok := cellFold(h, ax, ay, bx, by); ok && fold > t && fold < min(exit, end) &&
			!visit(float32(fold), float32(alt)) {
			return
		}
		if exit >= end {
			visit(length, p.at(geom.NewVec(origin.X+dir.X*length, origin.Y+dir.Y*length)))
			return
		}
		u, v := min(max(ax+bx*exit, 0), 1), min(max(ay+by*exit, 0), 1)
		acrossX := nx <= ny
		var alt float64
		switch {
		case acrossX && sx > 0:
			alt = h[1] + v*(h[3]-h[1])
		case acrossX:
			alt = h[0] + v*(h[2]-h[0])
		case sy > 0:
			alt = h[2] + u*(h[3]-h[2])
		default:
			alt = h[0] + u*(h[1]-h[0])
		}
		if !visit(float32(exit), float32(alt)) {
			return
		}
		if acrossX {
			cx, nx = cx+sx, nx+dx
		} else {
			cy, ny = cy+sy, ny+dy
		}
		h = p.corners(cx, cy)
		var entry float64
		switch {
		case acrossX && sx > 0:
			entry = h[0] + v*(h[2]-h[0])
		case acrossX:
			entry = h[1] + v*(h[3]-h[1])
		case sy > 0:
			entry = h[0] + u*(h[1]-h[0])
		default:
			entry = h[2] + u*(h[3]-h[2])
		}
		if entry != alt && !visit(float32(exit), float32(entry)) {
			return
		}
		t = exit
	}
}

func cellCrossings(f, b float64) (step, next, delta float64) {
	switch {
	case b > 0:
		return 1, (math.Floor(f) + 1 - f) / b, 1 / b
	case b < 0:
		return -1, (math.Floor(f) - f) / b, -1 / b
	}
	return 0, math.Inf(1), math.Inf(1)
}

// along the diagonal whose corners differ less in height, along 1–2 where they differ alike
func cellFold(h [4]float64, ax, ay, bx, by float64) (t, alt float64, ok bool) {
	if math.Abs(h[0]-h[3]) < math.Abs(h[1]-h[2]) {
		if bx == by {
			return 0, 0, false
		}
		t = (ay - ax) / (bx - by)
		return t, h[0] + min(max(ax+bx*t, 0), 1)*(h[3]-h[0]), true
	}
	if bx+by == 0 {
		return 0, 0, false
	}
	t = (1 - ax - ay) / (bx + by)
	return t, h[1] + min(max(ay+by*t, 0), 1)*(h[2]-h[1]), true
}

// top-left, top-right, bottom-left, bottom-right; 0 off the field
func (p latticeProfile) corners(cx, cy float64) (h [4]float64) {
	x, y, cols := int(cx), int(cy), p.f.Cols
	if x < 0 || y < 0 || x >= cols-1 || y >= p.f.Rows-1 {
		return h
	}
	c := p.f.Corners
	return [4]float64{float64(c[y*cols+x]), float64(c[y*cols+x+1]), float64(c[(y+1)*cols+x]),
		float64(c[(y+1)*cols+x+1])}
}

func (p latticeProfile) at(q geom.Vec) float32 {
	size := float64(p.f.Cell)
	fx, fy := float64(q.X)/size, float64(q.Y)/size
	x0, y0 := math.Floor(fx), math.Floor(fy)
	h, u, v := p.corners(x0, y0), fx-x0, fy-y0
	switch {
	case math.Abs(h[0]-h[3]) < math.Abs(h[1]-h[2]) && u >= v:
		return float32(h[0] + u*(h[1]-h[0]) + v*(h[3]-h[1]))
	case math.Abs(h[0]-h[3]) < math.Abs(h[1]-h[2]):
		return float32(h[0] + v*(h[2]-h[0]) + u*(h[3]-h[2]))
	case u+v <= 1:
		return float32(h[0] + u*(h[1]-h[0]) + v*(h[2]-h[0]))
	}
	return float32(h[3] + (1-u)*(h[2]-h[3]) + (1-v)*(h[1]-h[3]))
}

// Benchmark_View_Elevated is Benchmark_View_Translucent's scene and cone climbed rung by rung into
// heights: none (the reference), entity bands alone, then ground sampled every 32 as a constant, a
// raster lookup, the raster at the default step, the sine computed on the spot, and the sine at
// the corners of 32-unit cells read exactly along each ray: through a profile's calls, and as the
// cone's own field. An eye 6 up; every fifth entity flies at 30–34, every other stands to 2, the
// rest to 12.
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
	field := lattice()
	profiled := withHeights(latticeProfile{field}.at, 0)
	profiled.Profile = latticeProfile{field}
	fielded := withHeights(nil, 0)
	fielded.Heights = field
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
		{"profile", profiled},
		{"field", fielded},
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

// Benchmark_View_Tick is sight in a tick of a game: the boxes rebuilt, so the first scan indexes
// the grid again, then eight observers round the middle scan a cone each, looking out.
func Benchmark_View_Tick(b *testing.B) {
	var eyes []geom.Vec
	var cones []aabbworld.Cone
	for k := range 8 {
		a := float64(k) * math.Pi / 4
		x, y := 2000+600*math.Cos(a), 2000+600*math.Sin(a)
		eyes = append(eyes, geom.NewVec(float32(x), float32(y)))
		cone := viewCone
		cone.Direction = geom.NewVec(float32(math.Cos(a)), float32(math.Sin(a)))
		cones = append(cones, cone)
	}
	for _, n := range []int{1000, 10000} {
		b.Run(entities(n), func(b *testing.B) {
			space, items := scatter(b, n, 0, eyes)
			v := &aabbworld.View{}
			b.ReportAllocs()
			for b.Loop() {
				space.Rebuild(items)
				for k, cone := range cones {
					if space.Scan(uid.UID64(k+1), cone, v) {
						v.Entities(func(uid.UID64, float32) {})
					}
				}
			}
		})
	}
}

// Benchmark_View_Seam is Benchmark_View_Visible on a torus with the observer by its right edge,
// so that the cone looks across the seam.
func Benchmark_View_Seam(b *testing.B) {
	for _, n := range []int{100, 1000} {
		b.Run(entities(n), func(b *testing.B) {
			space, _ := scatter(b, n, aabbworld.Torus, []geom.Vec{geom.NewVec(3700, 1995)})
			v := &aabbworld.View{}
			b.ReportAllocs()
			for b.Loop() {
				if space.Scan(1, viewCone, v) {
					v.Entities(func(uid.UID64, float32) {})
				}
			}
		})
	}
}

// scatter is the scene of realSpace under edges, with an observer 10 on a side, id 1 on, at each
// of eyes, every box placed by the edge rules.
func scatter(
	b *testing.B, n int, edges aabbworld.Edges, eyes []geom.Vec,
) (*aabbworld.Space, []aabbworld.Item) {
	b.Helper()
	space, err := aabbworld.NewSpace(aabbworld.Config{
		Width: 4000, Height: 4000, Edges: edges,
		BucketSize: 256,
	})
	if err != nil {
		b.Fatal(err)
	}
	var items []aabbworld.Item
	add := func(id uid.UID64, at geom.Vec, side float32) {
		box := plane.NewAABB(at, side, side)
		space.Place(&box)
		items = append(items, aabbworld.Item{ID: id, Box: box})
	}
	for k, at := range eyes {
		add(uid.UID64(k+1), at, 10)
	}
	r := rand.New(rand.NewPCG(1, 2))
	for i := range n {
		add(uid.UID64(100+i), geom.NewVec(float32(r.IntN(4000)), float32(r.IntN(4000))), 20)
	}
	space.Rebuild(items)
	return space, items
}
