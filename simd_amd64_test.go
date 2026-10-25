//go:build goexperiment.simd && amd64

package aabbworld_test

import (
	"simd/archsimd"
	"testing"

	"github.com/kjkrol/aabbworld"
)

func TestSIMD_OnWhereTheCPUHasAVX2(t *testing.T) {
	if got, want := aabbworld.SIMD(), archsimd.X86.AVX2(); got != want {
		t.Errorf("SIMD() = %v on a CPU whose AVX2 is %v", got, want)
	}
}
