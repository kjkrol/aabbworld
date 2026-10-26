package aabbworld

import (
	"maps"
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/kjkrol/aabbworld/geom"
	iraycast "github.com/kjkrol/aabbworld/internal/raycast"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

// before is the Space as a scan read it before the index was asked for the cone: every piece in
// the square round the circle, less what the scan itself then dismissed, the nearest copy of a box
// farther from the observer's box than the radius or wholly beyond an edge of the cone.
type before struct {
	sight
	eye geom.AABB
}

func (b before) QueryCone(origin, dir geom.Vec, halfAngle, radius float32, fn func(uid.UID64)) int {
	o := [2]float64{float64(origin.X), float64(origin.Y)}
	theta, half := math.Atan2(float64(dir.Y), float64(dir.X)), float64(halfAngle)
	lo := [2]float64{math.Cos(theta - half), math.Sin(theta - half)}
	hi := [2]float64{math.Cos(theta + half), math.Sin(theta + half)}
	w, h := float64(b.s.Width), float64(b.s.Height)
	if !b.s.Edges.WrapsX() {
		w = 0
	}
	if !b.s.Edges.WrapsY() {
		h = 0
	}
	square := geom.NewAABB(geom.NewVec(origin.X-radius, origin.Y-radius),
		geom.NewVec(origin.X+radius, origin.Y+radius))
	found := 0
	b.s.Query(square, AnyCapability, func(id uid.UID64) {
		box, _ := b.s.grid.EntryAABB(id)
		cx := (float64(box.TopLeft.X) + float64(box.BottomRight.X)) / 2
		cy := (float64(box.TopLeft.Y) + float64(box.BottomRight.Y)) / 2
		d := geom.NewVec(float32(o[0]+nearest(cx-o[0], w)-cx), float32(o[1]+nearest(cy-o[1], h)-cy))
		box = geom.NewAABB(box.TopLeft.Add(d), box.BottomRight.Add(d))
		gap := math.Hypot(float64(b.eye.AxisDistanceX(box)), float64(b.eye.AxisDistanceY(box)))
		if gap > float64(radius) {
			return
		}
		if half < math.Pi/2 {
			pastLo, pastHi := true, true
			for _, x := range [2]float32{box.TopLeft.X, box.BottomRight.X} {
				for _, y := range [2]float32{box.TopLeft.Y, box.BottomRight.Y} {
					dx, dy := float64(x)-o[0], float64(y)-o[1]
					pastLo = pastLo && lo[0]*dy-lo[1]*dx < 0
					pastHi = pastHi && hi[0]*dy-hi[1]*dx > 0
				}
			}
			if pastLo || pastHi {
				return
			}
		}
		fn(id)
		found++
	})
	return found
}

// nearest is d folded onto the copy of it nearest zero on an axis of size, or d where size is 0.
func nearest(d, size float64) float64 {
	switch {
	case size > 0 && d > size/2:
		return d - size
	case size > 0 && d < -size/2:
		return d + size
	}
	return d
}

// read is a scan of what observer sees through cone, read three ways.
type read struct {
	seen    []seenAt
	depths  []float32
	shadows []iraycast.Shadow
}

type seenAt struct {
	id   uid.UID64
	dist float32
}

func readScan(t *testing.T, space iraycast.QueryableSpace, observer uid.UID64, cone Cone) read {
	t.Helper()
	var v iraycast.View
	if !v.Scan(space, observer, iraycast.Cone(cone)) {
		t.Fatalf("observer %v could not scan %+v", observer, cone)
	}
	var r read
	v.Entities(func(id uid.UID64, d float32) { r.seen = append(r.seen, seenAt{id, d}) })
	r.depths, r.shadows = v.Depths(63, nil), v.Shadows(63, nil)
	return r
}

func (r read) ids() map[uid.UID64]bool {
	ids := map[uid.UID64]bool{}
	for _, s := range r.seen {
		ids[s.id] = true
	}
	return ids
}

// viewScene is the scene of the View benchmarks: n boxes 20 on a side scattered over a 4000 plane,
// and an observer 10 on a side in its middle.
func viewScene(t *testing.T, n int) *Space {
	t.Helper()
	space, err := NewSpace(Config{Width: 4000, Height: 4000, BucketSize: 256})
	if err != nil {
		t.Fatal(err)
	}
	items := []Item{{ID: 1, Box: plane.NewAABB(geom.NewVec(2000, 2000), 10, 10)}}
	r := rand.New(rand.NewPCG(1, 2))
	for i := range n {
		at := geom.NewVec(float32(r.IntN(4000)), float32(r.IntN(4000)))
		items = append(items, Item{ID: uid.UID64(100 + i), Box: plane.NewAABB(at, 20, 20)})
	}
	space.Rebuild(items)
	return space
}

// viewCones are the cones of the View benchmarks: the plain one, three entities in ten seen
// through, and with heights the rungs of the elevated one.
func viewCones() map[string]Cone {
	plain := Cone{Direction: geom.NewVec(1, 0), HalfAngle: math.Pi / 4, Radius: 800}
	seeThrough := plain
	seeThrough.Transparency = func(id uid.UID64) float32 {
		if id%10 < 3 {
			return 0.5
		}
		return 0
	}
	rolling := func(p geom.Vec) float32 {
		return float32(5 + 5*math.Sin(float64(p.X)/90)*math.Cos(float64(p.Y)/70))
	}
	raster := func(p geom.Vec) float32 {
		return rolling(geom.NewVec(float32(int(p.X/32))*32, float32(int(p.Y/32))*32))
	}
	heights := func(ground func(geom.Vec) float32, step float32) Cone {
		c := seeThrough
		c.Eye, c.Ground, c.GroundStep = 6, ground, step
		c.Elevation = func(id uid.UID64) (float32, float32) {
			switch {
			case id%5 == 0:
				return 30, 34
			case id%2 == 0:
				return 0, 2
			}
			return 0, 12
		}
		return c
	}
	return map[string]Cone{
		"plain":          plain,
		"see-through":    seeThrough,
		"entities":       heights(nil, 0),
		"flat ground":    heights(func(geom.Vec) float32 { return 5 }, 32),
		"raster":         heights(raster, 32),
		"raster step 50": heights(raster, 0),
	}
}

func TestScan_TheViewScenesReadAsBeforeTheConeWasAsked(t *testing.T) {
	for _, n := range []int{100, 1000} {
		space := viewScene(t, n)
		eye, _ := space.grid.EntryAABB(1)
		for name, cone := range viewCones() {
			got := readScan(t, sight{space}, 1, cone)
			want := readScan(t, before{sight{space}, eye}, 1, cone)
			if !slices.Equal(got.seen, want.seen) {
				t.Errorf("%d entities, %s: seen %v, before %v", n, name, got.seen, want.seen)
			}
			if !slices.Equal(got.depths, want.depths) || !slices.Equal(got.shadows, want.shadows) {
				t.Errorf("%d entities, %s: the depths or the shadows differ from before", n, name)
			}
		}
	}
}

// A random scene reads as before the index was asked for the cone: the reach and the hidden ground
// at every angle and, but for an entity seen over less than the sweep resolves, all it sees. The
// sweep casts along the edges of every box it is handed, and was once handed some out of reach.
func TestScan_ReadsAsBeforeTheConeWasAsked(t *testing.T) {
	r := rand.New(rand.NewPCG(71, 73))
	for _, edges := range []Edges{0, Torus} {
		space, err := NewSpace(Config{Width: 1500, Height: 1500, Edges: edges, BucketSize: 64})
		if err != nil {
			t.Fatal(err)
		}
		var items []Item
		for i := range 1200 {
			box := space.WrapAABB(geom.NewAABBAt(
				geom.NewVec(float32(r.Float64()*1500), float32(r.Float64()*1500)),
				float32(4+r.Float64()*30), float32(4+r.Float64()*30)))
			items = append(items, Item{ID: uid.UID64(i + 1), Box: box})
		}
		space.Rebuild(items)
		for q := range 150 {
			observer := items[r.IntN(len(items))].ID
			a := r.Float64() * 2 * math.Pi
			cone := Cone{
				Direction: geom.NewVec(float32(math.Cos(a)), float32(math.Sin(a))),
				HalfAngle: float32(0.05 + 3*r.Float64()),
				Radius:    float32(20 + 900*r.Float64()),
			}
			if q%2 == 1 {
				cone.Transparency = func(id uid.UID64) float32 { return float32(id%3) / 4 }
			}
			eye, _ := space.grid.EntryAABB(observer)
			got := readScan(t, sight{space}, observer, cone)
			want := readScan(t, before{sight{space}, eye}, observer, cone)
			if !slices.Equal(got.depths, want.depths) || !slices.Equal(got.shadows, want.shadows) {
				t.Errorf("edges %v, %+v: the depths or the shadows differ from before", edges, cone)
			}
			now, then := got.ids(), want.ids()
			if cone.Transparency != nil || maps.Equal(now, then) {
				continue
			}
			for id := range firstHits(space, items, observer, cone) {
				if now[id] != then[id] {
					t.Errorf("edges %v, %+v: %v seen now %v, before %v", edges, cone, id, now[id],
						then[id])
				}
			}
		}
	}
}

// firstHits is every item whose copy nearest the observer is the first box a dense fan of rays
// from it meets within the radius, over more than the sweep resolves.
func firstHits(s *Space, items []Item, observer uid.UID64, cone Cone) map[uid.UID64]bool {
	eye, _ := s.grid.EntryAABB(observer)
	ox := (float64(eye.TopLeft.X) + float64(eye.BottomRight.X)) / 2
	oy := (float64(eye.TopLeft.Y) + float64(eye.BottomRight.Y)) / 2
	w, h := float64(s.Width), float64(s.Height)
	if !s.Edges.WrapsX() {
		w = 0
	}
	if !s.Edges.WrapsY() {
		h = 0
	}
	boxes := map[uid.UID64][4]float64{}
	for _, it := range items {
		if it.ID == observer {
			continue
		}
		b, _ := s.grid.EntryAABB(it.ID)
		x1, y1 := float64(b.TopLeft.X), float64(b.TopLeft.Y)
		x2, y2 := float64(b.BottomRight.X), float64(b.BottomRight.Y)
		dx := nearest((x1+x2)/2-ox, w) - ((x1+x2)/2 - ox)
		dy := nearest((y1+y2)/2-oy, h) - ((y1+y2)/2 - oy)
		boxes[it.ID] = [4]float64{x1 + dx - ox, y1 + dy - oy, x2 + dx - ox, y2 + dy - oy}
	}
	theta := math.Atan2(float64(cone.Direction.Y), float64(cone.Direction.X))
	half := float64(cone.HalfAngle)
	minArc := 2 * math.Atan2(1, float64(cone.Radius))
	steps := int(16*half/minArc) + 1
	arcs := map[uid.UID64]float64{}
	for i := 0; i <= steps; i++ {
		a := theta - half + 2*half*float64(i)/float64(steps)
		dir := [2]float64{math.Cos(a), math.Sin(a)}
		first, at := uid.UID64(0), float64(cone.Radius)
		for id, b := range boxes {
			near, far := math.Inf(-1), math.Inf(1)
			for k := range 2 {
				t1, t2 := b[k]/dir[k], b[k+2]/dir[k]
				near, far = max(near, min(t1, t2)), min(far, max(t1, t2))
			}
			if far >= near && far >= 0 && max(near, 0) <= at {
				first, at = id, max(near, 0)
			}
		}
		arcs[first] += 2 * half / float64(steps)
	}
	hits := map[uid.UID64]bool{}
	for id, arc := range arcs {
		if id != 0 && arc > minArc {
			hits[id] = true
		}
	}
	return hits
}

// The cone near a corner of a torus finds what lies round the corners across both seams.
func TestSight_QueryConeReachesAcrossTheSeams(t *testing.T) {
	space, err := NewSpace(Config{Width: 1000, Height: 600, Edges: Torus, BucketSize: 64})
	if err != nil {
		t.Fatal(err)
	}
	r := rand.New(rand.NewPCG(79, 83))
	var items []Item
	for i := range 800 {
		box := space.WrapAABB(geom.NewAABBAt(
			geom.NewVec(float32(r.Float64()*1000), float32(r.Float64()*600)),
			float32(2+r.Float64()*40), float32(2+r.Float64()*40)))
		items = append(items, Item{ID: uid.UID64(i + 1), Box: box})
	}
	space.Rebuild(items)
	for q := range 300 {
		origin := geom.NewVec(float32(r.Float64()*1000), float32(r.Float64()*600))
		if q%2 == 0 {
			origin = geom.NewVec(float32(r.Float64()*40), float32(560+r.Float64()*40))
		}
		a := r.Float64() * 2 * math.Pi
		dir := geom.NewVec(float32(math.Cos(a)), float32(math.Sin(a)))
		half, radius := float32(0.05+3*r.Float64()), float32(10+700*r.Float64())
		got := map[uid.UID64]bool{}
		sight{space}.QueryCone(origin, dir, half, radius, func(id uid.UID64) { got[id] = true })
		for _, it := range items {
			if reachesNearest(space, it, origin, dir, half, radius) && !got[it.ID] {
				t.Fatalf("cone from %v along %v, %v either side, %v long: the nearest copy of %v "+
					"reaches into it and was not reported", origin, dir, half, radius, it.ID)
			}
		}
	}
}

// reachesNearest reports whether the copy of the item's box nearest origin, as a scan takes it,
// holds a point well inside the cone, found by sampling the box.
func reachesNearest(s *Space, it Item, origin, dir geom.Vec, half, radius float32) bool {
	box, _ := s.grid.EntryAABB(it.ID)
	o := [2]float64{float64(origin.X), float64(origin.Y)}
	cx := (float64(box.TopLeft.X) + float64(box.BottomRight.X)) / 2
	cy := (float64(box.TopLeft.Y) + float64(box.BottomRight.Y)) / 2
	sx := nearest(cx-o[0], float64(s.Width)) - (cx - o[0])
	sy := nearest(cy-o[1], float64(s.Height)) - (cy - o[1])
	theta := math.Atan2(float64(dir.Y), float64(dir.X))
	for i := range 9 {
		for j := range 9 {
			x := float64(box.TopLeft.X) + float64(box.BottomRight.X-box.TopLeft.X)*float64(i)/8 + sx
			y := float64(box.TopLeft.Y) + float64(box.BottomRight.Y-box.TopLeft.Y)*float64(j)/8 + sy
			dx, dy := x-o[0], y-o[1]
			off := math.Abs(math.Remainder(math.Atan2(dy, dx)-theta, 2*math.Pi))
			if math.Hypot(dx, dy) < float64(radius)-1e-3 && off < float64(half)-1e-4 {
				return true
			}
		}
	}
	return false
}
