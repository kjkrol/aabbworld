package collide

import (
	pcollide "github.com/kjkrol/aabbworld/collide"
	"github.com/kjkrol/aabbworld/geom"
	iplane "github.com/kjkrol/aabbworld/internal/plane"
	"github.com/kjkrol/aabbworld/internal/spatial"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

// Handler is told what an Engine finds: each overlapping pair once, and every box it pushed.
type Handler interface {
	Touch(a, b uid.UID64, pen geom.Vec) (geom.Vec, bool)
	Contact(a, b uid.UID64, pen geom.Vec)
	Moved(id uid.UID64, box plane.AABB)
}

// Engine finds and separates the overlapping items of one grid; one Engine serves one goroutine.
type Engine struct {
	grid       *spatial.Grid
	surface    *iplane.Surface
	handler    Handler
	reach      float32
	iterations int

	solver Solver
	items  []spatial.Entry
	left   []pcollide.Leaver
	moved  []uint32
	stamp  uint32

	onPair func(a, b int32)
	touch  Touch
	tell   func(i int, pen geom.Vec)
	visit  func(item int32)
	field  *Field
}

// New builds an Engine over the CanCollide items of grid, reporting to handler; field, when not
// nil, is solid ground the movable items are pushed out of.
func New(grid *spatial.Grid, surface *iplane.Surface, handler Handler, reach float32, iterations int, field pcollide.SolidField) *Engine {
	e := &Engine{grid: grid, surface: surface, handler: handler, reach: reach, iterations: iterations}
	e.onPair, e.touch, e.tell, e.visit = e.add, e.ask, e.report, e.pushed
	if field != nil {
		e.field = &Field{Solid: field}
		if wf, ok := field.(pcollide.WallField); ok {
			e.field.Walls = wf
		}
		if fh, ok := handler.(pcollide.FieldHandler); ok {
			e.field.Touch = func(item int32, cell uint64, pen geom.Vec) (geom.Vec, bool) {
				return fh.TouchField(e.items[item].ID, cell, pen)
			}
			e.field.Contact = func(item int32, cell uint64, pen geom.Vec) { fh.ContactField(e.items[item].ID, cell, pen) }
		}
	}
	return e
}

// Tick separates every overlapping pair the grid holds now and reports as it goes, pushing the
// boxes where they lie. The grid answers its next Query with them on its own; no Rebuild is owed.
func (e *Engine) Tick() {
	e.items = e.grid.Items()
	e.solver.Reset(len(e.items))
	e.left = e.left[:0]
	e.grid.Pairs(e.reach, spatial.CanCollide, e.onPair)
	e.solver.Solve(e.items, e.surface, e.iterations, e.touch, e.tell, e.field)

	e.stamp++
	if cap(e.moved) < len(e.items) {
		e.moved = make([]uint32, len(e.items))
	}
	e.moved = e.moved[:len(e.items)]
	e.solver.VisitMoved(e.visit)
	e.grid.Invalidate()
}

// Left is who the last Tick pushed out through an open edge, and where to; good until the next
// Tick.
func (e *Engine) Left() []pcollide.Leaver { return e.left }

// add takes a pair the grid found, lower entity id first, as the handler has always heard them.
func (e *Engine) add(a, b int32) {
	if e.items[a].ID.Index() > e.items[b].ID.Index() {
		a, b = b, a
	}
	e.solver.Add(Pair{A: a, B: b, Flags: flagsOf(e.items[a].Caps, e.items[b].Caps)})
}

func (e *Engine) ask(i int, pen geom.Vec) (geom.Vec, bool) {
	p := e.solver.Pair(i)
	return e.handler.Touch(e.items[p.A].ID, e.items[p.B].ID, pen)
}

func (e *Engine) report(i int, pen geom.Vec) {
	p := e.solver.Pair(i)
	e.handler.Contact(e.items[p.A].ID, e.items[p.B].ID, pen)
}

func (e *Engine) pushed(item int32) {
	if e.moved[item] == e.stamp {
		return
	}
	e.moved[item] = e.stamp
	it := &e.items[item]
	if e.surface.Left(it.Box) {
		e.left = append(e.left, pcollide.Leaver{ID: it.ID, Box: *it.Box})
		return
	}
	e.handler.Moved(it.ID, *it.Box)
}

func flagsOf(a, b spatial.Capability) uint8 {
	var f uint8
	if a&spatial.Static != 0 {
		f |= StaticA
	}
	if b&spatial.Static != 0 {
		f |= StaticB
	}
	if (a|b)&spatial.Sensor != 0 {
		f |= Sensor
	}
	return f
}
