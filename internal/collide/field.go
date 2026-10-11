package collide

import (
	"math"

	pcollide "github.com/kjkrol/aabbworld/collide"
	"github.com/kjkrol/aabbworld/geom"
	iplane "github.com/kjkrol/aabbworld/internal/plane"
	"github.com/kjkrol/aabbworld/internal/spatial"
)

// Field is how a solve meets the solid ground: the boxes round an item, and whom to tell of them.
type Field struct {
	Solid   pcollide.SolidField
	Walls   pcollide.WallField // the Solid's walls, when it has any
	Touch   func(item int32, cell uint64, pen geom.Vec) (geom.Vec, bool)
	Contact func(item int32, cell uint64, pen geom.Vec)
}

// fieldKey is one item against one cell of the ground.
type fieldKey struct {
	item int32
	cell uint64
}

// What a tick has settled of an item against a cell.
const (
	fieldReported uint8 = 1 + iota
	fieldVetoed
)

// fieldPass is the state of pushing one item out of the ground.
type fieldPass struct {
	field   *Field
	surface *iplane.Surface
	item    int32
	it      *spatial.Item
	sensor  bool
	pushed  bool
	met     map[fieldKey]uint8
	visit   func(pcollide.FieldBox) bool
	wall    func(pcollide.FieldWall) bool
}

// meet pushes the item out of one solid box it overlaps, through the shallowest open side. It
// settles the box as settle does, written out here: called for every box met, the call to settle
// cost a twentieth of a tick.
func (p *fieldPass) meet(fb pcollide.FieldBox) bool {
	box := &p.it.Box
	a := geom.AABB{TopLeft: box.TopLeft, BottomRight: box.TopLeft.Add(box.Size)}
	b := fb.Box
	if a.BottomRight.X <= b.TopLeft.X || a.TopLeft.X >= b.BottomRight.X || a.BottomRight.Y <= b.TopLeft.Y || a.TopLeft.Y >= b.BottomRight.Y {
		return true
	}
	push := pushOut(a, b, fb.Open)
	key := fieldKey{p.item, fb.Cell}
	switch p.met[key] {
	case fieldVetoed:
		return true
	case 0:
		if p.field.Touch != nil {
			pen, ok := p.field.Touch(p.item, fb.Cell, push)
			if !ok {
				p.met[key] = fieldVetoed
				return true
			}
			push = pen
		}
		p.met[key] = fieldReported
		if p.field.Contact != nil {
			p.field.Contact(p.item, fb.Cell, push)
		}
	}
	if p.sensor || push == (geom.Vec{}) {
		return true
	}
	p.surface.Translate(box, push)
	p.pushed = true
	return true
}

// meetWall pushes the item out of one wall it overlaps.
func (p *fieldPass) meetWall(w pcollide.FieldWall) bool {
	box := &p.it.Box
	a := geom.AABB{TopLeft: box.TopLeft, BottomRight: box.TopLeft.Add(box.Size)}
	if push, ok := wallPush(a, w); ok {
		p.settle(w.Cell, push)
	}
	return true
}

// settle tells the handler of the item meeting cell of the ground, once a tick, and pushes it out
// by push, or by what the handler made of it.
func (p *fieldPass) settle(cell uint64, push geom.Vec) {
	key := fieldKey{p.item, cell}
	switch p.met[key] {
	case fieldVetoed:
		return
	case 0:
		if p.field.Touch != nil {
			pen, ok := p.field.Touch(p.item, cell, push)
			if !ok {
				p.met[key] = fieldVetoed
				return
			}
			push = pen
		}
		p.met[key] = fieldReported
		if p.field.Contact != nil {
			p.field.Contact(p.item, cell, push)
		}
	}
	if p.sensor || push == (geom.Vec{}) {
		return
	}
	p.surface.Translate(&p.it.Box, push)
	p.pushed = true
}

// wallPush is the way out of wall w for box a, square to the wall towards the side a's middle is
// on, or past a free end along it when that is shorter; false when they do not overlap — touching
// is not overlapping. It tests the four axes that can part them: x, y, the wall's and across it.
func wallPush(a geom.AABB, w pcollide.FieldWall) (geom.Vec, bool) {
	ex, ey := (a.BottomRight.X-a.TopLeft.X)/2, (a.BottomRight.Y-a.TopLeft.Y)/2
	vx, vy := w.B.X-w.A.X, w.B.Y-w.A.Y
	length := math.Hypot(vx, vy)
	dx, dy := 1.0, 0.0
	if length > 0 {
		dx, dy = vx/length, vy/length
	}
	nx, ny := -dy, dx
	half, h := length/2, w.Half
	cx, cy := a.TopLeft.X+ex-(w.A.X+w.B.X)/2, a.TopLeft.Y+ey-(w.A.Y+w.B.Y)/2
	alongX := ex + half*math.Abs(dx) + h*math.Abs(nx) - math.Abs(cx)
	alongY := ey + half*math.Abs(dy) + h*math.Abs(ny) - math.Abs(cy)
	s, t := cx*nx+cy*ny, cx*dx+cy*dy
	across := ex*math.Abs(nx) + ey*math.Abs(ny) + h - math.Abs(s)
	along := ex*math.Abs(dx) + ey*math.Abs(dy) + half - math.Abs(t)
	if alongX <= 0 || alongY <= 0 || across <= 0 || along <= 0 {
		return geom.Vec{}, false
	}
	side, end, way := 1.0, pcollide.EndA, -1.0
	if s <= 0 { // a box on the line goes back across it, as a box leaves a box by its first side
		side = -1
	}
	if t > 0 {
		end, way = pcollide.EndB, 1
	}
	if w.Open&end != 0 && along < across {
		return geom.NewVec(way*along*dx, way*along*dy), true
	}
	return geom.NewVec(side*across*nx, side*across*ny), true
}

// pushOut is the shortest way out of b for a through one of b's open sides; with none open, the
// shortest way out at all.
func pushOut(a, b geom.AABB, open pcollide.Sides) geom.Vec {
	ways := [4]struct {
		side pcollide.Sides
		v    geom.Vec
		d    float64
	}{
		{pcollide.Left, geom.NewVec(b.TopLeft.X-a.BottomRight.X, 0), a.BottomRight.X - b.TopLeft.X},
		{pcollide.Right, geom.NewVec(b.BottomRight.X-a.TopLeft.X, 0), b.BottomRight.X - a.TopLeft.X},
		{pcollide.Top, geom.NewVec(0, b.TopLeft.Y-a.BottomRight.Y), a.BottomRight.Y - b.TopLeft.Y},
		{pcollide.Bottom, geom.NewVec(0, b.BottomRight.Y-a.TopLeft.Y), b.BottomRight.Y - a.TopLeft.Y},
	}
	best := -1
	for i, w := range ways {
		if open&w.side != 0 && (best < 0 || w.d < ways[best].d) {
			best = i
		}
	}
	if best < 0 {
		for i, w := range ways {
			if best < 0 || w.d < ways[best].d {
				best = i
			}
		}
	}
	return ways[best].v
}

// movable reports whether an item takes part in collisions and something may shift it.
func movable(it *spatial.Item) bool {
	return it.Caps&spatial.CanCollide != 0 && it.Caps&spatial.Static == 0
}
