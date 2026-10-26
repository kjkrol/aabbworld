package raycast

import (
	"math"

	"github.com/kjkrol/aabbworld/geom"
)

// ProfileField is the ground along a ray, exactly: Along follows the ray from origin along the unit
// dir for length and calls visit, nearest first, with every point of it where the ground's slope
// changes — a ground of flat facets, the edges and folds of the cells it crosses — and with length
// itself: how far along it lies (d, above 0) and the ground's height there. visit returning false
// ends the walk. A wrapping world's owner folds the ray onto the world itself.
type ProfileField interface {
	Along(origin, dir geom.Vec, length float32, visit func(d, alt float32) bool)
}

// elevation is what a cast needs beyond the plane when the cone has heights: the eye, the ground
// and how far apart the ground is sampled along a ray, or its profile, or a field of heights.
type elevation struct {
	eye     float64
	ground  func(geom.Vec) float32 // nil: flat at 0
	profile ProfileField           // nil: the ground sampled every step
	lat     lattice                // with a field, the ground and the profile both
	step    float64
	w, h    float64 // wrap sizes, 0 where the axis does not wrap
}

// over has the ground read off f, sunk by bend, in place of a ground and a profile.
func (e *elevation) over(f *HeightField, bend float32) {
	e.lat.set(f, bend)
	e.ground, e.profile = nil, &e.lat
}

// grounded reports whether the ground can be read at any point, not only along a ray.
func (e *elevation) grounded() bool { return e.ground != nil || e.lat.f != nil }

// profileWalk is one cast's ground along its ray, as the profile hands it over.
type profileWalk struct {
	d, alt []float64
	length float64
	visit  func(d, alt float32) bool
}

// take records one point of the profile and says whether the walk goes on.
func (w *profileWalk) take(d, alt float64) bool {
	if d <= 0 {
		return true
	}
	if d > w.length {
		d = w.length
	}
	w.d, w.alt = append(w.d, d), append(w.alt, alt)
	return d < w.length
}

// profileAt is the ground along the ray of angle at, as the walk laid it, or walked now.
func (sc *scratch) profileAt(e *elevation, at int, origin, dir vec, length float64) *profileWalk {
	if w := sc.block.of(at); w != nil {
		return w
	}
	sc.walkProfile(e, origin, dir, length)
	return &sc.profile
}

// walkProfile gathers the ground along one ray into sc.profile, ending at length.
func (sc *scratch) walkProfile(e *elevation, origin, dir vec, length float64) {
	w := &sc.profile
	w.d, w.alt, w.length = w.d[:0], w.alt[:0], length
	if e.lat.f != nil {
		e.lat.walk(w, origin, dir)
	} else {
		if w.visit == nil {
			w.visit = func(d, alt float32) bool { return w.take(float64(d), float64(alt)) }
		}
		e.profile.Along(origin.vec32(), dir.vec32(), float32(length), w.visit)
	}
	e.close(w, origin, dir, length)
}

// close ends a profile at length: at the ground there, or the profile's last height, where the
// profile stops short of it.
func (e *elevation) close(w *profileWalk, origin, dir vec, length float64) {
	if n := len(w.d); n == 0 || w.d[n-1] < length {
		alt := 0.0
		switch {
		case e.grounded():
			alt = e.at(origin, dir, length)
		case n > 0:
			alt = w.alt[n-1]
		}
		w.d, w.alt = append(w.d, length), append(w.alt, alt)
	}
}

// at is the ground height under the point dist along the ray, folded back onto a wrapping world.
func (e *elevation) at(origin, dir vec, dist float64) float64 {
	if !e.grounded() {
		return 0
	}
	x, y := origin.x+dir.x*dist, origin.y+dir.y*dist
	p := geom.NewVec(float32(x), float32(y))
	if e.w > 0 {
		p.X = fold(x, e.w)
	}
	if e.h > 0 {
		p.Y = fold(y, e.h)
	}
	if e.lat.f != nil {
		return float64(e.lat.sunk(e.lat.at(p), float32(dist)))
	}
	return float64(e.ground(p))
}

// fold is v folded into [0, size) as a float32: a hair below size goes to 0, where rounding would
// have put it on size itself.
func fold(v, size float64) float32 {
	if v = math.Mod(v, size); v < 0 {
		v += size
	}
	if f := float32(v); f < float32(size) {
		return f
	}
	return 0
}

