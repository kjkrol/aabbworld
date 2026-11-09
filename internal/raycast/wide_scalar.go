//go:build !(goexperiment.simd && (amd64 || arm64 || wasm))

package raycast

// Without the simd experiment every slab and every ray's ground is worked out as it is cast.
const (
	avx2 = false
	wide = false
)

func (s *slabs) fillWide(x1, y1, x2, y2 float64, r run) {}

func invertWide(v []float64) {}
