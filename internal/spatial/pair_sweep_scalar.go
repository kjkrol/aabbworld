//go:build !(goexperiment.simd && amd64)

package spatial

// wide is off without the simd experiment on amd64: every cell is swept one entry at a time.
const wide = false

type sweepCols struct{}

func (s *pairSweep) visitWide(entries []sweepEntry, cx, cy uint32, fn func(a, b int32)) {}
