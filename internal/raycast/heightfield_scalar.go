//go:build !(goexperiment.simd && amd64)

package raycast

// walkWide walks nothing without the simd experiment on amd64: every ray is walked as it is cast.
func (b *profileBlock) walkWide(l *lattice, origin vec, dirs []vec, length float64) bool {
	return false
}