// ray is one elevated cast in progress: the crossings entered so far are its blockers, horizon the
// steepest sightline to the ground yet, reach the farthest lit ground and cut the wall it ends at.
type ray struct {
	origin, dir vec
	radius      float64
	cands       []candidate
	cross       []crossing
	e           *elevation
	sc          *scratch
	mark        bool
	horizon     float64
	reach       float64
	cut         int
	walls       int        // blocking crossings entered so far
	veils       int        // see-through crossings entered so far
	shade       *shadowing // nil unless the shadows of this angle are being read
}

// castElevated follows one angle with heights: an entry is seen when the sightline to its top
// clears the ground and every nearer blocking box within the budget; the reach is the farthest
// lit ground, a standing box's foot included. at is the angle's index among the walk's angles,
// below 0 for one between them.
func castElevated(
	origin vec,
	coneDir, rel float64,
	at int,
	radius float64,
	cands []candidate,
	active []int,
	e *elevation,
	sc *scratch,
	mark bool,
) sample {
	dir := sc.slabs.rayAt(at, coneDir+rel)

	cross := sc.crossings[:0]
	fromSlabs := laid(at)
	for _, i := range active {
		var near, far float64
		var ok bool
		if fromSlabs {
			near, far, ok = sc.slabs.slabAt(at, i)
		} else {
			near, far, ok = hitDistance(origin, dir, cands[i].box)
		}
		if !ok {
			continue
		}
		rate := 0.0
		if cands[i].tau > 0 {
			rate = 1 / cands[i].tau
		}
		cross = insertCrossing(cross, crossing{near: near, far: far, rate: rate, idx: i})
	}
	// The cover all the way to the radius: with heights a wall may be looked over.
	if sc.field != nil {
		sc.walkCover(origin, dir, radius, false, 0)
		for k := range sc.cover.near {
			cross = insertCrossing(cross, sc.crossingOf(k))
		}
	}
	sc.crossings = cross

	r := ray{origin: origin, dir: dir, radius: radius, cands: cands, cross: cross, e: e, sc: sc, mark: mark, horizon: math.Inf(-1), cut: noCut}
	if sc.shade.on {
		r.shade = &sc.shade
		r.shade.begin()
	}
	j := 0
	if e.profile != nil {
		p := sc.profileAt(e, at, origin, dir, radius)
		prevD, prevAlt := 0.0, p.alt[0]
		if e.grounded() {
			prevAlt = e.at(origin, dir, 0)
		}
		for k, d := range p.d {
			alt := p.alt[k]
			for j < len(cross) && cross[j].near <= d {
				// the ground where the ray meets the box, straight between the points either side,
				// hides what the points alone would not: a box on a slope rising to its face
				if near := cross[j].near; near > prevD && near < d {
					r.raise(near, prevAlt+(near-prevD)/(d-prevD)*(alt-prevAlt))
				}
				j = r.enterUpTo(cross[j].near, j)
			}
			if lit := r.ground(d, alt, j, noCut); r.shade != nil {
				r.shade.point(d, lit)
			}
			prevD, prevAlt = d, alt
		}
	} else {
		for d := e.step; ; d += e.step {
			if d > radius {
				d = radius
			}
			j = r.enterUpTo(d, j)
			if lit := r.ground(d, e.at(origin, dir, d), j, noCut); r.shade != nil {
				r.shade.point(d, lit)
			}
			if d >= radius {
				break
			}
		}
	}
	if r.shade != nil {
		r.shade.end(radius)
	}
	out := sample{angle: rel, dist: r.reach}
	switch {
	case r.cut >= 0:
		out.id, out.hit = cands[r.cut].id, true
	case isCover(r.cut):
		out.hit = true // a wall of cover, no entity
	}
	return out
}

// raise has the ground at d, alt high, hide what lies behind it, as a target of none.
func (r *ray) raise(d, alt float64) {
	if tan := (alt - r.e.eye) / d; tan > r.horizon {
		r.horizon = tan
	}
}

// enterUpTo enters every crossing from j on that begins no further than d, and says where it
// stopped.
func (r *ray) enterUpTo(d float64, j int) int {
	for j < len(r.cross) && r.cross[j].near <= d {
		r.enter(j)
		if r.cross[j].rate == 0 {
			r.walls++
		} else {
			r.veils++
		}
		j++
	}
	return j
}

