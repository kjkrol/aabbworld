package bench_test

import (
	"math"
	"testing"

	"github.com/kjkrol/aabbworld"
	"github.com/kjkrol/aabbworld/collide"
	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

// The ground scenes: a 100x100 grid of 20-unit cells, a wall along every tenth row with a gap
// every tenth cell, and units resting 2 units into the walls from above, pushed out every tick.
const (
	groundCell  = 20.0
	groundSide  = 100
	groundUnits = 400
)

func groundWall(x, y int) bool { return y%10 == 5 && x%10 != 0 }

// wallGround is the walls as a SolidField over a flat slice of cells.
type wallGround struct{ solid []bool }

func newWallGround() *wallGround {
	g := &wallGround{solid: make([]bool, groundSide*groundSide)}
	for y := range groundSide {
		for x := range groundSide {
			g.solid[y*groundSide+x] = groundWall(x, y)
		}
	}
	return g
}

func (g *wallGround) at(x, y int) bool {
	return x >= 0 && y >= 0 && x < groundSide && y < groundSide && g.solid[y*groundSide+x]
}

func (g *wallGround) Solid(_ uid.UID64, box geom.AABB, visit func(collide.FieldBox) bool) {
	x0, y0 := int(math.Floor(box.TopLeft.X/groundCell)), int(math.Floor(box.TopLeft.Y/groundCell))
	x1, y1 := int(math.Ceil(box.BottomRight.X/groundCell)), int(math.Ceil(box.BottomRight.Y/groundCell))
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if !g.at(x, y) {
				continue
			}
			var open collide.Sides
			if !g.at(x-1, y) {
				open |= collide.Left
			}
			if !g.at(x+1, y) {
				open |= collide.Right
			}
			if !g.at(x, y-1) {
				open |= collide.Top
			}
			if !g.at(x, y+1) {
				open |= collide.Bottom
			}
			b := geom.NewAABBAt(geom.NewVec(float64(x)*groundCell, float64(y)*groundCell), groundCell, groundCell)
			if !visit(collide.FieldBox{Box: b, Cell: uint64(y*groundSide + x), Open: open}) {
				return
			}
		}
	}
}

// segmentGround is the same walls as a WallField: each cell's wall a FieldWall along the row,
// half a cell thick, joined to the cells beside it.
type segmentGround struct{ *wallGround }

func (g segmentGround) Solid(uid.UID64, geom.AABB, func(collide.FieldBox) bool) {}

func (g segmentGround) Walls(_ uid.UID64, box geom.AABB, visit func(collide.FieldWall) bool) {
	x0 := int(math.Floor(box.TopLeft.X / groundCell))
	y0 := int(math.Floor(box.TopLeft.Y / groundCell))
	x1 := int(math.Ceil(box.BottomRight.X / groundCell))
	y1 := int(math.Ceil(box.BottomRight.Y / groundCell))
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if !g.at(x, y) {
				continue
			}
			var open collide.Ends
			if !g.at(x-1, y) {
				open |= collide.EndA
			}
			if !g.at(x+1, y) {
				open |= collide.EndB
			}
			mid := (float64(y) + 0.5) * groundCell
			w := collide.FieldWall{A: geom.NewVec(float64(x)*groundCell, mid),
				B: geom.NewVec(float64(x+1)*groundCell, mid), Half: groundCell / 2,
				Cell: uint64(y*groundSide + x), Open: open}
			if !visit(w) {
				return
			}
		}
	}
}

// quiet is a Handler that confirms every pair and listens to nothing.
type quiet struct{}

func (quiet) Touch(_, _ uid.UID64, pen geom.Vec) (geom.Vec, bool) { return pen, true }
func (quiet) Contact(_, _ uid.UID64, _ geom.Vec)                  {}
func (quiet) Moved(uid.UID64, plane.AABB)                         {}

// groundUnitsAt are the units' starting boxes, 2 into the walls' tops, clear of each other.
func groundUnitsAt() []aabbworld.Item {
	var items []aabbworld.Item
	for i := range groundUnits {
		row := 5 + 10*(i%10)
		x := float64(1+(i/10)%9)*groundCell + float64(i/90)*2 + 3
		items = append(items, aabbworld.Item{ID: uid.UID64(1 + i), Box: plane.NewAABB(geom.NewVec(x+float64(i/10/9)*200, float64(row)*groundCell-8), 10, 10), Caps: aabbworld.CanCollide})
	}
	return items
}

// Benchmark_Collide_Ground ticks a collision engine over units pushed out of walls: the walls as a
// field, as one static entity per run of wall, and as one static entity per cell.
func Benchmark_Collide_Ground(b *testing.B) {
	units := groundUnitsAt()
	var runs, cells []aabbworld.Item
	id := uid.UID64(100000)
	for y := range groundSide {
		for x := 0; x < groundSide; x++ {
			if groundWall(x, y) {
				cells = append(cells, aabbworld.Item{ID: id, Box: plane.NewAABB(geom.NewVec(float64(x)*groundCell, float64(y)*groundCell), groundCell, groundCell), Caps: aabbworld.CanCollide | aabbworld.Static})
				id++
			}
			if groundWall(x, y) && !groundWall(x-1, y) {
				n := 0
				for groundWall(x+n, y) && x+n < groundSide {
					n++
				}
				runs = append(runs, aabbworld.Item{ID: id, Box: plane.NewAABB(geom.NewVec(float64(x)*groundCell, float64(y)*groundCell), float64(n)*groundCell, groundCell), Caps: aabbworld.CanCollide | aabbworld.Static})
				id++
			}
		}
	}
	for _, v := range []struct {
		name  string
		walls []aabbworld.Item
		field collide.SolidField
	}{
		{"ground=field", nil, newWallGround()},
		{"ground=walls", nil, segmentGround{newWallGround()}},
		{"ground=run-entities", runs, nil},
		{"ground=cell-entities", cells, nil},
	} {
		b.Run(v.name, func(b *testing.B) {
			space, err := aabbworld.NewSpace(aabbworld.Config{Width: 2000, Height: 2000, BucketSize: 64})
			if err != nil {
				b.Fatal(err)
			}
			engine := space.CollideEngine(quiet{}, collide.Config{Reach: 0.5, Iterations: 16, Field: v.field})
			items := append(append([]aabbworld.Item{}, units...), v.walls...)
			b.ReportAllocs()
			for b.Loop() {
				copy(items, units)
				space.Rebuild(items)
				engine.Tick()
			}
		})
	}
}
