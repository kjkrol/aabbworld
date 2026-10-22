package spatial

import (
	"slices"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/kjkrol/aabbworld/geom"
	iplane "github.com/kjkrol/aabbworld/internal/plane"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

// Item is one box a Grid is told of at Rebuild.
type Item struct {
	ID   uid.UID64
	Box  plane.AABB
	Caps Capability
}

// Run is a column of records in memory the caller owns: record i starts i·Stride bytes past
// Base, with its box at BoxOff and its Capability at CapsOff, and IDs[i] names it.
type Run struct {
	IDs     []uid.UID64
	Base    unsafe.Pointer
	Stride  uintptr
	BoxOff  uintptr
	CapsOff uintptr
}

// Entry is one item of the last Rebuild: its box where it lies, its id and its capability.
type Entry struct {
	Box  *plane.AABB
	ID   uid.UID64
	Caps Capability
}

// column is a Run, or the items of a Rebuild, as the grid walks it.
type column struct {
	rec, id          unsafe.Pointer // the first record, its id
	stride, idStride uintptr
	boxOff, capsOff  uintptr
	n                int
}

func (c *column) box(i int) *plane.AABB {
	return (*plane.AABB)(unsafe.Add(c.rec, uintptr(i)*c.stride+c.boxOff))
}

func (c *column) caps(i int) *Capability {
	return (*Capability)(unsafe.Add(c.rec, uintptr(i)*c.stride+c.capsOff))
}

func (c *column) idAt(i int) uid.UID64 {
	return *(*uid.UID64)(unsafe.Add(c.id, uintptr(i)*c.idStride))
}

// Grid is a cell grid over the records of its last Rebuild, positioned in order across its runs
// and then its items. Pair sweeps read the records as they are; the cells are built from them at
// the first Query after a Rebuild or an Invalidate.
type Grid struct {
	surface  *iplane.Surface
	cellRes  Resolution
	side     uint32
	maxCoord uint32

	columns  []column
	count    int
	rebuilds uint32
	entries  []Entry
	listed   uint32 // the Rebuild entries were listed for
	slotOf   []int32
	stamps   []uint32
	mapped   uint32      // the Rebuild slotOf was filled for
	stale    atomic.Bool // the cells are behind
	unlisted atomic.Bool // the items and the ids' positions are behind
	mu       sync.Mutex  // whoever reads first after a Rebuild or Invalidate brings the grid up

	pieces []gridPiece
	starts []uint32
	cursor []uint32
	cells  []gridCell
	sweep  pairSweep

	listingID   uid.UID64
	listingCaps Capability
	onImage     plane.FragVisitor
}

// gridPiece is one image of an item as the grid holds it, and the cells it covers.
type gridPiece struct {
	gridCell
	x1, y1, x2, y2 uint32
}

type gridCell struct {
	box  geom.AABB
	id   uid.UID64
	caps Capability
}

// NewGrid builds an empty grid over surface, worldRes on a side, cellRes per cell.
func NewGrid(surface *iplane.Surface, worldRes, cellRes Resolution) *Grid {
	g := &Grid{
		surface:  surface,
		cellRes:  cellRes,
		side:     NewResolution(uint8(worldRes - cellRes)).Side(),
		maxCoord: worldRes.MaxCoord(),
	}
	g.onImage = g.addImage
	g.Rebuild(nil)
	return g
}

// Rebuild replaces what the grid holds with items, which must stay put until the next Rebuild.
// Their boxes may change in the meantime; Invalidate tells the grid so. The cells are built at
// the first Query that needs them, so several Rebuilds in a row cost one indexing.
func (g *Grid) Rebuild(items []Item) { g.RebuildRuns(nil, items) }

// RebuildRuns is Rebuild over the records of runs and then items: the memory of both must hold
// the same records in the same places until the next Rebuild, and their capabilities with them.
func (g *Grid) RebuildRuns(runs []Run, items []Item) {
	clear(g.columns)
	g.columns, g.count = g.columns[:0], 0
	for i := range runs {
		r := &runs[i]
		if len(r.IDs) == 0 {
			continue
		}
		g.columns = append(g.columns, column{
			rec: r.Base, id: unsafe.Pointer(unsafe.SliceData(r.IDs)),
			stride: r.Stride, idStride: unsafe.Sizeof(uid.UID64(0)),
			boxOff: r.BoxOff, capsOff: r.CapsOff, n: len(r.IDs),
		})
		g.count += len(r.IDs)
	}
	if len(items) > 0 {
		var it Item
		first := unsafe.Pointer(unsafe.SliceData(items))
		g.columns = append(g.columns, column{
			rec: first, id: unsafe.Add(first, unsafe.Offsetof(it.ID)),
			stride: unsafe.Sizeof(it), idStride: unsafe.Sizeof(it),
			boxOff: unsafe.Offsetof(it.Box), capsOff: unsafe.Offsetof(it.Caps), n: len(items),
		})
		g.count += len(items)
	}
	g.rebuilds++
	g.unlisted.Store(true)
	g.stale.Store(true)
}

// Invalidate marks the cells as behind the boxes of the items, which have moved since they were built.
func (g *Grid) Invalidate() { g.stale.Store(true) }

// Ready brings the cells, the items and the ids' positions up to the last Rebuild or Invalidate:
// the first reader after them does, under the lock, the others waiting; then every reader only
// reads, so several goroutines may read the grid at once while nothing writes it.
func (g *Grid) Ready() {
	if g.stale.Load() {
		g.bringUp()
	}
}

func (g *Grid) bringUp() {
	g.mu.Lock()
	if g.stale.Load() {
		g.index()
		g.unlisted.Store(false)
		g.stale.Store(false)
	}
	g.mu.Unlock()
}

// ready for the items and the ids' positions alone
func (g *Grid) readyListed() {
	if g.unlisted.Load() {
		g.listUp()
	}
}

func (g *Grid) listUp() {
	g.mu.Lock()
	if g.unlisted.Load() {
		g.mapIDs()
		g.unlisted.Store(false)
	}
	g.mu.Unlock()
}

// index builds the cells from the records as they are now, and the ids' positions once a Rebuild
func (g *Grid) index() {
	if g.mapped != g.rebuilds {
		g.mapIDs()
	}
	g.pieces = g.pieces[:0]
	cells := int(g.side) * int(g.side)
	if cap(g.starts) < cells+1 {
		g.starts = make([]uint32, cells+1)
		g.cursor = make([]uint32, cells)
	}
	g.starts = g.starts[:cells+1]
	g.cursor = g.cursor[:cells]
	clear(g.starts)

	for ci := range g.columns {
		c := &g.columns[ci]
		for i := range c.n {
			box, caps, id := c.box(i), *c.caps(i), c.idAt(i)
			if main, ok := g.clamp(box.AABB); ok {
				g.add(main, id, caps)
			}
			if box.Overhang != (geom.Vec{}) {
				g.listingID, g.listingCaps = id, caps
				box.VisitFragments(g.onImage)
			}
		}
	}

	for i := 1; i < len(g.starts); i++ {
		g.starts[i] += g.starts[i-1]
	}
	copy(g.cursor, g.starts)
	total := int(g.starts[cells])
	if cap(g.cells) < total {
		g.cells = make([]gridCell, total)
	}
	g.cells = g.cells[:total]
	for i := range g.pieces {
		p := &g.pieces[i]
		for y := p.y1; y <= p.y2; y++ {
			for x := p.x1; x <= p.x2; x++ {
				cell := y*g.side + x
				g.cells[g.cursor[cell]] = p.gridCell
				g.cursor[cell]++
			}
		}
	}
}

func (g *Grid) addImage(_ plane.FragPosition, image geom.AABB) bool {
	if frag, ok := g.clamp(image); ok {
		g.add(frag, g.listingID, g.listingCaps)
	}
	return true
}

func (g *Grid) add(box geom.AABB, id uid.UID64, caps Capability) {
	p := gridPiece{
		gridCell: gridCell{box: box, id: id, caps: caps},
		x1:       g.cellOf(box.TopLeft.X), y1: g.cellOf(box.TopLeft.Y),
		x2: g.cellOf(box.BottomRight.X), y2: g.cellOf(box.BottomRight.Y),
	}
	for y := p.y1; y <= p.y2; y++ {
		for x := p.x1; x <= p.x2; x++ {
			g.starts[y*g.side+x+1]++
		}
	}
	g.pieces = append(g.pieces, p)
}

// mapIDs notes the position of every id of the last Rebuild.
func (g *Grid) mapIDs() {
	g.mapped = g.rebuilds
	for at, e := range g.items() {
		g.remember(e.ID, int32(at))
	}
}

// remember maps the entity id names to its position for this Rebuild.
func (g *Grid) remember(id uid.UID64, at int32) {
	i := int(id.Index())
	for i >= len(g.slotOf) {
		g.slotOf = append(g.slotOf, 0)
		g.stamps = append(g.stamps, 0)
	}
	g.slotOf[i], g.stamps[i] = at, g.rebuilds
}

// Item is the item the entity id names, if the last Rebuild was told of it.
func (g *Grid) Item(id uid.UID64) (*Entry, bool) {
	g.readyListed()
	i := int(id.Index())
	if i >= len(g.slotOf) || g.stamps[i] != g.rebuilds {
		return nil, false
	}
	e := &g.entries[g.slotOf[i]]
	if e.ID != id {
		return nil, false
	}
	return e, true
}

// EntryAABB is the main box of the entity id names as the grid holds it.
func (g *Grid) EntryAABB(id uid.UID64) (geom.AABB, bool) {
	e, ok := g.Item(id)
	if !ok {
		return geom.AABB{}, false
	}
	return g.clamp(e.Box.AABB)
}

// Items is what the last Rebuild was told of, by position.
func (g *Grid) Items() []Entry {
	g.readyListed()
	return g.entries
}

func (g *Grid) items() []Entry {
	if g.listed != g.rebuilds {
		g.list()
	}
	return g.entries
}

func (g *Grid) list() {
	g.listed = g.rebuilds
	held := len(g.entries)
	g.entries = slices.Grow(g.entries[:0], g.count)
	for ci := range g.columns {
		c := &g.columns[ci]
		for i := range c.n {
			g.entries = append(g.entries, Entry{Box: c.box(i), ID: c.idAt(i), Caps: *c.caps(i)})
		}
	}
	if held > len(g.entries) {
		clear(g.entries[len(g.entries):held])
	}
}

// Query calls fn for every piece sharing want that intersects box, each once, and returns how many.
// It reads the boxes as they are now, building the cells first if they are behind.
func (g *Grid) Query(box geom.AABB, want Capability, fn func(uid.UID64)) int {
	area, ok := g.clamp(box)
	if !ok {
		return 0
	}
	g.Ready()
	x1, y1 := g.cellOf(area.TopLeft.X), g.cellOf(area.TopLeft.Y)
	x2, y2 := g.cellOf(area.BottomRight.X), g.cellOf(area.BottomRight.Y)
	found := 0
	for y := y1; y <= y2; y++ {
		for x := x1; x <= x2; x++ {
			cell := y*g.side + x
			for _, c := range g.cells[g.starts[cell]:g.starts[cell+1]] {
				if !c.caps.matches(want) || !area.Intersects(c.box) {
					continue
				}
				if max(g.cellOf(c.box.TopLeft.X), x1) != x || max(g.cellOf(c.box.TopLeft.Y), y1) != y {
					continue
				}
				fn(c.id)
				found++
			}
		}
	}
	return found
}

// Pairs calls fn once, lower position first, for every two items sharing want whose boxes grown
// by reach of their shorter side touch. It reads the boxes as they are now.
func (g *Grid) Pairs(reach float64, want Capability, fn func(a, b int32)) {
	s := &g.sweep
	s.shift, s.side = g.cellRes, g.side
	s.listItems(g.surface, g.columns, reach, want)
	s.sort()
	s.visit(fn)
	s.flushSeam(fn)
}

// cellOf is the column or row holding v, held inside the grid.
func (g *Grid) cellOf(v float64) uint32 {
	if !(v >= 0) {
		return 0
	}
	return min(uint32(v)>>g.cellRes, g.side-1)
}

// clamp holds box inside the grid; false when nothing of it is.
func (g *Grid) clamp(box geom.AABB) (geom.AABB, bool) {
	minX := clampToGrid(box.TopLeft.X, g.maxCoord)
	minY := clampToGrid(box.TopLeft.Y, g.maxCoord)
	maxX := clampToGrid(box.BottomRight.X, g.maxCoord)
	maxY := clampToGrid(box.BottomRight.Y, g.maxCoord)
	if maxX < minX || maxY < minY {
		return geom.AABB{}, false
	}
	return geom.NewAABB(geom.NewVec(minX, minY), geom.NewVec(maxX, maxY)), true
}

// clampToGrid holds a coordinate inside the indexable grid, NaN included.
func clampToGrid(val float64, max uint32) float64 {
	if !(val >= 0) {
		return 0
	}
	if val > float64(max) {
		return float64(max)
	}
	return val
}
