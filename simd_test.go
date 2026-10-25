//go:build !(goexperiment.simd && amd64)

package aabbworld_test

import (
	"testing"

	"github.com/kjkrol/aabbworld"
)

func TestSIMD_OffWithoutTheExperimentOnAmd64(t *testing.T) {
	if aabbworld.SIMD() {
		t.Error("SIMD() = true in a build without GOEXPERIMENT=simd on amd64")
	}
}
