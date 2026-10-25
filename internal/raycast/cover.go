package raycast

import (
	"math"

	"github.com/kjkrol/aabbworld/geom"
)

// CoverField is the ground cover of a world laid out on a grid: walls, forests, whatever stands on
// its cells and holds sight back. Walk follows the ray from origin along the unit dir for length and
// calls visit, nearest first, for every stretch of it inside a cell with cover: where the ray enters
// and leaves the cell, the band the cover spans (bottom, top; ±Inf without heights) and how
// see-through it is (tau: ≤ 0 blocks, 1 is empty). visit returning false ends the walk.
type CoverField interface {
	Walk(origin, dir geom.Vec, length float32, visit func(near, far, bottom, top, tau float32) bool)
}

// noCut marks a cast that ended on no wall; a crossing's idx below it names a stretch of cover.
const noCut = -1

// coverIdx is the crossing index of the k-th stretch of cover gathered for the current cast.
func coverIdx(k int) int { return -2 - k }

// isCover reports whether a crossing index names a stretch of cover rather than a candidate.
func isCover(idx int) bool { return idx <= -2 }

// coverWalk is one cast's walk over the cover: the stretches found — entry, exit, transparency,
// and with heights a candidate of their own for the band — and, on a plane, where the walk may stop.
type coverWalk struct {
	near, far, tau []float64
	stretches      []candidate // with heights only
	heights        bool
	// On a plane the walk stops at a wall, and once the cover alone has spent the budget when
	// nothing else on the ray is see-through.
	flat, byBudget bool
	pos, budget    float64
	visit          func(near, far, bottom, top, tau float32) bool
}

// take records one stretch of cover and says whether the walk goes on.
func (w *coverWalk) take(near, far, bottom, top, tau float64) bool {
	tau = min(tau, 1)
	if w.byBudget {
		if near-w.pos >= w.budget {
			return false
		}
		w.budget -= near - w.pos
	}
	w.near, w.far, w.tau = append(w.near, near), append(w.far, far), append(w.tau, max(tau, 0))
	if w.heights {
		w.stretches = append(w.stretches, candidate{tau: max(tau, 0), bottom: bottom, top: top, foot: bottom, standing: !math.IsInf(bottom, -1), seenAt: math.Inf(1)})
	}
	if !w.flat {
		return true
	}
	if tau <= 0 {
		return false
	}
	if w.byBudget {
		w.budget, w.pos = w.budget-(far-near)/tau, far
		return w.budget > 0
	}
	return true
}

// walkCover gathers the cover along one ray, up to length, into sc.cover: on a plane (flat) up to
// the first wall, and no further than budget when nothing else on the ray is see-through
// (budget > 0); with heights all of it, bands included.
func (sc *scratch) walkCover(origin, dir vec, length float64, flat bool, budget float64) {
	w := &sc.cover
	w.near, w.far, w.tau, w.stretches = w.near[:0], w.far[:0], w.tau[:0], w.stretches[:0]
	if sc.field == nil || length <= 0 {
		return
	}
	if w.visit == nil {
		w.visit = func(near, far, bottom, top, tau float32) bool {
			return w.take(float64(near), float64(far), float64(bottom), float64(top), float64(tau))
		}
	}
	w.flat, w.heights = flat, !flat
	w.byBudget, w.pos, w.budget = flat && budget > 0, 0, budget
	sc.field.Walk(origin.vec32(), dir.vec32(), float32(length), w.visit)
}

// crossingOf is the k-th stretch of cover as a crossing of the current cast.
func (sc *scratch) crossingOf(k int) crossing {
	rate := 0.0
	if tau := sc.cover.tau[k]; tau > 0 {
		rate = 1 / tau
	}
	return crossing{near: sc.cover.near[k], far: sc.cover.far[k], rate: rate, idx: coverIdx(k)}
}

// candidateOf resolves a crossing index to the candidate or the stretch of cover it names; a
// stretch has a candidate only in a cast with heights.
func candidateOf(cands []candidate, sc *scratch, idx int) *candidate {
	if isCover(idx) {
		return &sc.cover.stretches[-2-idx]
	}
	return &cands[idx]
}
