//go:build !(goexperiment.simd && amd64)

package raycast

import (
	"testing"

	"github.com/kjkrol/aabbworld/geom"
)

// SetWide skips t: without the simd experiment on amd64 every slab and every ray's ground is
// worked out as it is cast.
func SetWide(t testing.TB, on bool) {
	t.Skip("walks lay four angles at a time only with GOEXPERIMENT=simd on amd64")
}

// SideBySide skips t: without the simd experiment on amd64 every ray is walked as it is cast.
func SideBySide(t testing.TB, f *HeightField, bend float32, origin geom.Vec, dirs []geom.Vec,
	length float32) (d, alt [][]float64) {
	t.Skip("rays are walked side by side only with GOEXPERIMENT=simd on amd64")
	return nil, nil
}
