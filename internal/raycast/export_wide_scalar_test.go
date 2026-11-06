//go:build !(goexperiment.simd && (amd64 || arm64 || wasm))

package raycast

import "testing"

// SetWide skips t: without the simd experiment every slab and every ray's ground is worked out as
// it is cast.
func SetWide(t testing.TB, on bool) {
	t.Skip("walks lay a vector of angles at a time only with GOEXPERIMENT=simd")
}

// EachKernel skips t: without the simd experiment no wide kernel runs.
func EachKernel(t testing.TB, f func(kernel string)) {
	t.Skip("walks lay a vector of angles at a time only with GOEXPERIMENT=simd")
}
