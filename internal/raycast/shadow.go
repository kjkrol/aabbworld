package raycast

import (
	"math"

	"github.com/kjkrol/aabbworld/geom"
)

// arc is an angular interval, in radians, relative to a cone's direction.
type arc struct{ lo, hi float64 }

func (a arc) empty() bool { return a.lo > a.hi }

// wrapToPi folds an angle into (-π, π].
func wrapToPi(a float64) float64 {
	a = math.Mod(a+math.Pi, 2*math.Pi)
	if a <= 0 {
		a += 2 * math.Pi
	}
	return a - math.Pi
}

// subtendedArc is the arc box spans as seen from origin outside it, clipped to ±halfAngle.
func subtendedArc(origin vec, box geom.AABB, coneDir, halfAngle float64) arc {
	x1, y1 := float64(box.TopLeft.X)-origin.x, float64(box.TopLeft.Y)-origin.y
	x2, y2 := float64(box.BottomRight.X)-origin.x, float64(box.BottomRight.Y)-origin.y
	corners := [4][2]float64{{x1, y1}, {x2, y1}, {x2, y2}, {x1, y2}}

	lo, hi := corners[0], corners[0]
	for _, c := range corners[1:] {
		if lo[0]*c[1]-lo[1]*c[0] < 0 {
			lo = c
		}
		if hi[0]*c[1]-hi[1]*c[0] > 0 {
			hi = c
		}
	}

	start := math.Atan2(lo[1], lo[0])
	span := wrapToPi(math.Atan2(hi[1], hi[0]) - start)

	mid := wrapToPi(start + span/2 - coneDir)
	half := span / 2

	return arc{
		lo: math.Max(mid-half, -halfAngle),
		hi: math.Min(mid+half, halfAngle),
	}
}
