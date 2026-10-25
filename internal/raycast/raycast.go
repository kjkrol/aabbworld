package raycast

import (
	"math"
	"slices"

	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/uid"
)

// QueryableSpace is the slice of a spatial index this package needs.
type QueryableSpace interface {
	Query(aabb geom.AABB, fn func(id uid.UID64)) int
	EntryAABB(id uid.UID64) (geom.AABB, bool)
	Bounds() (width, height uint32, wrapX, wrapY bool)
}

// Cone bounds a visibility query: a direction, a half-angle either side of it, how far it reaches,
// how see-through each entity is (nil: nothing is) and, when sight has heights, where the eye is,
// how tall each entity stands (nil: all heights) and how high the ground lies (nil: flat).
type Cone struct {
	Direction    geom.Vec
	HalfAngle    float32
	Radius       float32
	Transparency func(id uid.UID64) float32
	Eye          float32
	Elevation    func(id uid.UID64) (bottom, top float32)
	Ground       func(p geom.Vec) float32
	GroundStep   float32
	Profile      ProfileField
	Cover        CoverField
}

// View is one observer's line of sight: scanned once, then read as many ways as needed.
// The zero value is ready; it must not be copied once scanned, and serves one goroutine.
type View struct {
	scratch

	col     collector
	colFn   func(uid.UID64)
	areas   []geom.AABB
	found   []seen
	uniform []float64
	depths  []sample

	origin    vec
	coneDir   float64
	halfAngle float64
	radius    float64
	elev      elevation
	elevated  bool
	valid     bool
}

// seen is one entity's nearest hit across a sweep.
type seen struct {
	id   uid.UID64
	dist float64
}

// Scan records what observer sees through cone and reports whether it could.
func (v *View) Scan(space QueryableSpace, observer uid.UID64, cone Cone) bool {
	v.scratch.reset()
	v.valid = false
	v.field = cone.Cover

	if cone.HalfAngle <= 0 || cone.HalfAngle >= math.Pi || cone.Radius <= 0 {
		return false
	}
	eye, ok := space.EntryAABB(observer)
	if !ok {
		return false
	}

	width, height, wrapX, wrapY := space.Bounds()
	w, h := float64(width), float64(height)
	eyeBox := eye

	v.origin = centerOf(eyeBox).round32()
	v.coneDir = math.Atan2(float64(cone.Direction.Y), float64(cone.Direction.X))
	v.halfAngle = float64(cone.HalfAngle)
	v.radius = float64(cone.Radius)
	v.elevated = cone.Elevation != nil || cone.Ground != nil || cone.Profile != nil
	var elev *elevation
	if v.elevated {
		step := float64(cone.GroundStep)
		if step <= 0 {
			step = v.radius / 16
		}
		v.elev = elevation{eye: float64(cone.Eye), ground: cone.Ground, profile: cone.Profile, step: step, w: wrapSize(w, wrapX), h: wrapSize(h, wrapY)}
		elev = &v.elev
	}

	v.samples = sweep(v.origin, v.coneDir, v.halfAngle, v.radius, v.gather(space, observer, eyeBox, cone, w, h, wrapX, wrapY), elev, &v.scratch)
	v.valid = true
	return true
}

// Entities calls fn for every entity in view, nearest first, and returns how many.
func (v *View) Entities(fn func(id uid.UID64, dist float32)) int {
	if !v.valid {
		return 0
	}

	found := v.found[:0]
	for _, c := range v.cands {
		if !math.IsInf(c.seenAt, 1) {
			found = append(found, seen{c.id, c.seenAt})
		}
	}
	v.found = found

	slices.SortFunc(found, func(a, b seen) int {
		switch {
		case a.dist < b.dist:
			return -1
		case a.dist > b.dist:
			return 1
		default:
			return 0
		}
	})
	for _, f := range found {
		fn(f.id, float32(f.dist))
	}
	return len(found)
}

// Depths appends to dst the reach at k ≥ 2 angles evenly spaced across the cone.
func (v *View) Depths(k int, dst []float32) []float32 {
	if !v.valid || k < 2 {
		return dst
	}

	angles := v.uniform[:0]
	step := 2 * v.halfAngle / float64(k-1)
	for i := range k {
		angles = append(angles, -v.halfAngle+float64(i)*step)
	}
	v.uniform = angles

	var elev *elevation
	if v.elevated {
		elev = &v.elev
	}
	v.active, v.depths = walk(v.origin, v.coneDir, v.radius, 0,
		angles, v.events, v.cands, v.active, elev, &v.scratch, false, v.depths[:0])

	for _, s := range v.depths {
		dst = append(dst, float32(s.dist))
	}
	return dst
}

// Shadows appends to dst the stretches of ground the observer cannot see within the radius, at the
// same k ≥ 2 angles as Depths: with heights, every run of hidden ground (behind a crest, in a wall's
// shadow, past a cliff) up to where ground is lit again; on a plane, the stretch from the reach to
// the radius. Shadow.Sample is the angle's index.
func (v *View) Shadows(k int, dst []Shadow) []Shadow {
	if !v.valid || k < 2 {
		return dst
	}
	angles := v.uniform[:0]
	step := 2 * v.halfAngle / float64(k-1)
	for i := range k {
		angles = append(angles, -v.halfAngle+float64(i)*step)
	}
	v.uniform = angles

	var elev *elevation
	if v.elevated {
		elev = &v.elev
	}
	v.shade.on, v.shade.out = true, dst
	v.active, v.depths = walk(v.origin, v.coneDir, v.radius, 0,
		angles, v.events, v.cands, v.active, elev, &v.scratch, false, v.depths[:0])
	dst = v.shade.out
	v.shade.on, v.shade.out = false, nil
	return dst
}

