//go:build goexperiment.simd && (amd64 || arm64 || wasm)

package simdx

import "simd"

// On says the simd package runs in hardware rather than emulating it.
func On() bool { return !simd.Emulated() }
