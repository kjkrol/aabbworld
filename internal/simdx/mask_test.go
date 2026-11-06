//go:build goexperiment.simd && (amd64 || arm64 || wasm)

package simdx

import (
	"math/rand/v2"
	"simd"
	"testing"
)

func TestBits_OneBitALane(t *testing.T) {
	n32 := simd.BroadcastFloat32s(0).Len()
	for want := uint64(0); want < 1<<n32; want = want*3 + 1 {
		var v [Pad32 + 1]float32
		for i := range n32 {
			if want&(1<<i) != 0 {
				v[i] = 1
			}
		}
		if got := Bits32(simd.LoadFloat32s(v[:]).Greater(simd.BroadcastFloat32s(0))); got != want {
			t.Errorf("Bits32 of %v = %b, want %b", v[:n32], got, want)
		}
	}
	n64 := simd.BroadcastFloat64s(0).Len()
	for want := uint64(0); want < 1<<n64; want++ {
		var v [Pad64 + 1]float64
		for i := range n64 {
			if want&(1<<i) != 0 {
				v[i] = 1
			}
		}
		if got := Bits64(simd.LoadFloat64s(v[:]).Greater(simd.BroadcastFloat64s(0))); got != want {
			t.Errorf("Bits64 of %v = %b, want %b", v[:n64], got, want)
		}
	}
}

// The way through memory, the only one on wasm, sets the bits the type switch sets.
func TestBits_StoredAgreesWithTheMask(t *testing.T) {
	r := rand.New(rand.NewPCG(71, 73))
	for range 1000 {
		var v32 [Pad32 + 1]float32
		var v64 [Pad64 + 1]float64
		for i := range v32 {
			v32[i] = float32(r.IntN(3) - 1)
		}
		for i := range v64 {
			v64[i] = float64(r.IntN(3) - 1)
		}
		if got, want := bothBits32(v32[:]); got != want {
			t.Fatalf("bits32Stored of %v = %b, Bits32 %b", v32, got, want)
		}
		if got, want := bothBits64(v64[:]); got != want {
			t.Fatalf("bits64Stored of %v = %b, Bits64 %b", v64, got, want)
		}
	}
}

// bothBits32 is bits32Stored and Bits32 of the lanes of v above 0.
func bothBits32(v []float32) (stored, switched uint64) {
	m := simd.LoadFloat32s(v).Greater(simd.BroadcastFloat32s(0))
	return bits32Stored(m), Bits32(m)
}

// bothBits64 is bothBits32 for float64.
func bothBits64(v []float64) (stored, switched uint64) {
	m := simd.LoadFloat64s(v).Greater(simd.BroadcastFloat64s(0))
	return bits64Stored(m), Bits64(m)
}
