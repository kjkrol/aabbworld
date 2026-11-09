//go:build goexperiment.simd && (amd64 || arm64 || wasm)

package simdx

import "simd"

// bits32Stored is Bits32 by way of memory, for a vector the type switch did not know.
func bits32Stored(m simd.Mask32s) uint64 {
	v := m.ToInt32s()
	var at [Pad32 + 1]int32
	v.Store(at[:])
	var b uint64
	for i := range v.Len() {
		if at[i] != 0 {
			b |= 1 << i
		}
	}
	return b
}

// bits64Stored is Bits64 by way of memory, for a vector the type switch did not know.
func bits64Stored(m simd.Mask64s) uint64 {
	v := m.ToInt64s()
	var at [Pad64 + 1]int64
	v.Store(at[:])
	var b uint64
	for i := range v.Len() {
		if at[i] != 0 {
			b |= 1 << i
		}
	}
	return b
}
