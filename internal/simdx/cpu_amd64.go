//go:build goexperiment.simd && amd64

package simdx

import (
	"simd"
	"simd/archsimd"
)

// ArchFirst says the kernels written for AVX2 run here rather than simd's: on amd64 they are as
// fast or faster at every width simd gives, and twice as wide where simd holds a CPU without
// VPCLMULQDQ to 128 bits. It is false where simd is emulated, GODEBUG=simd=0 included, so that no
// vector code runs.
func ArchFirst() bool { return archsimd.X86.AVX2() && !simd.Emulated() }

// Settle clears the upper halves of the vector registers, so that SSE code called next, as a
// callback may be, runs at full speed.
func Settle() { archsimd.ClearAVXUpperBits() }