// enter looks at crossing j as a target and, when it stands on the ground, lights its foot.
func (r *ray) enter(j int) {
	x := r.cross[j]
	c := candidateOf(r.cands, r.sc, x.idx)
	if x.near == 0 {
		r.see(c, 0) // the eye is inside the box: seen from where it stands
		return
	}
	if tan := (c.top - r.e.eye) / x.near; tan >= r.horizon && !r.blocked(tan, x.near, j) && r.spend(tan, x.near, j) >= x.near {
		r.see(c, x.near)
	}
	if !c.standing {
		return
	}
	cut := noCut
	if x.rate == 0 {
		cut = x.idx
	}
	if lit := r.ground(x.near, c.foot, j, cut); r.shade != nil {
		r.shade.point(x.near, lit)
	}
}

// ground takes the ground point at d as a target: lit when the sightline to it clears everything
// nearer, it becomes the reach, cut by candidate cut if any; hidden ground is passed over. It
// reports whether the point was seen.
func (r *ray) ground(d, alt float64, j, cut int) bool {
	tan := (alt - r.e.eye) / d
	if tan < r.horizon {
		return false
	}
	r.horizon = tan
	if r.blocked(tan, d, j) {
		return false
	}
	if reach := r.spend(tan, d, j); reach < d {
		r.lit(reach, noCut)
		return false
	}
	r.lit(d, cut)
	return true
}

func (r *ray) see(c *candidate, dist float64) {
	if r.mark {
		c.see(dist)
	}
}

func (r *ray) lit(d float64, cut int) {
	if d >= r.reach {
		r.reach, r.cut = d, cut
	}
}

// blocked reports whether the sightline with tangent tan, followed to upto, passes through the
// band of a blocking box among the first j crossings.
func (r *ray) blocked(tan, upto float64, j int) bool {
	if r.walls == 0 {
		return false
	}
	for _, x := range r.cross[:j] {
		if x.rate != 0 {
			continue
		}
		c := candidateOf(r.cands, r.sc, x.idx)
		lo, hi := r.heights(tan, x.near, min(x.far, upto))
		if hi > c.bottom && lo < c.top {
			return true
		}
	}
	return false
}

// spend returns where the budget runs out along the sightline with tangent tan followed to upto,
// charging every see-through box among the first j crossings for the stretch the line spends in
// its band.
func (r *ray) spend(tan, upto float64, j int) float64 {
	if r.veils == 0 {
		return r.radius
	}
	st := r.sc.stretches[:0]
	for _, x := range r.cross[:j] {
		if x.rate == 0 {
			continue
		}
		c := candidateOf(r.cands, r.sc, x.idx)
		if lo, hi, ok := r.inBand(tan, x.near, min(x.far, upto), c.bottom, c.top); ok {
			st = insertCrossing(st, crossing{near: lo, far: hi, rate: x.rate})
		}
	}
	r.sc.stretches = st
	if len(st) == 0 {
		return r.radius
	}
	return spend(st, upto, r.radius)
}

// heights is the sightline's height at both ends of a stretch, lower first; a vertical line
// (an entry with no top) is as high as anything past the eye.
func (r *ray) heights(tan, from, to float64) (lo, hi float64) {
	lo, hi = r.at(tan, from), r.at(tan, to)
	if lo > hi {
		lo, hi = hi, lo
	}
	return lo, hi
}

func (r *ray) at(tan, d float64) float64 {
	if d == 0 {
		return r.e.eye
	}
	return r.e.eye + tan*d
}

// inBand is the part of [from, to] where the sightline runs between bottom and top.
func (r *ray) inBand(tan, from, to, bottom, top float64) (lo, hi float64, ok bool) {
	switch {
	case math.IsInf(tan, 0):
		return from, to, math.IsInf(top, 1) && from < to
	case tan == 0:
		return from, to, bottom <= r.e.eye && r.e.eye <= top && from < to
	}
	d1, d2 := (bottom-r.e.eye)/tan, (top-r.e.eye)/tan
	if d1 > d2 {
		d1, d2 = d2, d1
	}
	lo, hi = max(from, d1), min(to, d2)
	return lo, hi, lo < hi
}
