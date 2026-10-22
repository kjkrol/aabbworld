package aabbworld

import (
	"fmt"
	"unsafe"

	"github.com/kjkrol/aabbworld/collide"
	"github.com/kjkrol/aabbworld/geom"
	icollide "github.com/kjkrol/aabbworld/internal/collide"
	iplane "github.com/kjkrol/aabbworld/internal/plane"
	"github.com/kjkrol/aabbworld/internal/spatial"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

// Space is a plane with edge rules and a grid over the items of its last Rebuild, read as they
// are now: a Tick that pushes them is seen by the next Query without another Rebuild.
type Space struct {
	Config
	grid    *spatial.Grid
	surface *iplane.Surface
	read    SpaceRead
}

// SpaceRead reads a Space: what lies where and what an observer sees, from as many goroutines at
// once as read it while nothing writes the Space; a Scan's View is one goroutine's.
type SpaceRead struct{ s *Space }

// Capability is what may be done with an entity, as a set of up to eight bits —
// a question names the bits it wants, and only entities sharing one are answered with.
type Capability uint8

const (
	// Plain is an entity that is nothing but geometry; the zero value.
	Plain Capability = 0
	// CanCollide is an entity that takes part in collisions — what a CollideEngine pairs up.
	CanCollide Capability = 1 << 1
	// Static is an entity nothing shifts; a contact pushes only the other side.
	Static Capability = 1 << 2
	// Sensor is an entity only ever detected; a contact with it separates nothing.
	Sensor Capability = 1 << 3
	// AnyCapability accepts every entity.
	AnyCapability Capability = 0xFF
)

// Edges is what happens to a box at the edges of the world, per axis; zero stops it whole.
type Edges uint8

const (
	// WrapX returns a box leaving by the left or right edge through the opposite one.
	WrapX Edges = 1 << iota
	// WrapY returns a box leaving by the top or bottom edge through the opposite one.
	WrapY
	// OpenX lets a box leave by the left or right edge; wholly outside, it is dropped from the index.
	OpenX
	// OpenY lets a box leave by the top or bottom edge; wholly outside, it is dropped from the index.
	OpenY
	// Torus wraps on both axes.
	Torus = WrapX | WrapY
)

// WrapsX reports whether the left and right edges wrap.
func (e Edges) WrapsX() bool { return e&WrapX != 0 }

// WrapsY reports whether the top and bottom edges wrap.
func (e Edges) WrapsY() bool { return e&WrapY != 0 }

// Config sizes a Space.
type Config struct {
	// Width and Height of the world, in world units.
	Width, Height uint32
	// Edges is what happens to a box at the edges of the world; the zero value stops it whole.
	Edges Edges
	// BucketSize is the side of one grid cell in world units, rounded up to a power of two.
	BucketSize uint32
}

// Item is one box a Rebuild hands the Space: whose it is, where it lies, what may be done with it.
type Item struct {
	ID   uid.UID64
	Box  plane.AABB
	Caps Capability
}

// Run is a column of boxes in memory the caller owns: record i starts i·Stride bytes past Base,
// with its box at BoxOff and its Capability at CapsOff, and IDs[i] names it.
type Run struct {
	IDs     []uid.UID64
	Base    unsafe.Pointer
	Stride  uintptr
	BoxOff  uintptr
	CapsOff uintptr
}

// RunOf is the run over rows, named by ids, with the box and the capability that box and caps
// point at in a row.
func RunOf[T any](
	ids []uid.UID64, rows []T, box func(*T) *plane.AABB, caps func(*T) *Capability,
) Run {
	if len(ids) != len(rows) {
		panic("aabbworld: RunOf over rows and ids of different lengths")
	}
	if len(rows) == 0 {
		return Run{}
	}
	row := &rows[0]
	stride, at := unsafe.Sizeof(*row), uintptr(unsafe.Pointer(row))
	boxOff := uintptr(unsafe.Pointer(box(row))) - at
	capsOff := uintptr(unsafe.Pointer(caps(row))) - at
	if boxOff >= stride || stride-boxOff < unsafe.Sizeof(plane.AABB{}) || capsOff >= stride {
		panic("aabbworld: RunOf with a box or a capability outside the row")
	}
	return Run{
		IDs: ids, Base: unsafe.Pointer(row), Stride: stride, BoxOff: boxOff, CapsOff: capsOff,
	}
}

// NewSpace builds a Space, fitting the world into the nearest power-of-two grid.
func NewSpace(cfg Config) (*Space, error) {
	if cfg.Width == 0 || cfg.Height == 0 || cfg.BucketSize == 0 {
		return nil, fmt.Errorf("invalid dimensions")
	}
	if !iplane.Edges(cfg.Edges).Valid() {
		return nil, fmt.Errorf("an axis cannot both wrap and be open")
	}
	surface := iplane.NewSurface(float64(cfg.Width), float64(cfg.Height), iplane.Edges(cfg.Edges))
	grid := spatial.NewGrid(surface, spatial.ResolutionFrom(max(cfg.Width, cfg.Height)), spatial.ResolutionFrom(cfg.BucketSize))
	s := &Space{Config: cfg, grid: grid, surface: surface}
	s.read.s = s
	return s, nil
}

// Read reads s.
func (s *Space) Read() *SpaceRead { return &s.read }

// Query is the Space's Query.
func (r *SpaceRead) Query(box geom.AABB, want Capability, fn func(id uid.UID64)) int {
	return r.s.Query(box, want, fn)
}

// Scan is the Space's Scan.
func (r *SpaceRead) Scan(observer uid.UID64, cone Cone, v *View) bool {
	return r.s.Scan(observer, cone, v)
}

// Ready indexes the Space as the first Query or Scan after a Rebuild would: called before
// readers fan out, none of them waits for another to index it.
func (r *SpaceRead) Ready() { r.s.grid.Ready() }

// Bounds is the Space's Bounds.
func (r *SpaceRead) Bounds() (width, height uint32, edges Edges) { return r.s.Bounds() }

// WrapAABB is the Space's WrapAABB.
func (r *SpaceRead) WrapAABB(box geom.AABB) plane.AABB { return r.s.WrapAABB(box) }

// Rebuild hands the Space items, which must stay put until the next Rebuild. Indexing waits for
// the first Query or Scan that needs it, so several Rebuilds in a row cost one indexing.
func (s *Space) Rebuild(items []Item) { s.RebuildRuns(nil, items) }

// RebuildRuns is Rebuild over the boxes of runs and then of items, read where they lie: Query and
// Scan see them as they are, and a Tick pushes them in place; a box may move between Rebuilds, a
// Capability may not. Until the last Query, Scan or Tick before the next Rebuild, the memory under
// both must hold the same records in the same places; a caller that moves records, as an ECS does
// when entities migrate (in goke, once any table's ChunkSnapshot.TableVer changed), gathers its
// runs again and rebuilds.
func (s *Space) RebuildRuns(runs []Run, items []Item) {
	s.grid.RebuildRuns(
		unsafe.Slice((*spatial.Run)(unsafe.Pointer(unsafe.SliceData(runs))), len(runs)),
		unsafe.Slice((*spatial.Item)(unsafe.Pointer(unsafe.SliceData(items))), len(items)))
}

// Place puts a fresh box under the edge rules; false if it lies wholly past an open edge.
func (s *Space) Place(box *plane.AABB) bool {
	s.surface.Translate(box, geom.Vec{})
	return !s.surface.Left(box)
}

// Move moves box by delta under the edge rules; false once it has left by an open edge.
func (s *Space) Move(box *plane.AABB, delta geom.Vec) bool {
	s.surface.Translate(box, delta)
	return !s.surface.Left(box)
}

// MoveRun is Move of the box of record i of run by deltas[i], for every i whose delta is not
// zero. It returns left with the ID of every box it moved that has left by an open edge appended.
func (s *Space) MoveRun(run Run, deltas []geom.Vec, left []uid.UID64) []uid.UID64 {
	if len(deltas) != len(run.IDs) {
		panic("aabbworld: MoveRun over deltas and ids of different lengths")
	}
	if len(deltas) == 0 {
		return left
	}
	boxes := unsafe.Add(run.Base, run.BoxOff)
	s.surface.TranslateRun(boxes, run.Stride, deltas)
	if s.Edges&(OpenX|OpenY) == 0 {
		return left
	}
	for i, d := range deltas {
		box := (*plane.AABB)(unsafe.Add(boxes, uintptr(i)*run.Stride))
		if (d.X != 0 || d.Y != 0) && s.surface.Left(box) {
			left = append(left, run.IDs[i])
		}
	}
	return left
}

// MoveTo puts box with its top-left at to, under the edge rules; false once it has left that way.
func (s *Space) MoveTo(box *plane.AABB, to geom.Vec) bool {
	return s.Move(box, to.Sub(box.TopLeft))
}

// WrapAABB folds any rectangle into the space: wrapped pieces on a wrapping axis, else clipped.
func (s *Space) WrapAABB(box geom.AABB) plane.AABB { return s.surface.WrapAABB(box) }

// Bounds is the world's size and what happens at its edges.
func (s *Space) Bounds() (width, height uint32, edges Edges) {
	return s.Config.Width, s.Config.Height, s.Config.Edges
}

// Query calls fn for every indexed piece in box sharing want, seams included, and returns how many.
func (s *Space) Query(box geom.AABB, want Capability, fn func(id uid.UID64)) int {
	area := s.surface.WrapAABB(box)
	caps := spatial.Capability(want)
	found := s.grid.Query(area.AABB, caps, fn)
	area.VisitFragments(func(_ plane.FragPosition, image geom.AABB) bool {
		found += s.grid.Query(image, caps, fn)
		return true
	})
	return found
}

// CollideEngine builds an engine over the Space's CanCollide boxes, reporting to handler.
func (s *Space) CollideEngine(handler collide.Handler, cfg collide.Config) collide.Engine {
	return icollide.New(s.grid, s.surface, handler, cfg.Reach, cfg.Iterations, cfg.Field)
}
