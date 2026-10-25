package plane

import (
	"math"

	"github.com/kjkrol/aabbworld/geom"
)

// Clamp bounds v to the inclusive box [0,size].
func Clamp(v, size geom.Vec) geom.Vec {
	return geom.Vec{X: clamp(v.X, size.X), Y: clamp(v.Y, size.Y)}
}

// Wrap folds v back into [0,size) on each axis.
func Wrap(v, size geom.Vec) geom.Vec {
	return geom.Vec{X: wrap(v.X, size.X), Y: wrap(v.Y, size.Y)}
}

func clamp(val, max float32) float32 {
	if val > max {
		return max
	}
	if val < 0 {
		return 0
	}
	return val
}

// wrap folds val into [0,max).
func wrap(val, max float32) float32 {
	if max == 0 {
		return val
	}
	if max < 0 {
		max = -max
	}

	if val >= 0 && val < max {
		return val
	}
	var r float32
	switch {
	case val >= max && val < max+max:
		r = val - max
	case val < 0 && val >= -max:
		r = val + max
	default:
		r = float32(math.Mod(float64(val), float64(max)))
		if r < 0 {
			r += max
		}
	}
	// A value a hair below zero rounds up to max once max is added to it.
	if r >= max {
		return 0
	}
	return r
}
