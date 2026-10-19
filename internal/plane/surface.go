package plane

import (
	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/plane"
)

// Edges is what happens to a box at the edges of the world, per axis; zero stops it whole.
type Edges uint8

const (
	WrapX Edges = 1 << iota
	WrapY
	OpenX
	OpenY
	Torus = WrapX | WrapY
)

// Valid reports whether no axis is asked both to wrap and to be open.
func (e Edges) Valid() bool {
	return e&(WrapX|OpenX) != WrapX|OpenX && e&(WrapY|OpenY) != WrapY|OpenY
}

// Surface is a plane of a given size that keeps boxes canonical under its edge rules.
type Surface struct {
	size  geom.Vec
	edges Edges
}

// NewSurface builds a plane of the given size and edge rules.
func NewSurface(sizeX, sizeY float64, edges Edges) *Surface {
	return &Surface{size: geom.NewVec(sizeX, sizeY), edges: edges}
}

// NewEuclidean2D builds a plane closed on every side.
func NewEuclidean2D(sizeX, sizeY float64) *Surface { return NewSurface(sizeX, sizeY, 0) }

// NewToroidal2D builds a plane that wraps on both axes.
func NewToroidal2D(sizeX, sizeY float64) *Surface { return NewSurface(sizeX, sizeY, Torus) }

// WrapAABB folds a rectangle into the plane: wrapped on a wrapping axis, clipped on any other.
func (s *Surface) WrapAABB(aabb geom.AABB) plane.AABB {
	width := aabb.BottomRight.X - aabb.TopLeft.X
	height := aabb.BottomRight.Y - aabb.TopLeft.Y
	wrapped := plane.NewAABB(aabb.TopLeft, width, height)
	s.normalizeAABB(&wrapped)
	return wrapped
}

// Expand grows aabb by margin on every side and folds it like WrapAABB.
func (s *Surface) Expand(aabb *plane.AABB, margin float64) {
	aabb.TopLeft.AddMutable(geom.NewVec(-margin, -margin))
	aabb.Size.AddMutable(geom.NewVec(2*margin, 2*margin))
	s.normalizeAABB(aabb)
}

// Translate moves aabb by delta: wrapped, stopped whole, or let go, by the rule of each axis.
func (s *Surface) Translate(aabb *plane.AABB, delta geom.Vec) {
	if lo := aabb.TopLeft.X + delta.X; inside(lo, aabb.Size.X, s.size.X) {
		aabb.TopLeft.X, aabb.BottomRight.X, aabb.Overhang.X = lo, lo+aabb.Size.X, 0
	} else {
		aabb.TopLeft.X, aabb.BottomRight.X, aabb.Overhang.X =
			move(lo, aabb.Size.X, s.size.X, s.edges&WrapX != 0, s.edges&OpenX != 0)
	}
	if lo := aabb.TopLeft.Y + delta.Y; inside(lo, aabb.Size.Y, s.size.Y) {
		aabb.TopLeft.Y, aabb.BottomRight.Y, aabb.Overhang.Y = lo, lo+aabb.Size.Y, 0
	} else {
		aabb.TopLeft.Y, aabb.BottomRight.Y, aabb.Overhang.Y =
			move(lo, aabb.Size.Y, s.size.Y, s.edges&WrapY != 0, s.edges&OpenY != 0)
	}
}

// inside reports whether a box of length from lo lies within the world: every edge rule leaves it
func inside(lo, length, world float64) bool {
	return lo >= 0 && lo < world && lo+length <= world
}

// Left reports whether aabb lies wholly past an open edge.
func (s *Surface) Left(aabb *plane.AABB) bool {
	if s.edges&OpenX != 0 && (aabb.BottomRight.X <= 0 || aabb.TopLeft.X >= s.size.X) {
		return true
	}
	return s.edges&OpenY != 0 && (aabb.BottomRight.Y <= 0 || aabb.TopLeft.Y >= s.size.Y)
}

func move(lo, length, world float64, wraps, open bool) (newLo, hi, overhang float64) {
	switch {
	case wraps:
		return fold(lo, length, world)
	case open:
		return lo, lo + length, 0
	default:
		lo = clamp(lo, max(world-length, 0))
		return lo, lo + length, 0
	}
}

func fold(lo, length, world float64) (newLo, hi, overhang float64) {
	lo = wrap(lo, world)
	hi = lo + length
	if hi > world {
		return lo, world, hi - world
	}
	return lo, hi, 0
}

func clip(lo, length, world float64) (newLo, hi, overhang float64) {
	return clamp(lo, world), clamp(lo+length, world), 0
}

func (s *Surface) normalizeVec(vec geom.Vec) geom.Vec {
	if s.edges&WrapX != 0 {
		vec.X = wrap(vec.X, s.size.X)
	} else {
		vec.X = clamp(vec.X, s.size.X)
	}
	if s.edges&WrapY != 0 {
		vec.Y = wrap(vec.Y, s.size.Y)
	} else {
		vec.Y = clamp(vec.Y, s.size.Y)
	}
	return vec
}

func (s *Surface) normalizeAABB(aabb *plane.AABB) {
	if s.edges&WrapX != 0 {
		aabb.TopLeft.X, aabb.BottomRight.X, aabb.Overhang.X = fold(aabb.TopLeft.X, aabb.Size.X, s.size.X)
	} else {
		aabb.TopLeft.X, aabb.BottomRight.X, aabb.Overhang.X = clip(aabb.TopLeft.X, aabb.Size.X, s.size.X)
	}
	if s.edges&WrapY != 0 {
		aabb.TopLeft.Y, aabb.BottomRight.Y, aabb.Overhang.Y = fold(aabb.TopLeft.Y, aabb.Size.Y, s.size.Y)
	} else {
		aabb.TopLeft.Y, aabb.BottomRight.Y, aabb.Overhang.Y = clip(aabb.TopLeft.Y, aabb.Size.Y, s.size.Y)
	}
}
