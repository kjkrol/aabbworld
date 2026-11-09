//go:build !(goexperiment.simd && amd64)

package raycast

// fillAVX2 lays nothing without the simd experiment on amd64: simd's kernel lays, if any.
func (s *slabs) fillAVX2(x1, y1, x2, y2 float64, r run) {}

// invertAVX2 inverts nothing without the simd experiment on amd64: simd's kernel inverts, if any.
func invertAVX2(v []float64) {}
