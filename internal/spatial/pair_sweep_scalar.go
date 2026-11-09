//go:build !(goexperiment.simd && amd64)

package spatial

// visitAVX2 sweeps nothing without the simd experiment on amd64: simd's kernel sweeps, if any.
func (s *pairSweep) visitAVX2([]sweepEntry, uint32, uint32, func(a, b int32)) {}
