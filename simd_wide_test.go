//go:build goexperiment.simd && (amd64 || arm64 || wasm)

package aabbworld_test

import (
	"simd"
	"testing"

	"github.com/kjkrol/aabbworld"
)

func TestSIMD_OnWhereVectorsRunInHardware(t *testing.T) {
	if got, want := aabbworld.SIMD(), !simd.Emulated(); got != want {
		t.Errorf("SIMD() = %v where simd.Emulated() is %v", got, !want)
	}
}
