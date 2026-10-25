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

// slanted is solid ground of walls alone, or of boxes alone: a WallField the tests hold against
// the boxes it stands for.
type slanted struct {
	walls []collide.FieldWall
	boxes []collide.FieldBox
	asked int
}

func (f *slanted) Solid(_ uid.UID64, box geom.AABB, visit func(collide.FieldBox) bool) {
	f.asked++
	for _, b := range f.boxes {
		if b.Box.Intersects(box) && !visit(b) {
			return
		}
	}
}

func (f *slanted) Walls(_ uid.UID64, box geom.AABB, visit func(collide.FieldWall) bool) {
	for _, w := range f.walls {
		lo := geom.NewVec(min(w.A.X, w.B.X)-w.Half, min(w.A.Y, w.B.Y)-w.Half)
		hi := geom.NewVec(max(w.A.X, w.B.X)+w.Half, max(w.A.Y, w.B.Y)+w.Half)
		if geom.NewAABB(lo, hi).Intersects(box) && !visit(w) {
			return
		}
	}
}

// slantedScene is a scene whose engine pushes out of field.
func slantedScene(t *testing.T, field *slanted) (*scene, *fieldHooks) {
	t.Helper()
	s := &scene{space: engineSpace(t, false), items: make([]aabbworld.Item, 0, 64)}
	h := &fieldHooks{}
	h.moved = func(id uid.UID64, _ plane.AABB) { s.moved = append(s.moved, id) }
	s.engine = s.space.CollideEngine(h, collide.Config{Reach: 0.5, Iterations: 16, Field: field})
	return s, h
}

// rest is where a 10 by 10 box put at x, y comes to rest after a tick over field.
func rest(t *testing.T, field *slanted, x, y float32) geom.Vec {
	t.Helper()
	s, _ := slantedScene(t, field)
	box := s.put(1, x, y)
	s.tick(nil, nil)
	return box.TopLeft
}

func near(a, b geom.Vec) bool { return math.Hypot(float64(a.X-b.X), float64(a.Y-b.Y)) < 1e-9 }

// A wall along an axis pushes a box out as the box of the same rectangle does, open along its
// long sides and at its free ends.
func TestWall_AlongAnAxisPushesAsTheBoxItCovers(t *testing.T) {
	for _, open := range []collide.Ends{0, collide.EndA, collide.EndB, collide.EndA | collide.EndB} {
		wall := collide.FieldWall{A: geom.NewVec(100, 300), B: geom.NewVec(200, 300), Half: 3,
			Cell: 7, Open: open}
		sides := collide.Top | collide.Bottom
		if open&collide.EndA != 0 {
			sides |= collide.Left
		}
		if open&collide.EndB != 0 {
			sides |= collide.Right
		}
		box := collide.FieldBox{Box: geom.NewAABB(geom.NewVec(100, 297), geom.NewVec(200, 303)),
			Cell: 7, Open: sides}
		for x := float32(85.37); x <= 205; x += 7 { // off whole numbers: no box lies as deep in two ways
			for y := float32(286.21); y <= 304; y += 3 {
				got := rest(t, &slanted{walls: []collide.FieldWall{wall}}, x, y)
				want := rest(t, &slanted{boxes: []collide.FieldBox{box}}, x, y)
				if !near(got, want) {
					t.Fatalf("ends open %b, a box at (%v, %v): the wall left it at %v, the box at %v",
						open, x, y, got, want)
				}
			}
		}
	}
}

// A slanted wall pushes a box square to itself, out of it.
func TestWall_SlantedPushesSquareToItself(t *testing.T) {
	wall := collide.FieldWall{A: geom.NewVec(100, 100), B: geom.NewVec(300, 300), Half: 1, Cell: 1}
	s, _ := slantedScene(t, &slanted{walls: []collide.FieldWall{wall}})
	box := s.put(1, 196, 186) // its middle at (201, 191), 7 off the wall's line
	s.tick(nil, nil)
	moved := box.TopLeft.Sub(geom.NewVec(196, 186))
	if math.Abs(float64(moved.X+moved.Y)) > 1e-9 || moved.X <= 0 {
		t.Errorf("moved by %v, want square to the wall, up and to the right", moved)
	}
	if push, ok := apart(*box, wall); !ok {
		t.Errorf("the box at %v still overlaps the wall by %v", box.TopLeft, push)
	}
}

