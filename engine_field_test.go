package aabbworld_test

import (
	"math"
	"testing"

	"github.com/kjkrol/aabbworld"
	"github.com/kjkrol/aabbworld/collide"
	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

// wallField is solid ground on a grid of square cells.
type wallField struct {
	cell  float32
	solid map[[2]int]bool
	asked int
}

func newWallField(cell float32) *wallField { return &wallField{cell: cell, solid: map[[2]int]bool{}} }

func (f *wallField) set(x, y int) { f.solid[[2]int{x, y}] = true }

func (f *wallField) Solid(_ uid.UID64, box geom.AABB, visit func(collide.FieldBox) bool) {
	f.asked++
	x0, y0 := int(math.Floor(float64(box.TopLeft.X/f.cell))), int(math.Floor(float64(box.TopLeft.Y/f.cell)))
	x1, y1 := int(math.Ceil(float64(box.BottomRight.X/f.cell))), int(math.Ceil(float64(box.BottomRight.Y/f.cell)))
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if !f.solid[[2]int{x, y}] {
				continue
			}
			var open collide.Sides
			for _, n := range []struct {
				dx, dy int
				side   collide.Sides
			}{{-1, 0, collide.Left}, {1, 0, collide.Right}, {0, -1, collide.Top}, {0, 1, collide.Bottom}} {
				if !f.solid[[2]int{x + n.dx, y + n.dy}] {
					open |= n.side
				}
			}
			b := geom.NewAABBAt(geom.NewVec(float32(x)*f.cell, float32(y)*f.cell), f.cell, f.cell)
			if !visit(collide.FieldBox{Box: b, Cell: uint64(y)<<32 | uint64(x), Open: open}) {
				return
			}
		}
	}
}

// fieldHooks is hooks told about the ground too.
type fieldHooks struct {
	hooks
	touchField   func(id uid.UID64, cell uint64, pen geom.Vec) (geom.Vec, bool)
	contactField func(id uid.UID64, cell uint64, pen geom.Vec)
}

func (h *fieldHooks) TouchField(id uid.UID64, cell uint64, pen geom.Vec) (geom.Vec, bool) {
	if h.touchField == nil {
		return pen, true
	}
	return h.touchField(id, cell, pen)
}

func (h *fieldHooks) ContactField(id uid.UID64, cell uint64, pen geom.Vec) {
	if h.contactField != nil {
		h.contactField(id, cell, pen)
	}
}

// fieldScene is a scene whose engine pushes out of field.
func fieldScene(t *testing.T, field *wallField) (*scene, *fieldHooks) {
	t.Helper()
	s := &scene{space: engineSpace(t, false), items: make([]aabbworld.Item, 0, 64)}
	h := &fieldHooks{}
	h.moved = func(id uid.UID64, _ plane.AABB) { s.moved = append(s.moved, id) }
	s.engine = s.space.CollideEngine(h, collide.Config{Reach: 0.5, Iterations: 16, Field: field})
	return s, h
}

// wallRow is cells x0..x1 of row y, solid.
func wallRow(cell float32, y, x0, x1 int) *wallField {
	f := newWallField(cell)
	for x := x0; x <= x1; x++ {
		f.set(x, y)
	}
	return f
}

// The field pushes a box out as one static entity spanning the whole wall would; a wall of one
// static entity per cell pushes it sideways both ways at a seam and leaves it in the wall.
func TestField_PushesABoxOutAsOneWallEntityWould(t *testing.T) {
	withField, _ := fieldScene(t, wallRow(20, 10, 0, 9)) // y 200 to 220, x 0 to 200
	box := withField.put(1, 50, 203)                     // 7 into the wall from above
	withField.tick(nil, nil)

	entity := newScene(t, false)
	entity.put(1, 50, 203)
	entity.put(100, 0, 200)
	entity.items[1].Box = plane.NewAABB(geom.NewVec(0, 200), 200, 20)
	entity.mark(100, aabbworld.Static)
	entity.tick(nil, nil)

	if want := entity.items[0].Box.TopLeft; box.TopLeft != want || want != geom.NewVec(50, 190) {
		t.Errorf("pushed out of the field to %v, out of one wall entity to %v; want both (50, 190)", box.TopLeft, want)
	}
}

func TestField_ABoxSlidesAlongAWallOfManyCellsWithoutCatchingOnTheSeams(t *testing.T) {
	s, _ := fieldScene(t, wallRow(20, 10, 0, 9))
	box := s.put(1, 95, 192) // 2 into the wall's top, straddling the seam at x 100
	s.tick(nil, nil)
	if box.TopLeft.X != 95 || box.TopLeft.Y != 190 {
		t.Errorf("the box came to rest at %v, want (95, 190): up out of the wall, not sideways at the seam", box.TopLeft)
	}
}

