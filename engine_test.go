package aabbworld_test

import (
	"slices"
	"testing"

	"github.com/kjkrol/aabbworld"
	"github.com/kjkrol/aabbworld/collide"
	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

func torusIf(toroidal bool) aabbworld.Edges {
	if toroidal {
		return aabbworld.Torus
	}
	return 0
}

func engineSpace(t *testing.T, toroidal bool) *aabbworld.Space {
	t.Helper()
	space, err := aabbworld.NewSpace(aabbworld.Config{
		Width: 1000, Height: 1000, Edges: torusIf(toroidal), BucketSize: 64,
	})
	if err != nil {
		t.Fatalf("NewSpace: %v", err)
	}
	return space
}

// hooks is a Handler made of optional funcs; an unset Touch confirms every pair.
type hooks struct {
	touch   func(a, b uid.UID64, pen geom.Vec) (geom.Vec, bool)
	contact func(a, b uid.UID64, pen geom.Vec)
	moved   func(id uid.UID64, box plane.AABB)
}

func (h *hooks) Touch(a, b uid.UID64, pen geom.Vec) (geom.Vec, bool) {
	if h.touch == nil {
		return pen, true
	}
	return h.touch(a, b, pen)
}

func (h *hooks) Contact(a, b uid.UID64, pen geom.Vec) {
	if h.contact != nil {
		h.contact(a, b, pen)
	}
}

func (h *hooks) Moved(id uid.UID64, box plane.AABB) {
	if h.moved != nil {
		h.moved(id, box)
	}
}

// scene is a few items the space is rebuilt from before every tick, and an engine over them.
type scene struct {
	space  *aabbworld.Space
	items  []aabbworld.Item
	moved  []uid.UID64
	hooks  hooks
	engine collide.Engine
}

func newScene(t *testing.T, toroidal bool) *scene {
	return newSceneIn(engineSpace(t, toroidal), 16)
}

func newSceneIn(space *aabbworld.Space, iterations int) *scene {
	s := &scene{space: space, items: make([]aabbworld.Item, 0, 8)}
	s.hooks.moved = func(id uid.UID64, _ plane.AABB) { s.moved = append(s.moved, id) }
	s.engine = space.CollideEngine(&s.hooks, collide.Config{Reach: 0.5, Iterations: iterations})
	return s
}

func (s *scene) put(id uid.UID64, x, y float64) *plane.AABB {
	s.items = append(s.items, aabbworld.Item{ID: id, Box: plane.NewAABB(geom.NewVec(x, y), 10, 10), Caps: aabbworld.CanCollide})
	return &s.items[len(s.items)-1].Box
}

func (s *scene) mark(id uid.UID64, caps aabbworld.Capability) {
	for i := range s.items {
		if s.items[i].ID == id {
			s.items[i].Caps |= caps
		}
	}
}

func (s *scene) tick(touch func(a, b uid.UID64, pen geom.Vec) (geom.Vec, bool), contact func(a, b uid.UID64, pen geom.Vec)) {
	s.hooks.touch, s.hooks.contact = touch, contact
	s.space.Rebuild(s.items)
	s.moved = s.moved[:0]
	s.engine.Tick()
}

// where lists every id the index finds inside box, each once.
func (s *scene) where(box geom.AABB) []uid.UID64 {
	var ids []uid.UID64
	s.space.Query(box, aabbworld.AnyCapability, func(id uid.UID64) {
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	})
	slices.Sort(ids)
	return ids
}

func TestTick_SeparatesWhatOverlaps_AndReportsWhoMoved(t *testing.T) {
	s := newScene(t, false)
	a, b := s.put(1, 100, 100), s.put(2, 104, 100)
	s.put(3, 500, 500)

	var contacts []idPair
	s.tick(nil, func(a, b uid.UID64, pen geom.Vec) {
		contacts = append(contacts, idPair{a, b})
		if pen == (geom.Vec{}) {
			t.Error("a contact was reported with no penetration")
		}
	})

	if len(contacts) != 1 || contacts[0] != (idPair{1, 2}) {
		t.Errorf("contacts = %v, want the one overlapping pair, once", contacts)
	}
	if a.Penetration(b.AABB) != (geom.Vec{}) {
		t.Errorf("a and b still overlap after Tick: %v and %v", a, b)
	}
	if a.TopLeft.X >= 100 || b.TopLeft.X <= 104 {
		t.Errorf("a at x=%v and b at x=%v, want both pushed apart", a.TopLeft.X, b.TopLeft.X)
	}
	slices.Sort(s.moved)
	if !slices.Equal(s.moved, []uid.UID64{1, 2}) {
		t.Errorf("Moved heard %v, want both pushed boxes once each", s.moved)
	}
	if got := len(s.engine.Left()); got != 0 {
		t.Errorf("Left names %d entities in a closed world, want none", got)
	}
	if got := s.where(geom.NewAABB(geom.NewVec(90, 100), geom.NewVec(99, 110))); !slices.Equal(got, []uid.UID64{1}) {
		t.Errorf("the index finds %v where only a came to rest, want [1]", got)
	}

	s.tick(nil, func(uid.UID64, uid.UID64, geom.Vec) { t.Error("a settled scene reported a contact") })
	if len(s.moved) != 0 {
		t.Errorf("a settled scene moved %v", s.moved)
	}
}

func TestTick_SeparatesAcrossTheSeamOfAToroidalSpace(t *testing.T) {
	s := newScene(t, true)
	left, right := s.put(1, 0, 100), s.put(2, 994, 100)

	s.tick(nil, nil)

	if _, overlapping := left.DeepestOverlapWith(right); overlapping {
		t.Errorf("the pair still overlaps across the seam: %v and %v", left, right)
	}
	if travelled := left.TopLeft.X; travelled > 10 {
		t.Errorf("left ended up at x=%v — it went the long way round instead of through the seam", travelled)
	}
}

func TestTick_LeavesAloneWhoCannotCollide(t *testing.T) {
	s := newScene(t, false)
	a, b := s.put(1, 100, 100), s.put(2, 104, 100)
	s.items[1].Caps = aabbworld.Plain
	startA, startB := *a, *b

	s.tick(nil, func(uid.UID64, uid.UID64, geom.Vec) { t.Error("a plain box reported a contact") })

	if *a != startA || *b != startB {
		t.Errorf("boxes moved to %v and %v although one of them cannot collide", a, b)
	}
}

func TestTick_TouchVetoesAPair(t *testing.T) {
	s := newScene(t, false)
	a, b := s.put(1, 100, 100), s.put(2, 104, 100)
	startA, startB := *a, *b

	var asked []idPair
	s.tick(func(a, b uid.UID64, pen geom.Vec) (geom.Vec, bool) {
		asked = append(asked, idPair{a, b})
		return pen, false
	}, func(uid.UID64, uid.UID64, geom.Vec) { t.Error("a pair the shapes do not touch in reported a contact") })

	if len(asked) != 1 || asked[0] != (idPair{1, 2}) {
		t.Errorf("touch asked about %v, want once for the one overlapping pair", asked)
	}
	if *a != startA || *b != startB {
		t.Errorf("boxes moved to %v and %v although touch said no", a, b)
	}
	if len(s.moved) != 0 {
		t.Errorf("Moved heard %v for a vetoed pair", s.moved)
	}
}

func TestTick_TouchMayRefineThePenetration(t *testing.T) {
	s := newSceneIn(engineSpace(t, false), 1)
	a, b := s.put(1, 100, 100), s.put(2, 104, 100)

	var reported geom.Vec
	s.tick(func(_, _ uid.UID64, pen geom.Vec) (geom.Vec, bool) {
		return geom.NewVec(pen.X/3, 0), true
	}, func(_, _ uid.UID64, pen geom.Vec) { reported = pen })

	if reported != geom.NewVec(-2, 0) {
		t.Errorf("Contact got %v, want the refined penetration (-2,0)", reported)
	}
	if a.TopLeft.X != 99 || b.TopLeft.X != 105 {
		t.Errorf("a at x=%v, b at x=%v, want one pass to move each by half of the refined 2", a.TopLeft.X, b.TopLeft.X)
	}
}

func TestTick_ASensorPairIsReportedButNeverSeparated(t *testing.T) {
	s := newScene(t, false)
	a, b := s.put(1, 100, 100), s.put(2, 104, 100)
	s.mark(2, aabbworld.Sensor)
	startA, startB := *a, *b

	reported := 0
	s.tick(nil, func(uid.UID64, uid.UID64, geom.Vec) { reported++ })

	if reported != 1 {
		t.Errorf("reported %d contacts, want 1", reported)
	}
	if *a != startA || *b != startB {
		t.Errorf("a sensor pair was separated: %v and %v", a, b)
	}
}

func TestTick_AStaticSideNeverMoves(t *testing.T) {
	s := newScene(t, false)
	a, b := s.put(1, 100, 100), s.put(2, 104, 100)
	s.mark(1, aabbworld.Static)
	startA := *a

	s.tick(nil, nil)

	if *a != startA {
		t.Errorf("the static side moved from %v to %v", startA.AABB, a.AABB)
	}
	if a.Penetration(b.AABB) != (geom.Vec{}) {
		t.Errorf("still overlapping: %v and %v", a, b)
	}
	if !slices.Equal(s.moved, []uid.UID64{2}) {
		t.Errorf("Moved heard %v, want only the side that gave way", s.moved)
	}
}

func TestTick_WhoIsPushedOutOfAnOpenEdgeIsLeftNotMoved(t *testing.T) {
	space, err := aabbworld.NewSpace(aabbworld.Config{Width: 1000, Height: 1000, Edges: aabbworld.OpenX, BucketSize: 64})
	if err != nil {
		t.Fatal(err)
	}
	s := newSceneIn(space, 16)
	s.put(1, -8, 100)
	s.put(2, 0, 100)
	s.mark(2, aabbworld.Static)

	s.tick(nil, nil)

	got := s.engine.Left()
	if len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("Left = %v, want the box pushed past the open edge", got)
	}
	if box := got[0].Box; box.BottomRight.X > 0 || box.TopLeft.Y != 100 {
		t.Errorf("Left names it at %v, want it wholly past the edge at x=0, where it was on y", box.AABB)
	}
	if len(s.moved) != 0 {
		t.Errorf("Moved heard %v for a box that left the world", s.moved)
	}
}

func TestCollideEngine_ReadsItsOwnSpace(t *testing.T) {
	crowded, empty := engineSpace(t, false), engineSpace(t, false)
	crowded.Rebuild([]aabbworld.Item{
		{ID: uid.UID64(1), Box: plane.NewAABB(geom.NewVec(10, 10), 10, 10), Caps: aabbworld.CanCollide},
		{ID: uid.UID64(2), Box: plane.NewAABB(geom.NewVec(18, 10), 10, 10), Caps: aabbworld.CanCollide},
	})

	if got := len(touched(crowded)); got != 1 {
		t.Errorf("found %d pairs in the space holding two overlapping boxes, want 1", got)
	}
	if got := len(touched(empty)); got != 0 {
		t.Errorf("found %d pairs in an empty space, want 0", got)
	}
}
