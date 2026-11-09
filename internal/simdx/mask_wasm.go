//go:build goexperiment.simd && wasm

package simdx

import "simd"

// Bits32 is m as a bitmask, lane i the bit 1<<i, by way of memory.
func Bits32(m simd.Mask32s) uint64 { return bits32Stored(m) }

// Bits64 is Bits32 for a mask of 64-bit lanes.
func Bits64(m simd.Mask64s) uint64 { return bits64Stored(m) }
