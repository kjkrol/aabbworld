package raycast

import "math"

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
