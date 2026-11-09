// Package simdx is what the wide code needs beside the portable simd package: whether vectors run
// in hardware and which kernels run on them, how far past its data a slice loaded a vector at a
// time must run, and a mask as a bitmask, one bit a lane, which simd has no method for.
//
// A kernel over simd's vectors is a function, not a method: the compiler's rewrite of simd code
// for each vector width fails on methods.
package simdx
