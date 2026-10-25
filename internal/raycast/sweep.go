package raycast

import (
	"math"
	"slices"

	"github.com/kjkrol/uid"
)

// sample is the reach of the view at one angle: how far it gets before meeting
// something, and what that something was.
type sample struct {
	angle float64 // relative to the cone's direction
	dist  float64
	id    uid.UID64
	hit   bool
}

// event marks a candidate entering or leaving the set of boxes whose angular
// span covers the sweep angle.
type event struct {
	angle float64
	idx   int
	enter bool
}

// arcStep is the angle between samples where the reach is curved: a free arc, a see-through box,
// any cone with heights.
const arcStep = math.Pi / 90 // two degrees

// sweep records what the view meets at every angle where the reach can change shape.
func sweep(origin vec, coneDir, halfAngle, radius float64, cands []candidate, elev *elevation, sc *scratch) []sample {
	eps := math.Atan2(1, radius)

	angles := sc.angles
	add := func(a float64) {
		if a >= -halfAngle && a <= halfAngle {
			angles = append(angles, a)
		}
	}
	add(-halfAngle)
	add(halfAngle)
	// Over uneven ground or ground cover the reach changes everywhere, so the whole cone is sampled
	// at the arc step, and around cover edges more finely still.
	everywhere := elev != nil && (elev.ground != nil || elev.profile != nil) || sc.field != nil
	if everywhere {
		for a := -halfAngle + arcStep; a < halfAngle; a += arcStep {
			add(a)
		}
	}

	events := sc.events
	for i, c := range cands {
		add(c.span.lo - eps)
		add(c.span.lo + eps)
		add(c.span.hi - eps)
		add(c.span.hi + eps)
		// The reach behind a see-through box, or any box with a height, is curved, so its span is
		// sampled at the arc step too.
		if !everywhere && (c.tau > 0 || elev != nil) {
			for a := c.span.lo + arcStep; a < c.span.hi; a += arcStep {
				add(a)
			}
		}
		events = append(events,
			event{angle: c.span.lo - eps, idx: i, enter: true},
			event{angle: c.span.hi + eps, idx: i})
	}
	slices.Sort(angles)
	slices.SortFunc(events, func(a, b event) int {
		switch {
		case a.angle < b.angle:
			return -1
		case a.angle > b.angle:
			return 1
		default:
			return 0
		}
	})

	active, out := walk(origin, coneDir, radius, eps/8, angles, events, cands, sc.active, elev, sc, true, sc.samples)
	sc.angles, sc.events, sc.active, sc.samples = angles, events, active, out
	return out
}

// walk casts along ascending angles, one sample each, skipping any within minGap of the last;
// with mark the casts note on each candidate where they saw it.
func walk(
	origin vec,
	coneDir, radius, minGap float64,
	angles []float64,
	events []event,
	cands []candidate,
	active []int,
	elev *elevation,
	sc *scratch,
	mark bool,
	out []sample,
) ([]int, []sample) {
	active = active[:0]
	next := 0
	// The marking sweep over cover refines where the reach jumps between two samples: a cover edge.
	refine := mark && sc.field != nil

	for i, a := range angles {
		if i > 0 && a-angles[i-1] < minGap {
			continue
		}
		sc.shade.sample = i
		moved := false
		for next < len(events) && events[next].angle <= a {
			e := events[next]
			if e.enter {
				active = append(active, e.idx)
			} else if at := slices.Index(active, e.idx); at >= 0 {
				active = slices.Delete(active, at, at+1)
			}
			next++
			moved = true
		}
		var s sample
		if elev != nil {
			s = castElevated(origin, coneDir, a, radius, cands, active, elev, sc, mark)
		} else {
			s = castFlat(origin, coneDir, a, radius, cands, active, sc, mark)
			if sc.shade.on {
				sc.shade.emit(s.dist, radius) // on a plane nothing past the reach is seen
			}
		}
		if refine && !moved && len(out) > 0 {
			c := caster{origin: origin, coneDir: coneDir, radius: radius, cands: cands, active: active, elev: elev, sc: sc, mark: mark}
			out = c.refine(out, out[len(out)-1], s, maxRefine, 8*minGap)
		}
		out = append(out, s)
	}
	return active, out
}

// maxRefine caps how many times a gap between two samples is halved at a cover edge.
const maxRefine = 8

// caster casts along one angle with what a walk has at hand: the active candidates and the cover.
type caster struct {
	origin  vec
	coneDir float64
	radius  float64
	cands   []candidate
	active  []int
	elev    *elevation
	sc      *scratch
	mark    bool
}

func (c *caster) cast(a float64) sample {
	if c.elev != nil {
		return castElevated(c.origin, c.coneDir, a, c.radius, c.cands, c.active, c.elev, c.sc, c.mark)
	}
	s := castFlat(c.origin, c.coneDir, a, c.radius, c.cands, c.active, c.sc, c.mark)
	if c.sc.shade.on {
		c.sc.shade.emit(s.dist, c.radius) // on a plane nothing past the reach is seen
	}
	return s
}

