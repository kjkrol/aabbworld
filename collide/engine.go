package collide

import (
	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/kjkrol/uid"
)

// Handler is told what an Engine finds: each overlapping pair once, and every box it pushed.
type Handler interface {
	// Touch says whether the shapes in two overlapping boxes really touch, and by how much.
	Touch(a, b uid.UID64, pen geom.Vec) (geom.Vec, bool)
	// Contact is a pair Touch confirmed, with the penetration it is being separated by.
	Contact(a, b uid.UID64, pen geom.Vec)
	// Moved is a box the Engine pushed, where it came to rest.
	Moved(id uid.UID64, box plane.AABB)
}

// Config is how far an Engine looks for pairs and how long it spends separating them.
type Config struct {
	// Reach is how far a box may travel in a tick, as a fraction of its shorter side.
	Reach float32
	// Iterations caps the passes one Tick spends on chained overlaps.
	Iterations int
	// Field is the solid ground the Engine pushes movable boxes out of in the same passes as the
	// pairs; nil is none.
	Field SolidField
}

// Engine finds and separates the overlapping CanCollide boxes of its Space, on one goroutine.
type Engine interface {
	// Tick separates every overlapping pair the Space holds now and reports as it goes; the
	// Space answers its next Query with the pushed boxes without another Rebuild.
	Tick()
	// Left is who the last Tick pushed out through an open edge, and where to; good until the
	// next Tick.
	Left() []Leaver
}

// Leaver is a box an Engine pushed out through an open edge, where it came to rest — wholly past
// the edge. It is not Moved: whoever owns the box writes it back from here.
type Leaver struct {
	ID  uid.UID64
	Box plane.AABB
}
