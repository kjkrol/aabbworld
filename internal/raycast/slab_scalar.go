//go:build !(goexperiment.simd && amd64)

package raycast

// wide is off without the simd experiment on amd64: every slab is worked out as it is cast.
const wide = false

func (s *slabs) fillWide(x1, y1, x2, y2 float64, r run) {}

func invertWide(v []float64) {}
