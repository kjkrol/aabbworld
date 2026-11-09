//go:build !(goexperiment.simd && amd64)

package spatial

import "github.com/kjkrol/uid"

// walkConeAVX2 walks nothing without the simd experiment on amd64: simd's kernel walks, if any.
func (g *Grid) walkConeAVX2(*coneQuery, Capability, func(uid.UID64)) int { return 0 }