// apart reports whether box a and wall w no longer overlap, by the distance from the box's corners
// to the wall's line against the box's reach across it.
func apart(a plane.AABB, w collide.FieldWall) (float64, bool) {
	dx, dy := float64(w.B.X-w.A.X), float64(w.B.Y-w.A.Y)
	l := math.Hypot(dx, dy)
	nx, ny := -dy/l, dx/l
	cx := float64(a.TopLeft.X) + float64(a.Size.X)/2 - float64(w.A.X)
	cy := float64(a.TopLeft.Y) + float64(a.Size.Y)/2 - float64(w.A.Y)
	reach := float64(a.Size.X)/2*math.Abs(nx) + float64(a.Size.Y)/2*math.Abs(ny) + float64(w.Half)
	over := reach - math.Abs(cx*nx+cy*ny)
	return over, over <= 1e-9
}

// Ten pieces joined end to end push a box straddling a joint as one wall does: it slides on.
func TestWall_PiecesJoinedEndToEndPushAsOne(t *testing.T) {
	one := collide.FieldWall{A: geom.NewVec(100, 100), B: geom.NewVec(400, 400), Half: 1, Cell: 1}
	var pieces []collide.FieldWall
	for k := range 10 {
		a := geom.NewVec(100+30*float32(k), 100+30*float32(k))
		b := geom.NewVec(130+30*float32(k), 130+30*float32(k))
		open := collide.Ends(0)
		if k == 0 {
			open |= collide.EndA
		}
		if k == 9 {
			open |= collide.EndB
		}
		pieces = append(pieces, collide.FieldWall{A: a, B: b, Half: 1, Cell: uint64(k), Open: open})
	}
	one.Open = collide.EndA | collide.EndB
	for _, at := range []geom.Vec{geom.NewVec(186, 186), geom.NewVec(215, 205), geom.NewVec(240, 228)} {
		got := rest(t, &slanted{walls: pieces}, at.X, at.Y)
		want := rest(t, &slanted{walls: []collide.FieldWall{one}}, at.X, at.Y)
		if !near(got, want) {
			t.Errorf("a box at %v: the pieces left it at %v, one wall at %v", at, got, want)
		}
	}
}

// Past a free end a box is pushed round the end, along the wall; at a joined end, square to it.
func TestWall_RoundAFreeEndAlongIt(t *testing.T) {
	free := collide.FieldWall{A: geom.NewVec(100, 300), B: geom.NewVec(200, 300), Half: 2,
		Cell: 1, Open: collide.EndB}
	if at := rest(t, &slanted{walls: []collide.FieldWall{free}}, 198, 296); !near(at,
		geom.NewVec(200, 296)) {
		t.Errorf("past the free end the box rests at %v, want (200, 296): round the end", at)
	}
	joined := free
	joined.Open = 0
	if at := rest(t, &slanted{walls: []collide.FieldWall{joined}}, 198, 296); at.X != 198 {
		t.Errorf("past a joined end the box rests at %v, want it pushed square to the wall", at)
	}
}

func TestWall_ASensorIsToldButNeverPushedAndACellMayBeVetoed(t *testing.T) {
	wall := collide.FieldWall{A: geom.NewVec(100, 300), B: geom.NewVec(200, 300), Half: 3, Cell: 5}
	s, h := slantedScene(t, &slanted{walls: []collide.FieldWall{wall}})
	box := s.put(1, 150, 292)
	s.mark(1, aabbworld.Sensor)
	told := 0
	h.contactField = func(_ uid.UID64, cell uint64, _ geom.Vec) {
		if cell == 5 {
			told++
		}
	}
	s.tick(nil, nil)
	if box.TopLeft != geom.NewVec(150, 292) || told != 1 {
		t.Errorf("sensor at %v, told %d times; want it left where it was and told once", box.TopLeft,
			told)
	}
	s, h = slantedScene(t, &slanted{walls: []collide.FieldWall{wall}})
	box = s.put(1, 150, 292)
	h.touchField = func(uid.UID64, uint64, geom.Vec) (geom.Vec, bool) { return geom.Vec{}, false }
	s.tick(nil, nil)
	if box.TopLeft != geom.NewVec(150, 292) {
		t.Errorf("a vetoed wall pushed the box to %v", box.TopLeft)
	}
}

func TestWall_TickDoesNotAllocateOnceWarm(t *testing.T) {
	field := &slanted{}
	for k := range 10 {
		a := geom.NewVec(100+30*float32(k), 100+30*float32(k))
		field.walls = append(field.walls, collide.FieldWall{A: a, B: a.Add(geom.NewVec(30, 30)),
			Half: 1, Cell: uint64(k)})
	}
	s, _ := slantedScene(t, field)
	for i := range 20 {
		s.put(uid.UID64(i+1), 100+float32(i)*14, 100+float32(i)*14-12)
	}
	s.hooks.moved = nil
	s.space.Rebuild(s.items)
	s.engine.Tick()
	allocs := testing.AllocsPerRun(20, func() {
		s.space.Rebuild(s.items)
		s.engine.Tick()
	})
	if allocs > 0 {
		t.Errorf("a warm tick with walls allocates %.0f times, want none", allocs)
	}
}
