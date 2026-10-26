package raycast

import (
	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/uid"
)

// Reach is one sample of a View: the angle, how far sight gets along it and what it meets there.
type Reach struct {
	Angle, Dist float64
	ID          uid.UID64
	Hit         bool
}

// Reaches is what the last Scan sampled.
func (v *View) Reaches() []Reach { return reaches(v.samples) }

// DepthReaches is what the last Depths or Shadows sampled.
func (v *View) DepthReaches() []Reach { return reaches(v.depths) }

func reaches(samples []sample) []Reach {
	out := make([]Reach, len(samples))
	for i, s := range samples {
		out[i] = Reach{Angle: s.angle, Dist: s.dist, ID: s.id, Hit: s.hit}
	}
	return out
}

// ProfileOf is the ground along a ray as a cast records it: read off f sunk by bend, or, where f
// is nil, handed over by p with ground under the end should p stop short.
func ProfileOf(f *HeightField, bend float32, p ProfileField, ground func(geom.Vec) float32,
	origin, dir geom.Vec, length float32) (d, alt []float64) {
	e := elevation{ground: ground, profile: p}
	if f != nil {
		e.over(f, bend)
	}
	var sc scratch
	o, u := vec{float64(origin.X), float64(origin.Y)}, vec{float64(dir.X), float64(dir.Y)}
	sc.walkProfile(&e, o, u, float64(length))
	return sc.profile.d, sc.profile.alt
}

// FieldProfile is f sunk by bend as the ProfileField a scan holds it as.
func FieldProfile(f *HeightField, bend float32) ProfileField {
	var l lattice
	l.set(f, bend)
	return &l
}

// HeightAt is f's ground under p.
func HeightAt(f *HeightField, p geom.Vec) float32 {
	var l lattice
	l.set(f, 0)
	return l.at(p)
}
