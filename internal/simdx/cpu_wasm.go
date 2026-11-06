//go:build goexperiment.simd && wasm

package simdx

// ArchFirst is false: no archsimd kernel is written for this architecture.
func ArchFirst() bool { return false }

// Settle does nothing: no vector state slows the code after it here.
func Settle() {}
