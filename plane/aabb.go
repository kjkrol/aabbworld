package plane

import (
	"github.com/kjkrol/aabbworld/geom"
)

// FragPosition names a wrapped piece of a box by the edge of the parent it lies past.
type FragPosition int

const (
	FRAG_MAIN FragPosition = iota
	// FRAG_RIGHT is the fragment that spans the parent's right edge.
	FRAG_RIGHT
	// FRAG_BOTTOM is the fragment along the parent's bottom edge.
	FRAG_BOTTOM
	// FRAG_BOTTOM_RIGHT is the fragment covering the parent's bottom-right quadrant.
	FRAG_BOTTOM_RIGHT
)

// AABB is a box kept canonical within a Space: its corners, its size, and its Overhang —
// how far it runs past the world's far edges, from which its wrapped pieces follow.
type AABB struct {
	geom.AABB
	Size     geom.Vec
	Overhang geom.Vec
}

// NewAABB builds a AABB at pos with the given size, not yet wrapped by any Space.
func NewAABB(pos geom.Vec, width, height float32) AABB {
	return AABB{
		AABB: geom.AABB{
			TopLeft:     pos,
			BottomRight: geom.NewVec(pos.X+width, pos.Y+height),
		},
		Size: geom.NewVec(width, height),
	}
}

// --------------------------------------------------------------------------

// String formats the aabb using its top-left and bottom-right corners.
func (ab AABB) String() string {
	return ab.AABB.String()
}

// Equals reports whether ab and other share the same corners.
func (ab AABB) Equals(other AABB) bool {
	return ab.AABB.Equals(other.AABB)
}

// visitImagePairs walks every image of ab against every image of other until fn returns false.
func (ab AABB) visitImagePairs(other AABB, fn func(a, b geom.AABB) bool) {
	if !fn(ab.AABB, other.AABB) {
		return
	}

	abFrags, otherFrags := ab.hasFragments(), other.hasFragments()
	if !abFrags && !otherFrags {
		return
	}

	going := true
	if otherFrags {
		other.VisitFragments(func(_ FragPosition, b geom.AABB) bool {
			going = fn(ab.AABB, b)
			return going
		})
		if !going {
			return
		}
	}
	if !abFrags {
		return
	}

	ab.VisitFragments(func(_ FragPosition, a geom.AABB) bool {
		if !fn(a, other.AABB) {
			going = false
			return false
		}
		if otherFrags {
			other.VisitFragments(func(_ FragPosition, b geom.AABB) bool {
				going = fn(a, b)
				return going
			})
		}
		return going
	})
}

// hasFragments reports whether the box reaches past a world edge at all.
func (ab AABB) hasFragments() bool {
	var none float32
	return ab.Overhang.X > none || ab.Overhang.Y > none
}

// FragVisitor is handed each wrapped piece of a box and says whether to go on to the next.
type FragVisitor func(pos FragPosition, box geom.AABB) bool

// VisitFragments calls fn for each piece the box wraps into, until fn returns false.
func (ab *AABB) VisitFragments(fn FragVisitor) {
	var none float32
	dx, dy := ab.Overhang.X, ab.Overhang.Y

	if dx > none {
		box := geom.NewAABB(geom.NewVec(none, ab.TopLeft.Y), geom.NewVec(dx, ab.BottomRight.Y))
		if !fn(FRAG_RIGHT, box) {
			return
		}
	}
	if dy > none {
		box := geom.NewAABB(geom.NewVec(ab.TopLeft.X, none), geom.NewVec(ab.BottomRight.X, dy))
		if !fn(FRAG_BOTTOM, box) {
			return
		}
	}
	if dx > none && dy > none {
		fn(FRAG_BOTTOM_RIGHT, geom.NewAABB(geom.NewVec(none, none), geom.NewVec(dx, dy)))
	}
}
