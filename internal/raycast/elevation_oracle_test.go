package raycast_test

import (
	"math"
	"sort"

	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/internal/raycast"
	"github.com/kjkrol/uid"
)

// elevatedOracle is a scene with heights, and the sight model over it spelled out plainly: a
// point is in sight when the straight line from the eye to it stays above every nearer ground
// point, outside every nearer blocking box, and within the budget through the see-through ones.
type elevatedOracle struct {
	boxes  map[uid.UID64][4]float64
	taus   map[uid.UID64]float64
	bands  map[uid.UID64]band
	ground func(geom.Vec) float64
	eye    float64
	step   float64
	// profile, when set, gives the ground points of a ray in place of a sample every step
	profile raycast.ProfileField
}

type oracleHit struct {
	id        uid.UID64
	near, far float64
	tau       float64
	band      band
	standing  bool
}

// groundPoint is a lit-or-hidden ground target: a sample along the ray, or the foot of the box self.
type groundPoint struct {
	dist, alt float64
	self      uid.UID64
}

// cast follows one ray: the lit reach along it and the entities it sees.
func (o elevatedOracle) cast(origin geom.Vec, angle, radius float64) (reach float64, seen map[uid.UID64]bool) {
	reach, seen, _ = o.castAll(origin, angle, radius)
	return reach, seen
}

// groundSeen is one ground point of a ray, a sample or a foot, and whether the eye sees it.
type groundSeen struct {
	dist    float64
	visible bool
}

// castAll is cast with every ground point the ray judges, in no particular order.
func (o elevatedOracle) castAll(origin geom.Vec, angle, radius float64) (reach float64, seen map[uid.UID64]bool, ground []groundSeen) {
	dx, dy := math.Cos(angle), math.Sin(angle)
	at := func(d float64) geom.Vec { return geom.NewVec(origin.X+dx*d, origin.Y+dy*d) }

	var hits []oracleHit
	for id, b := range o.boxes {
		near, far, ok := slabHit(b, origin, dx, dy)
		if !ok {
			continue
		}
		foot := o.ground(geom.NewVec((b[0]+b[2])/2, (b[1]+b[3])/2))
		hits = append(hits, oracleHit{id, near, far, math.Min(o.taus[id], 1), o.bands[id], o.bands[id][0] <= foot+1e-6})
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].near < hits[j].near })

	var points, faces []groundPoint
	if o.profile != nil {
		o.profile.Along(origin, geom.NewVec(dx, dy), radius, func(d, alt float64) bool {
			points = append(points, groundPoint{d, alt, 0})
			return true
		})
		// the ground where the ray meets each box, straight between the profile's points: it hides,
		// up to the box, what lies behind it
		prof := append([]groundPoint{{0, o.ground(origin), 0}}, points...)
		for _, h := range hits {
			for k := 1; k < len(prof); k++ {
				if a, b := prof[k-1], prof[k]; h.near > a.dist && h.near < b.dist {
					f := (h.near - a.dist) / (b.dist - a.dist)
					faces = append(faces, groundPoint{h.near, a.alt + f*(b.alt-a.alt), h.id})
				}
			}
		}
	} else {
		for d := o.step; d < radius; d += o.step {
			points = append(points, groundPoint{d, o.ground(at(d)), 0})
		}
		points = append(points, groundPoint{radius, o.ground(at(radius)), 0})
	}
	for _, h := range hits {
		if h.standing && h.near > 0 && h.near <= radius {
			points = append(points, groundPoint{h.near, h.band[0], h.id})
		}
	}

	height := func(tan, d float64) float64 {
		if d == 0 {
			return o.eye
		}
		return o.eye + tan*d
	}
	// aboveGround: no nearer ground point pokes above the line.
	aboveGround := func(tan, upto float64) bool {
		for _, p := range points {
			if p.dist < upto && (p.alt-o.eye)/p.dist > tan {
				return false
			}
		}
		for _, f := range faces {
			if f.dist <= upto && (f.alt-o.eye)/f.dist > tan {
				return false
			}
		}
		return true
	}
	// clearOfWalls: the line passes no blocking box, nearer or entered at the same distance.
	clearOfWalls := func(tan, upto float64, self uid.UID64) bool {
		for _, h := range hits {
			if h.tau > 0 || h.id == self || h.near > upto {
				continue
			}
			lo, hi := height(tan, h.near), height(tan, math.Min(h.far, upto))
			if lo > hi {
				lo, hi = hi, lo
			}
			if hi > h.band[0] && lo < h.band[1] {
				return false
			}
		}
		return true
	}
	// budgetEnd: where the radius runs out along the line, charging see-through bands by depth.
	budgetEnd := func(tan, upto float64, self uid.UID64) float64 {
		type stretch struct{ lo, hi, tau float64 }
		var st []stretch
		for _, h := range hits {
			if h.tau <= 0 || h.id == self || h.near > upto {
				continue
			}
			from, to := h.near, math.Min(h.far, upto)
			var lo, hi float64
			switch {
			case math.IsInf(tan, 0):
				if !math.IsInf(h.band[1], 1) {
					continue
				}
				lo, hi = from, to
			case tan == 0:
				if o.eye < h.band[0] || o.eye > h.band[1] {
					continue
				}
				lo, hi = from, to
			default:
				d1, d2 := (h.band[0]-o.eye)/tan, (h.band[1]-o.eye)/tan
				lo, hi = math.Max(from, math.Min(d1, d2)), math.Min(to, math.Max(d1, d2))
			}
			if lo < hi {
				st = append(st, stretch{lo, hi, h.tau})
			}
		}
		sort.Slice(st, func(i, j int) bool { return st[i].lo < st[j].lo })
		budget, pos := radius, 0.0
		for _, s := range st {
			if s.hi <= pos {
				continue
			}
			start := math.Max(s.lo, pos)
			if start-pos >= budget {
				break
			}
			budget -= start - pos
			pos = start
			if cost := (s.hi - start) / s.tau; cost >= budget {
				return pos + budget*s.tau
			} else {
				budget -= cost
				pos = s.hi
			}
		}
		return pos + budget
	}

	seen = map[uid.UID64]bool{}
	for _, h := range hits {
		if h.near == 0 {
			seen[h.id] = true
			continue
		}
		tan := (h.band[1] - o.eye) / h.near
		if aboveGround(tan, h.near) && clearOfWalls(tan, h.near, h.id) && budgetEnd(tan, h.near, h.id) >= h.near {
			seen[h.id] = true
		}
	}
	for _, p := range points {
		tan := (p.alt - o.eye) / p.dist
		if !aboveGround(tan, p.dist) || !clearOfWalls(tan, p.dist, p.self) {
			ground = append(ground, groundSeen{p.dist, false})
			continue
		}
		end := budgetEnd(tan, p.dist, p.self)
		reach = math.Max(reach, math.Min(p.dist, end))
		ground = append(ground, groundSeen{p.dist, end >= p.dist})
	}
	return reach, seen, ground
}