// Outline appends the lit region to dst as a fan from the observer; maxArcStep 0 picks a default.
func (v *View) Outline(maxArcStep float32, dst []geom.Vec) []geom.Vec {
	if !v.valid {
		return dst
	}
	step := float64(maxArcStep)
	if step <= 0 {
		step = arcStep
	}

	dst = append(dst, v.origin.vec32())
	at := func(rel, dist float64) geom.Vec {
		a := v.coneDir + rel
		return vec{v.origin.x + dist*math.Cos(a), v.origin.y + dist*math.Sin(a)}.vec32()
	}

	for i, s := range v.samples {
		if i > 0 {
			prev := v.samples[i-1]
			if v.free(prev) && v.free(s) {
				for a := prev.angle + step; a < s.angle; a += step {
					dst = append(dst, at(a, v.radius))
				}
			}
		}
		dst = append(dst, at(s.angle, s.dist))
	}
	return dst
}

// free reports whether s reached the full radius, so the outline runs along the arc up to it.
func (v *View) free(s sample) bool { return !s.hit && v.radius-s.dist <= 1e-9*v.radius }

// scratch holds the working buffers one scan needs — candidates, event list,
// samples — kept across scans so a repeated query stops allocating.
type scratch struct {
	cands     []candidate
	dedup     map[uid.UID64]struct{}
	angles    []float64
	events    []event
	active    []int
	crossings []crossing
	stretches []crossing
	samples   []sample
	shade     shadowing
	field     CoverField // the ground cover the casts walk, nil for none
	cover     coverWalk  // the current cast's stretches of cover
	profile   profileWalk
}

func (s *scratch) reset() {
	if s.dedup == nil {
		s.dedup = map[uid.UID64]struct{}{}
	}
	s.cands = s.cands[:0]
	clear(s.dedup)
	s.angles = s.angles[:0]
	s.events = s.events[:0]
	s.active = s.active[:0]
	s.crossings = s.crossings[:0]
	s.stretches = s.stretches[:0]
	s.samples = s.samples[:0]
}

// candidate is an entity the cone may reach: tau is its transparency (0 blocks sight), bottom and
// top its heights, foot the ground under it, standing whether it rests on that ground, and seenAt
// the nearest distance a cast saw it from (+Inf until one does).
type candidate struct {
	id       uid.UID64
	dist     float64
	span     arc
	box      rect
	tau      float64
	bottom   float64
	top      float64
	foot     float64
	standing bool
	seenAt   float64
}

func (c *candidate) see(dist float64) { c.seenAt = math.Min(c.seenAt, dist) }

// collector is what one gather needs while the index walks it.
type collector struct {
	space    QueryableSpace
	observer uid.UID64
	eyeBox   geom.AABB
	origin   vec
	cone     Cone
	coneDir  float64
	radius   float64
	half     float64 // the cone's half-angle
	edges    wedge
	w, h     float64
	wrapX    bool
	wrapY    bool
	sc       *scratch
}

func (c *collector) take(id uid.UID64) {
	if id == c.observer {
		return
	}
	if _, dup := c.sc.dedup[id]; dup {
		return
	}
	raw, ok := c.space.EntryAABB(id)
	if !ok {
		return
	}
	box := raw
	if c.wrapX || c.wrapY {
		box = nearestImage(c.origin, box, wrapSize(c.w, c.wrapX), wrapSize(c.h, c.wrapY))
	}
	dist := boxDistance(c.eyeBox, box)
	if dist > c.radius || c.edges.excludes(c.origin, box) {
		return
	}
	tau := 0.0
	if c.cone.Transparency != nil {
		tau = min(float64(c.cone.Transparency(id)), 1)
	}
	span := subtendedArc(c.origin, box, c.coneDir, c.half)
	if tau > 0 && surrounds(box, c.origin) {
		span = arc{-c.half, c.half} // a see-through box around the eye dims the whole cone
	}
	if span.empty() {
		return
	}
	cand := candidate{id: id, dist: dist, span: span, box: rectOf(box), tau: tau, bottom: math.Inf(-1), top: math.Inf(1), standing: true, seenAt: math.Inf(1)}
	if c.cone.Elevation != nil {
		bottom, top := c.cone.Elevation(id)
		cand.bottom, cand.top = float64(bottom), float64(top)
	}
	if c.cone.Ground != nil {
		cand.foot = float64(c.cone.Ground(centerOf(raw).vec32()))
	}
	// resting on the ground, not floating above it; the slack is a few float32 steps of the height
	cand.standing = cand.bottom <= cand.foot+1e-6*max(1, math.Abs(cand.foot))
	c.sc.dedup[id] = struct{}{}
	c.sc.cands = append(c.sc.cands, cand)
}

// gather collects the entities within range whose angular span overlaps the cone.
func (v *View) gather(
	space QueryableSpace,
	observer uid.UID64,
	eyeBox geom.AABB,
	cone Cone,
	w, h float64,
	wrapX, wrapY bool,
) []candidate {
	v.col = collector{
		space: space, observer: observer, eyeBox: eyeBox, origin: v.origin, cone: cone,
		coneDir: v.coneDir, radius: v.radius, half: v.halfAngle, edges: newWedge(v.coneDir, v.halfAngle),
		w: w, h: h, wrapX: wrapX, wrapY: wrapY, sc: &v.scratch,
	}
	if v.colFn == nil {
		v.colFn = v.col.take
	}

	v.areas = searchAreas(v.origin, v.radius, w, h, wrapX, wrapY, v.areas[:0])
	for _, r := range v.areas {
		space.Query(r, v.colFn)
	}
	return v.cands
}
