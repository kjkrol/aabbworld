//go:build !(goexperiment.simd && (amd64 || arm64 || wasm))

package spatial

import "github.com/kjkrol/uid"

// Without the simd experiment every cell is swept and every cone walked one piece at a time.
const wide = false

func (s *pairSweep) visitWide([]sweepEntry, uint32, uint32, func(a, b int32)) {}

func (g *Grid) walkConeWide(*coneQuery, Capability, func(uid.UID64)) int { return 0 }

type sweepCols struct{}

type coneCols struct{}

func (c *coneCols) fill([]gridCell) {}
