package geom

import "fmt"

// AABB is a minimal axis-aligned rectangle defined by its top-left and bottom-right corners.
type AABB struct {
	TopLeft     Vec
	BottomRight Vec
}

// NewAABB constructs a axis-aligned bounding box from explicit corner vectors.
func NewAABB(topLeft, bottomRight Vec) AABB {
	return AABB{
		TopLeft:     topLeft,
		BottomRight: bottomRight,
	}
}

// NewAABBAt builds a AABB starting at pos with the provided width and height.
func NewAABBAt(pos Vec, width, height float32) AABB {
	bottomRight := NewVec(pos.X+width, pos.Y+height)
	return NewAABB(pos, bottomRight)
}

// Equals reports whether ab and other share the same corners.
func (ab AABB) Equals(other AABB) bool {
	return ab.TopLeft == other.TopLeft && ab.BottomRight == other.BottomRight
}

// String formats the box using its top-left and bottom-right corners.
func (ab AABB) String() string {
	return fmt.Sprintf("{%v %v}", ab.TopLeft, ab.BottomRight)
}

// Contains reports whether other lies entirely within axis-aligned bounding box.
func (ab AABB) Contains(other AABB) bool {
	return ab.TopLeft.X <= other.TopLeft.X &&
		ab.TopLeft.Y <= other.TopLeft.Y &&
		ab.BottomRight.X >= other.BottomRight.X &&
		ab.BottomRight.Y >= other.BottomRight.Y
}

// Intersects reports whether the boxes overlap or touch.
func (ab AABB) Intersects(other AABB) bool {
	return ab.TopLeft.X <= other.BottomRight.X &&
		ab.BottomRight.X >= other.TopLeft.X &&
		ab.TopLeft.Y <= other.BottomRight.Y &&
		ab.BottomRight.Y >= other.TopLeft.Y
}

// AxisDistanceTo returns the gap between tow given AABBs on the axis selected by axisValue.
func (ab AABB) AxisDistanceX(other AABB) float32 {
	return axisDistance1D(ab.TopLeft.X, ab.BottomRight.X, other.TopLeft.X, other.BottomRight.X)
}

func (ab AABB) AxisDistanceY(other AABB) float32 {
	return axisDistance1D(ab.TopLeft.Y, ab.BottomRight.Y, other.TopLeft.Y, other.BottomRight.Y)
}

func axisDistance1D(aMin, aMax, bMin, bMax float32) float32 {

	if aMax < bMin {
		return bMin - aMax
	}

	if bMax < aMin {
		return aMin - bMax
	}

	return 0
}

// Penetration is the shortest translation that moves ab clear of other, or zero when apart.
func (ab AABB) Penetration(other AABB) Vec {
	leftX := max(ab.TopLeft.X, other.TopLeft.X)
	rightX := min(ab.BottomRight.X, other.BottomRight.X)
	if rightX-leftX <= 0 {
		return Vec{}
	}

	topY := max(ab.TopLeft.Y, other.TopLeft.Y)
	bottomY := min(ab.BottomRight.Y, other.BottomRight.Y)
	if bottomY-topY <= 0 {
		return Vec{}
	}

	pushRight := other.BottomRight.X - ab.TopLeft.X
	pushLeft := ab.BottomRight.X - other.TopLeft.X
	pushDown := other.BottomRight.Y - ab.TopLeft.Y
	pushUp := ab.BottomRight.Y - other.TopLeft.Y

	minPush := pushRight
	mtv := Vec{X: pushRight, Y: 0}

	if pushLeft < minPush {
		minPush = pushLeft
		mtv = Vec{X: -pushLeft, Y: 0}
	}
	if pushDown < minPush {
		minPush = pushDown
		mtv = Vec{X: 0, Y: pushDown}
	}
	if pushUp < minPush {
		mtv = Vec{X: 0, Y: -pushUp}
	}

	return mtv
}
