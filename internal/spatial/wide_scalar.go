//go:build !(goexperiment.simd && (amd64 || arm64 || wasm))

package spatial

import (
	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/uid"
)

// Without the simd experiment every cell is swept, every cone walked and every query tested one
// piece at a time.
const (
	wide          = false
	wideQuery     = false
	queryWideFrom = 0
)

func lanes32() int { return 0 }

func (s *pairSweep) visitWide([]sweepEntry, uint32, uint32, func(a, b int32)) {}

func (g *Grid) walkConeWide(*coneQuery, Capability, func(uid.UID64)) int { return 0 }

func (g *Grid) queryWide(geom.AABB, uint32, uint32, uint32, uint32, Capability,
	func(uid.UID64)) int {
	return 0
}

type sweepCols struct{}

type cellCols struct{}

func (c *cellCols) fill([]gridCell) {}
