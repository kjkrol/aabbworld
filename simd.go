package aabbworld

import "github.com/kjkrol/aabbworld/internal/spatial"

// SIMD reports whether the collision engine's pair sweep tests a crowded cell a vector of boxes at
// a time, and sight its cone's boxes and rays several at a time: built with
// GOEXPERIMENT=simd on amd64, arm64 or wasm, and run where the simd package has hardware vectors
// (on amd64, AVX).
func SIMD() bool { return spatial.Wide() }
