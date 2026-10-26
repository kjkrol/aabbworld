package aabbworld

import "github.com/kjkrol/aabbworld/internal/spatial"

// SIMD reports whether the collision engine's pair sweep tests a crowded cell eight boxes at a
// time, and sight its cone's boxes and rays several at a time: built with GOEXPERIMENT=simd on
// amd64 and run on a CPU with AVX2.
func SIMD() bool { return spatial.Wide() }