func TestField_AnEnclosedCellPushesOutTheShortestWay(t *testing.T) {
	field := newWallField(20)
	for x := 10; x < 13; x++ {
		for y := 10; y < 13; y++ {
			field.set(x, y)
		}
	}
	s, _ := fieldScene(t, field)
	box := s.put(1, 222, 222) // inside the middle cell, no side of it open
	s.tick(nil, nil)
	if box.BottomRight.X > 200 && box.TopLeft.X < 260 && box.BottomRight.Y > 200 && box.TopLeft.Y < 260 {
		t.Errorf("the box rests at %v, still inside the solid block", box.TopLeft)
	}
}

func TestField_ASecondBoxPushesTheFirstIntoTheWallAndTheWallHolds(t *testing.T) {
	s, _ := fieldScene(t, wallRow(20, 10, 0, 9))
	a := s.put(1, 50, 188) // 2 short of the wall
	b := s.put(2, 50, 182) // 4 into a from above
	s.tick(nil, nil)
	if a.BottomRight.Y > 200+1e-9 {
		t.Errorf("a ends at y %v, pushed into the wall", a.BottomRight.Y)
	}
	if d := b.BottomRight.Y - a.TopLeft.Y; d > 1e-9 {
		t.Errorf("a and b still overlap by %v", d)
	}
}

func TestField_ASensorIsToldButNeverPushed(t *testing.T) {
	s, h := fieldScene(t, wallRow(20, 10, 0, 9))
	box := s.put(1, 50, 203)
	s.mark(1, aabbworld.Sensor)
	told := 0
	h.contactField = func(uid.UID64, uint64, geom.Vec) { told++ }
	s.tick(nil, nil)
	if box.TopLeft != geom.NewVec(50, 203) || told != 1 {
		t.Errorf("sensor at %v, told %d times; want it left at (50, 203) and told once", box.TopLeft, told)
	}
}

func TestField_TouchFieldMayVetoACell(t *testing.T) {
	s, h := fieldScene(t, wallRow(20, 10, 0, 9))
	box := s.put(1, 50, 203)
	h.touchField = func(uid.UID64, uint64, geom.Vec) (geom.Vec, bool) { return geom.Vec{}, false }
	s.tick(nil, nil)
	if box.TopLeft != geom.NewVec(50, 203) {
		t.Errorf("a vetoed cell pushed the box to %v", box.TopLeft)
	}
}

func TestField_ReportsEachCellOnceATickWithThePushThatFreesTheBox(t *testing.T) {
	s, h := fieldScene(t, wallRow(20, 10, 0, 9))
	s.put(1, 95, 192) // over two cells
	cells := map[uint64]geom.Vec{}
	h.contactField = func(id uid.UID64, cell uint64, pen geom.Vec) {
		if _, again := cells[cell]; again {
			t.Errorf("cell %d reported twice in one tick", cell)
		}
		cells[cell] = pen
	}
	s.tick(nil, nil)
	if len(cells) == 0 {
		t.Fatal("no contact with the ground reported")
	}
	for cell, pen := range cells {
		if pen != geom.NewVec(0, -2) {
			t.Errorf("cell %d reported with push %v, want (0, -2) up out of the wall", cell, pen)
		}
	}
}

func TestField_AStaticBoxNeverAsksTheGround(t *testing.T) {
	field := wallRow(20, 10, 0, 9)
	s, _ := fieldScene(t, field)
	box := s.put(1, 50, 203)
	s.mark(1, aabbworld.Static)
	s.tick(nil, nil)
	if field.asked != 0 || box.TopLeft != geom.NewVec(50, 203) {
		t.Errorf("a static box asked the ground %d times and rests at %v", field.asked, box.TopLeft)
	}
}

func TestField_TickDoesNotAllocateOnceWarm(t *testing.T) {
	s, _ := fieldScene(t, wallRow(20, 10, 0, 9))
	for i := range 20 {
		s.put(uid.UID64(i+1), float32(i)*9, 192)
	}
	s.hooks.moved = nil
	s.space.Rebuild(s.items)
	s.engine.Tick()
	allocs := testing.AllocsPerRun(20, func() {
		s.space.Rebuild(s.items)
		s.engine.Tick()
	})
	if allocs > 0 {
		t.Errorf("a warm tick with a field allocates %.0f times, want none", allocs)
	}
}