// refine appends, in order, the samples between lo and hi that halving the gap finds where the
// reach jumps — a wall of cover begins or ends — down to minGap or depth halvings.
func (c *caster) refine(out []sample, lo, hi sample, depth int, minGap float64) []sample {
	if depth == 0 || hi.angle-lo.angle < minGap || !jumps(lo, hi, c.radius) {
		return out
	}
	m := c.cast((lo.angle + hi.angle) / 2)
	out = c.refine(out, lo, m, depth-1, minGap)
	out = append(out, m)
	return c.refine(out, m, hi, depth-1, minGap)
}

// jumps reports whether the reach changes between two samples more than a smooth wall would.
func jumps(a, b sample, radius float64) bool {
	return a.hit != b.hit || math.Abs(a.dist-b.dist) > 2*radius*(b.angle-a.angle)
}

// crossing is one box on a ray: entry, exit, the budget each unit inside costs (0 for a box that
// blocks) and which candidate it is.
type crossing struct {
	near, far float64
	rate      float64 // 1/tau, 0 when opaque
	idx       int
}

// castFlat follows one angle on a plane: the nearest blocking candidate is the wall, see-through
// ones eat the budget. The sample is a hit only when the wall is reached before the budget runs out.
func castFlat(origin vec, coneDir, rel, radius float64, cands []candidate, active []int, sc *scratch, mark bool) sample {
	abs := coneDir + rel
	dir := vec{math.Cos(abs), math.Sin(abs)}.round32()
	cross := &sc.crossings

	best, wall := sample{angle: rel, dist: radius}, noCut
	crossed := 0
	for _, i := range active {
		c := cands[i]
		near, far, ok := hitDistance(origin, dir, c.box)
		if !ok {
			continue
		}
		if c.tau > 0 {
			if crossed == 0 {
				*cross = (*cross)[:0]
			}
			*cross = insertCrossing(*cross, crossing{near: near, far: far, rate: 1 / c.tau, idx: i})
			crossed++
			continue
		}
		if near > best.dist {
			continue
		}
		best.dist, best.id, best.hit, wall = near, c.id, true, i
	}
	// The cover up to the nearest wall so far: the first blocking stretch may be nearer still.
	if sc.field != nil {
		crossed = castCover(origin, dir, radius, &best, &wall, crossed, sc)
	}
	if crossed > 0 {
		// An empty stretch costs its length, one inside a box length·rate; overlaps are charged once.
		if reach := spend(*cross, best.dist, radius); !best.hit || best.dist > reach {
			best.dist, best.id, best.hit, wall = reach, 0, false, noCut
		}
		for _, x := range *cross {
			if mark && x.near <= best.dist && !isCover(x.idx) {
				cands[x.idx].see(x.near)
			}
		}
	}
	if mark && wall >= 0 {
		cands[wall].see(best.dist)
	}
	return best
}

// castCover walks the cover along a flat cast up to its wall so far: a blocking stretch nearer
// becomes the wall, see-through ones join the crossings; it returns how many crossings there are.
func castCover(origin, dir vec, radius float64, best *sample, wall *int, crossed int, sc *scratch) int {
	budget := 0.0
	if crossed == 0 {
		budget = radius // only the cover spends it, so the walk may stop where it runs out
	}
	sc.walkCover(origin, dir, best.dist, true, budget)
	cross := &sc.crossings
	for k := range sc.cover.near {
		x := sc.crossingOf(k)
		if x.rate == 0 {
			if x.near < best.dist {
				best.dist, best.id, best.hit, *wall = x.near, 0, true, x.idx
			}
			break
		}
		if crossed == 0 {
			*cross = (*cross)[:0]
		}
		*cross = insertCrossing(*cross, x)
		crossed++
	}
	return crossed
}

// spend walks the see-through stretches sorted by entry, charging each once, and returns where the
// budget runs out; nothing at or past limit is charged.
func spend(cross []crossing, limit, budget float64) float64 {
	pos := 0.0
	for _, x := range cross {
		if x.far <= pos || x.near >= limit {
			continue
		}
		start := math.Max(x.near, pos)
		if gap := start - pos; gap >= budget {
			break
		}
		budget, pos = budget-(start-pos), start
		cost := (x.far - start) * x.rate
		if cost >= budget {
			budget, pos = 0, start+budget/x.rate
			break
		}
		budget, pos = budget-cost, x.far
	}
	return pos + budget
}

// insertCrossing keeps cross sorted by where each box is entered; the list is a handful long.
func insertCrossing(cross []crossing, x crossing) []crossing {
	cross = append(cross, x)
	for i := len(cross) - 1; i > 0 && cross[i-1].near > x.near; i-- {
		cross[i] = cross[i-1]
		cross[i-1] = x
	}
	return cross
}
