package raycast

import (
	"math"
	"slices"
)

// hitDistance reports where a unit ray enters box (zero from inside) and leaves it, if at all.
func hitDistance(origin, dir vec, box rect) (near, far float64, ok bool) {
	near, far = math.Inf(-1), math.Inf(1)
	x1, y1, x2, y2 := box.x1, box.y1, box.x2, box.y2

	if dir.x == 0 {
		if origin.x < x1 || origin.x > x2 {
			return 0, 0, false
		}
	} else {
		inv := 1 / dir.x
		t1 := (x1 - origin.x) * inv
		t2 := (x2 - origin.x) * inv
		if t1 > t2 {
			t1, t2 = t2, t1
		}
		near, far = t1, t2
	}

	if dir.y == 0 {
		if origin.y < y1 || origin.y > y2 {
			return 0, 0, false
		}
	} else {
		inv := 1 / dir.y
		t1 := (y1 - origin.y) * inv
		t2 := (y2 - origin.y) * inv
		if t1 > t2 {
			t1, t2 = t2, t1
		}
		if t1 > near {
			near = t1
		}
		if t2 < far {
			far = t2
		}
	}

	if far < near || far < 0 {
		return 0, 0, false
	}
	return math.Max(near, 0), far, true
}

// rayDir is the unit ray abs radians from the x axis, rounded as round32 rounds it.
func rayDir(abs float64) vec {
	sin, cos := math.Sincos(abs)
	return vec{cos, sin}.round32()
}

// minWide is the fewest angles a candidate is active at for its slabs to be laid four at a time.
const minWide = 2

// slabs is hitDistance of the rays of one walk through the boxes of the candidates active at their
// angles, laid out before the walk casts; a slab that misses has near above far.
type slabs struct {
	dirs      []vec
	along     []int32   // the angles whose ray runs along an axis
	ix, iy    []float64 // 1/dir of each angle, padded three past the last for loads near the end
	runs      []run
	near, far []float64
}

// run is the angles from to to (exclusive) a candidate is active at; its slab of angle j lies at
// at+j.
type run struct{ from, to, at int32 }

// lay works out the slabs of a walk along angles that skips those within minGap of the last, as
// the events make each candidate active.
func (s *slabs) lay(
	origin vec,
	coneDir, minGap float64,
	angles []float64,
	events []event,
	cands []candidate,
) {
	n := len(angles)
	s.dirs = slices.Grow(s.dirs[:0], n)[:n]
	for j, a := range angles {
		if j > 0 && a-angles[j-1] < minGap {
			s.dirs[j] = s.dirs[j-1] // never cast
		} else {
			s.dirs[j] = rayDir(coneDir + a)
		}
	}
	if len(cands) == 0 {
		return
	}
	s.aim()

	// A candidate turns active at the first angle at or past its event, as walk has it.
	s.runs = slices.Grow(s.runs[:0], len(cands))[:len(cands)]
	j := 0
	for _, e := range events {
		for j < n && angles[j] < e.angle {
			j++
		}
		if e.enter {
			s.runs[e.idx].from = int32(j)
		} else {
			s.runs[e.idx].to = int32(j)
		}
	}
	size := int32(0)
	for i := range s.runs {
		r := &s.runs[i]
		r.at = size - r.from
		size += r.to - r.from
	}
	// Three past the last run, for stores of a whole vector near its end.
	s.near = slices.Grow(s.near[:0], int(size)+3)[:size+3]
	s.far = slices.Grow(s.far[:0], int(size)+3)[:size+3]

	// Runs are filled in the order they lie in: what a store writes past one is overwritten by the
	// next.
	for i, r := range s.runs {
		if r.from < r.to {
			s.fill(origin, cands[i].box, r)
		}
	}
}

// aim readies the rays for the wide fill: the reciprocals of all, and those that run along an axis.
func (s *slabs) aim() {
	if !wide {
		return
	}
	n := len(s.dirs)
	s.ix, s.iy = slices.Grow(s.ix[:0], n+3)[:n+3], slices.Grow(s.iy[:0], n+3)[:n+3]
	s.along = s.along[:0]
	for j, d := range s.dirs {
		s.ix[j], s.iy[j] = d.x, d.y
		if d.x == 0 || d.y == 0 {
			s.along = append(s.along, int32(j))
		}
	}
	for j := n; j < n+3; j++ {
		s.ix[j], s.iy[j] = 1, 1
	}
	invertWide(s.ix)
	invertWide(s.iy)
}

// fill lays the slabs of box against the rays of run's angles.
func (s *slabs) fill(origin vec, box rect, r run) {
	if !wide || r.to-r.from < minWide {
		for j := r.from; j < r.to; j++ {
			s.near[r.at+j], s.far[r.at+j] = slab(origin, s.dirs[j], box)
		}
		return
	}
	x1, y1 := box.x1-origin.x, box.y1-origin.y
	x2, y2 := box.x2-origin.x, box.y2-origin.y
	s.fillWide(x1, y1, x2, y2, r)
	// An edge through the origin times the infinite reciprocal of a ray along it is NaN.
	if x1 == 0 || y1 == 0 || x2 == 0 || y2 == 0 {
		for _, j := range s.along {
			if j >= r.from && j < r.to {
				s.near[r.at+j], s.far[r.at+j] = slab(origin, s.dirs[j], box)
			}
		}
	}
}

// slab is hitDistance with a miss as the slabs hold it, near above far.
func slab(origin, dir vec, box rect) (near, far float64) {
	near, far, ok := hitDistance(origin, dir, box)
	if !ok {
		return math.Inf(1), math.Inf(-1)
	}
	return near, far
}

// laid reports whether the walk laid out the slabs of angle at; at below 0 is an angle between its
// own.
func laid(at int) bool { return wide && at >= 0 }

// rayAt is the ray of angle at, abs radians from the x axis.
func (s *slabs) rayAt(at int, abs float64) vec {
	if !laid(at) {
		return rayDir(abs)
	}
	return s.dirs[at]
}

// slabAt is hitDistance of the ray of a laid angle at through the box of candidate i.
func (s *slabs) slabAt(at, i int) (near, far float64, ok bool) {
	p := s.runs[i].at + int32(at)
	near, far = s.near[p], s.far[p]
	return near, far, near <= far
}
