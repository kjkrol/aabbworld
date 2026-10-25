package raycast

import (
	"math"

	"github.com/kjkrol/aabbworld/geom"
)

// vec is a point or a direction as sight works with it: in float64, where the difference of two
// float32 corners is exact and distances along a ray do not drift.
type vec struct{ x, y float64 }

// vec32 is v as the boxes hold it, for what the cone's fields are handed.
func (v vec) vec32() geom.Vec { return geom.NewVec(float32(v.x), float32(v.y)) }

// round32 is v rounded to what vec32 hands over, so the boxes, the cover and the profile are met
// along the same ray.
func (v vec) round32() vec { return vec{float64(float32(v.x)), float64(float32(v.y))} }

// rect is a box as sight works with it, in float64 like vec: read once from the float32 corners,
// then met by every ray of the scan.
type rect struct{ x1, y1, x2, y2 float64 }

func rectOf(b geom.AABB) rect {
	return rect{float64(b.TopLeft.X), float64(b.TopLeft.Y), float64(b.BottomRight.X), float64(b.BottomRight.Y)}
}

// surrounds reports whether p lies inside box, its edges included.
func surrounds(box geom.AABB, p vec) bool {
	return p.x >= float64(box.TopLeft.X) && p.x <= float64(box.BottomRight.X) &&
		p.y >= float64(box.TopLeft.Y) && p.y <= float64(box.BottomRight.Y)
}

func centerOf(a geom.AABB) vec {
	return vec{
		(float64(a.TopLeft.X) + float64(a.BottomRight.X)) / 2,
		(float64(a.TopLeft.Y) + float64(a.BottomRight.Y)) / 2,
	}
}

// boxDistance is the gap between two boxes, zero when they touch or overlap.
func boxDistance(a, b geom.AABB) float64 {
	return math.Hypot(float64(a.AxisDistanceX(b)), float64(a.AxisDistanceY(b)))
}

// nearestImage shifts box to whichever of its wrapped copies lies closest to origin.
func nearestImage(origin vec, box geom.AABB, w, h float64) geom.AABB {
	c := centerOf(box)
	dx := shortestDelta(c.x-origin.x, w)
	dy := shortestDelta(c.y-origin.y, h)
	d := geom.NewVec(float32(origin.x+dx-c.x), float32(origin.y+dy-c.y))
	return geom.NewAABB(box.TopLeft.Add(d), box.BottomRight.Add(d))
}

func shortestDelta(d, size float64) float64 {
	if size <= 0 {
		return d
	}
	half := size / 2
	switch {
	case d > half:
		return d - size
	case d < -half:
		return d + size
	default:
		return d
	}
}

// searchAreas appends the index rectangles covering a cone of radius, split at each wrapping seam.
func searchAreas(origin vec, radius, w, h float64, wrapX, wrapY bool, dst []geom.AABB) []geom.AABB {
	xs, nx := axisSpans(origin.x-radius, origin.x+radius, w, wrapX)
	ys, ny := axisSpans(origin.y-radius, origin.y+radius, h, wrapY)
	for _, x := range xs[:nx] {
		for _, y := range ys[:ny] {
			dst = append(dst, clampRect(vec{x[0], y[0]}, vec{x[1], y[1]}, w, h))
		}
	}
	return dst
}

func axisSpans(lo, hi, size float64, wraps bool) (spans [2][2]float64, n int) {
	if !wraps {
		return [2][2]float64{{lo, hi}}, 1
	}
	return splitAxis(lo, hi, size)
}

// wrapSize is size on a wrapping axis and zero on any other, which shortestDelta leaves alone.
func wrapSize(size float64, wraps bool) float64 {
	if wraps {
		return size
	}
	return 0
}

// splitAxis wraps [lo, hi] into [0, size): the first n of spans, two where it crosses the seam.
func splitAxis(lo, hi, size float64) (spans [2][2]float64, n int) {
	if size <= 0 || hi-lo >= size {
		return [2][2]float64{{0, size}}, 1
	}
	span := hi - lo
	lo = math.Mod(lo, size)
	if lo < 0 {
		lo += size
	}
	hi = lo + span
	if hi <= size {
		return [2][2]float64{{lo, hi}}, 1
	}
	return [2][2]float64{{lo, size}, {0, hi - size}}, 2
}

func clampRect(lo, hi vec, w, h float64) geom.AABB {
	cl := func(v, max float64) float64 {
		switch {
		case v < 0:
			return 0
		case v > max:
			return max
		default:
			return v
		}
	}
	return geom.NewAABB(
		vec{cl(lo.x, w), cl(lo.y, h)}.vec32(),
		vec{cl(hi.x, w), cl(hi.y, h)}.vec32(),
	)
}
