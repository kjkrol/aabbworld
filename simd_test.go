//go:build !(goexperiment.simd && (amd64 || arm64 || wasm))

package aabbworld_test

import (
	"testing"

	"github.com/kjkrol/aabbworld"
)

func TestSIMD_OffWithoutTheExperiment(t *testing.T) {
	if aabbworld.SIMD() {
		t.Error("SIMD() = true in a build without GOEXPERIMENT=simd")
	}
}
