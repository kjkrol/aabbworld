//go:build goexperiment.simd && amd64

package simdx

import (
	"simd"
	"simd/archsimd"
)

// Bits32 is m as a bitmask, lane i the bit 1<<i: VMOVMSKPS or KMOVW.
func Bits32(m simd.Mask32s) uint64 {
	switch a := m.ToArch().(type) {
	case archsimd.Mask32x4:
		return uint64(a.ToBits())
	case archsimd.Mask32x8:
		return uint64(a.ToBits())
	case archsimd.Mask32x16:
		return uint64(a.ToBits())
	}
	return bits32Stored(m)
}

// Bits64 is Bits32 for a mask of 64-bit lanes.
func Bits64(m simd.Mask64s) uint64 {
	switch a := m.ToArch().(type) {
	case archsimd.Mask64x2:
		return uint64(a.ToBits())
	case archsimd.Mask64x4:
		return uint64(a.ToBits())
	case archsimd.Mask64x8:
		return uint64(a.ToBits())
	}
	return bits64Stored(m)
}
