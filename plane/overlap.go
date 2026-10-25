package plane

import "github.com/kjkrol/aabbworld/geom"

// Overlap is where two wrapped boxes meet: which image of each was involved,
// and how deeply they interpenetrate.
type Overlap struct {
	BoxA, BoxB  geom.AABB
	Penetration geom.Vec
}

// DeepestOverlapWith is the deepest overlap of ab and other, wrapped images included, if any.
func (ab *AABB) DeepestOverlapWith(other *AABB) (Overlap, bool) {
	if !ab.hasFragments() && !other.hasFragments() {
		pen := ab.AABB.Penetration(other.AABB)
		if pen.X == 0 && pen.Y == 0 {
			return Overlap{}, false
		}
		return Overlap{BoxA: ab.AABB, BoxB: other.AABB, Penetration: pen}, true
	}

	var best Overlap
	deepest := float32(0)
	ab.visitImagePairs(*other, func(a, b geom.AABB) bool {
		pen := a.Penetration(b)
		if pen.X == 0 && pen.Y == 0 {
			return true
		}
		if d := abs(pen.X) + abs(pen.Y); d > deepest {
			deepest = d
			best = Overlap{BoxA: a, BoxB: b, Penetration: pen}
		}
		return true
	})
	return best, deepest > 0
}

func abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
