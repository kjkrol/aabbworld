package raycast_test

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/internal/raycast"
	"github.com/kjkrol/uid"
)

// scene builds an arrangement whose shape depends on seed, crowded and sparse by turns.
func scene(seed uint64) (*fakeSpace, raycast.Cone) {
	r := rand.New(rand.NewPCG(seed, seed+1))
	s := newFake(4000, 4000, false)
	s.put(eye, 2000, 2000, 10, 10)
	for i := range 24 - 22*int(seed%2) {
		s.put(uid.UID64(100+i),
			float32(2100+i*90+r.IntN(40)), float32(1700+r.IntN(600)),
			float32(20+r.IntN(40)), float32(20+r.IntN(40)))
	}
	return s, eastward(float32(math.Pi/3+float64(seed%3)*0.2), float32(500+r.IntN(400)))
}

// freshScan answers through a View that has never been used.
func freshScan(t *testing.T, s raycast.QueryableSpace, c raycast.Cone) ([]seenPair, []geom.Vec) {
	t.Helper()
	v := &raycast.View{}
	if !v.Scan(s, eye, c) {
		t.Fatal("Scan failed on a fresh View")
	}
	var got []seenPair
	v.Entities(func(id uid.UID64, d float32) { got = append(got, seenPair{id, d}) })
	return got, v.Outline(0, nil)
}

type seenPair struct {
	id   uid.UID64
	dist float32
}

func TestView_ReusedAcrossScansMatchesOneShotCalls(t *testing.T) {
	v := &raycast.View{}
	var buf []geom.Vec

	for seed := range uint64(8) {
		s, cone := scene(seed)
		wantIDs, wantPts := freshScan(t, s, cone)

		if !v.Scan(s, eye, cone) {
			t.Fatalf("seed %d: Scan failed on a scene a fresh View answered", seed)
		}

		var gotIDs []seenPair
		n := v.Entities(func(id uid.UID64, d float32) { gotIDs = append(gotIDs, seenPair{id, d}) })
		if n != len(wantIDs) {
			t.Fatalf("seed %d: Entities = %d, a fresh View reports %d", seed, n, len(wantIDs))
		}
		for i := range wantIDs {
			if gotIDs[i] != wantIDs[i] {
				t.Errorf("seed %d: entity %d = %v, want %v", seed, i, gotIDs[i], wantIDs[i])
			}
		}

		buf = v.Outline(0, buf[:0])
		if len(buf) != len(wantPts) {
			t.Fatalf("seed %d: outline has %d points, a fresh View gives %d", seed, len(buf), len(wantPts))
		}
		for i := range wantPts {
			if buf[i] != wantPts[i] {
				t.Errorf("seed %d: outline point %d = %v, want %v", seed, i, buf[i], wantPts[i])
			}
		}
	}
}

func TestView_ZeroValueIsUsable(t *testing.T) {
	s := newFake(2000, 2000, false)
	s.put(eye, 1000, 1000, 10, 10)
	s.put(near, 1200, 990, 20, 20)

	var v raycast.View
	if !v.Scan(s, eye, eastward(math.Pi/4, 500)) {
		t.Fatal("Scan on a zero-value View failed")
	}
	if n := v.Entities(func(uid.UID64, float32) {}); n != 1 {
		t.Errorf("Entities = %d, want 1", n)
	}
	if pts := v.Outline(0, nil); len(pts) == 0 {
		t.Error("Outline on a zero-value View returned nothing")
	}
}

func TestView_ReadsNothingWithoutASuccessfulScan(t *testing.T) {
	s := newFake(2000, 2000, false)
	s.put(eye, 1000, 1000, 10, 10)
	cone := eastward(math.Pi/4, 500)

	cases := []struct {
		name  string
		setup func(v *raycast.View)
	}{
		{"never scanned", func(*raycast.View) {}},
		{"unknown observer", func(v *raycast.View) { v.Scan(s, uid.UID64(999), cone) }},
		{"degenerate cone", func(v *raycast.View) { v.Scan(s, eye, eastward(0, 500)) }},
		{"scan invalidates an earlier one", func(v *raycast.View) {
			v.Scan(s, eye, cone)
			v.Scan(s, eye, eastward(0, 500))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := &raycast.View{}
			tc.setup(v)

			if n := v.Entities(func(uid.UID64, float32) { t.Error("fn called") }); n != 0 {
				t.Errorf("Entities = %d, want 0", n)
			}
			dst := []geom.Vec{{X: 1, Y: 2}}
			if got := v.Outline(0, dst); len(got) != 1 || got[0] != dst[0] {
				t.Errorf("Outline = %v, want dst untouched", got)
			}
		})
	}
}

func TestView_OutlineAppendsToDst(t *testing.T) {
	s := newFake(2000, 2000, false)
	s.put(eye, 1000, 1000, 10, 10)

	v := &raycast.View{}
	if !v.Scan(s, eye, eastward(math.Pi/4, 300)) {
		t.Fatal("Scan failed")
	}

	head := geom.NewVec(7.0, 9.0)
	got := v.Outline(0, []geom.Vec{head})
	if got[0] != head {
		t.Errorf("got[0] = %v, want the caller's own point %v", got[0], head)
	}
	if got[1] != geom.NewVec(1005.0, 1005.0) {
		t.Errorf("got[1] = %v, want the observer's centre", got[1])
	}
	if len(got) != len(v.Outline(0, nil))+1 {
		t.Error("appending to a non-empty dst changed how many points were produced")
	}
}
