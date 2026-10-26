//go:build !(goexperiment.simd && amd64)

package spatial

import "github.com/kjkrol/uid"

type coneCols struct{}

func (c *coneCols) fill([]gridCell) {}

func (g *Grid) walkConeWide(*coneQuery, Capability, func(uid.UID64)) int { return 0 }
