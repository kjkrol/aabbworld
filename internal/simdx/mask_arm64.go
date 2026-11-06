//go:build goexperiment.simd && arm64

package simdx

import (
	"simd"
	"simd/archsimd"
)

// Bits32 is m as a bitmask, lane i the bit 1<<i. Neon has no move-mask: each lane, all ones where
// set, keeps its own bit of the weights, and ADDV sums them.
func Bits32(m simd.Mask32s) uint64 {
	if a, ok := m.ToInt32s().ToArch().(archsimd.Int32x4); ok {
		w := archsimd.LoadInt32x4Array(&[4]int32{1, 2, 4, 8})
		return uint64(a.And(w).ReduceSum())
	}
	return bits32Stored(m)
}

// Bits64 is Bits32 for a mask of 64-bit lanes.
func Bits64(m simd.Mask64s) uint64 {
	if a, ok := m.ToInt64s().ToArch().(archsimd.Int64x2); ok {
		return uint64(a.GetElem(0)&1 | a.GetElem(1)&2)
	}
	return bits64Stored(m)
}
